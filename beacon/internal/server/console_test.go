package server

import (
	"bytes"
	"context"
	"io"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"gamepanel/beacon/internal/runtime"
)

type fakeConsoleSession struct {
	reader *io.PipeReader
	output *io.PipeWriter
	mu     sync.Mutex
	input  bytes.Buffer
	closed bool
}

func newFakeConsoleSession() *fakeConsoleSession {
	reader, output := io.Pipe()
	return &fakeConsoleSession{reader: reader, output: output}
}

func (s *fakeConsoleSession) Read(p []byte) (int, error) { return s.reader.Read(p) }
func (s *fakeConsoleSession) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.input.Write(p)
}
func (s *fakeConsoleSession) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()
	_ = s.output.Close()
	return s.reader.Close()
}
func (s *fakeConsoleSession) emit(value string) error {
	_, err := io.WriteString(s.output, value)
	return err
}
func (s *fakeConsoleSession) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

type consoleRuntime struct {
	stubRuntime
	mu       sync.Mutex
	sessions map[string][]*fakeConsoleSession
	attaches map[string]int
}

func newConsoleRuntime() *consoleRuntime {
	return &consoleRuntime{sessions: make(map[string][]*fakeConsoleSession), attaches: make(map[string]int)}
}

func (r *consoleRuntime) AttachConsole(_ context.Context, serverID string) (runtime.ConsoleSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	session := newFakeConsoleSession()
	r.sessions[serverID] = append(r.sessions[serverID], session)
	r.attaches[serverID]++
	return session, nil
}

func (r *consoleRuntime) session(serverID string) *fakeConsoleSession {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.sessions[serverID]
	if len(list) == 0 {
		return nil
	}
	return list[len(list)-1]
}

func receiveConsole(t *testing.T, channel <-chan []byte) string {
	t.Helper()
	select {
	case value, ok := <-channel:
		if !ok {
			t.Fatal("console subscription closed unexpectedly")
		}
		return string(value)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for console output")
		return ""
	}
}

func TestConsoleOutputIsServerScoped(t *testing.T) {
	rt := newConsoleRuntime()
	manager := newConsoleManager(context.Background(), rt)
	defer manager.Close()
	if err := manager.Ensure("server-a"); err != nil {
		t.Fatal(err)
	}
	if err := manager.Ensure("server-b"); err != nil {
		t.Fatal(err)
	}
	a, unsubscribeA, err := manager.Subscribe("server-a")
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribeA()
	b, unsubscribeB, err := manager.Subscribe("server-b")
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribeB()

	if err := rt.session("server-a").emit("only-a"); err != nil {
		t.Fatal(err)
	}
	if got := receiveConsole(t, a); got != "only-a" {
		t.Fatalf("server A output = %q", got)
	}
	select {
	case got := <-b:
		t.Fatalf("server A output leaked to server B: %q", got)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestConsoleReplayAndSubscriberCleanup(t *testing.T) {
	rt := newConsoleRuntime()
	manager := newConsoleManager(context.Background(), rt)
	defer manager.Close()
	if err := manager.Ensure("server-a"); err != nil {
		t.Fatal(err)
	}
	if err := rt.session("server-a").emit("replayed"); err != nil {
		t.Fatal(err)
	}
	channel, unsubscribe, err := manager.Subscribe("server-a")
	if err != nil {
		t.Fatal(err)
	}
	if got := receiveConsole(t, channel); got != "replayed" {
		t.Fatalf("replay = %q", got)
	}
	unsubscribe()

	manager.mu.Lock()
	producer := manager.producers["server-a"]
	manager.mu.Unlock()
	producer.mu.Lock()
	subscribers := len(producer.subs)
	producer.mu.Unlock()
	if subscribers != 0 {
		t.Fatalf("subscriber leak: %d remain", subscribers)
	}
}

func TestConsoleStopDeleteAndNoProducerLeaks(t *testing.T) {
	rt := newConsoleRuntime()
	server, _ := NewServer(rt, t.TempDir())
	defer server.Shutdown()

	if err := server.consoles.Ensure("stop-me"); err != nil {
		t.Fatal(err)
	}
	first := rt.session("stop-me")
	if err := server.manager.HandlePower(context.Background(), "stop-me", "stop"); err != nil {
		t.Fatalf("stop server: %v", err)
	}
	if !first.isClosed() || server.consoles.producerCount() != 0 {
		t.Fatal("stop did not close and remove console producer")
	}
	if err := server.consoles.Ensure("stop-me"); err != nil {
		t.Fatal(err)
	}
	if err := server.consoles.Ensure("stop-me"); err != nil {
		t.Fatal(err)
	}
	rt.mu.Lock()
	attaches := rt.attaches["stop-me"]
	rt.mu.Unlock()
	if attaches != 2 {
		t.Fatalf("expected one attach per lifecycle, got %d", attaches)
	}

	if err := server.consoles.Ensure("delete-me"); err != nil {
		t.Fatal(err)
	}
	deleted := rt.session("delete-me")
	req := httptest.NewRequest("DELETE", "/servers/delete-me", nil)
	req.SetPathValue("id", "delete-me")
	rec := httptest.NewRecorder()
	server.delete(rec, req)
	if rec.Code != 202 {
		t.Fatalf("delete status = %d: %s", rec.Code, rec.Body.String())
	}
	if !deleted.isClosed() {
		t.Fatal("delete did not close console producer")
	}

	server.consoles.Close()
	if server.consoles.producerCount() != 0 {
		t.Fatal("console producers leaked after shutdown")
	}
}
