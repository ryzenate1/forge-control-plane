package sftpserver

import (
	"testing"
	"time"
)

func TestEventString(t *testing.T) {
	e := Event{
		Action:    ActionFileRead,
		Path:      "/test/file.txt",
		UserID:    "user-1",
		ServerID:  "srv-1",
		IP:        "127.0.0.1",
		SessionID: "sess-1",
		Timestamp: time.Now(),
	}
	s := e.String()
	if s != "sftp.file.read /test/file.txt by user-1 on srv-1" {
		t.Fatalf("unexpected string: %q", s)
	}
}

func TestMultiPublisherFanOut(t *testing.T) {
	var calls1, calls2 int
	p1 := &testPublisher{fn: func(e Event) { calls1++ }}
	p2 := &testPublisher{fn: func(e Event) { calls2++ }}
	mp := NewMultiPublisher(p1, p2)
	mp.PublishEvent(Event{Action: ActionFileRead, Path: "/a", UserID: "u", ServerID: "s"})
	if calls1 != 1 || calls2 != 1 {
		t.Fatalf("expected both publishers called, got %d, %d", calls1, calls2)
	}
}

func TestActivityDBPublisherNilDB(t *testing.T) {
	p := NewActivityDBPublisher(nil)
	p.PublishEvent(Event{Action: ActionFileRead, Path: "/a", UserID: "u", ServerID: "s"})
}

func TestActionConstantsNonEmpty(t *testing.T) {
	actions := []Action{
		ActionFileRead,
		ActionFileWrite,
		ActionFileCreate,
		ActionFileDelete,
		ActionFileMkdir,
		ActionFileRename,
		ActionFileChmod,
		ActionFileList,
		ActionFileStat,
	}
	for _, a := range actions {
		if string(a) == "" {
			t.Error("action constant is empty")
		}
	}
}

type testPublisher struct {
	fn func(Event)
}

func (p *testPublisher) PublishEvent(event Event) {
	p.fn(event)
}
