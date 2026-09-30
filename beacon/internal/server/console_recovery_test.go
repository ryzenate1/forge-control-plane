package server

import (
	"context"
	"testing"
	"time"
)

func TestConsoleJournalRecoveryAfterCrash(t *testing.T) {
	rt := newConsoleRuntime()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager := newConsoleManager(ctx, rt)
	if err := manager.Ensure("server-a"); err != nil {
		t.Fatal(err)
	}
	session := rt.session("server-a")
	if err := session.emit("before-crash"); err != nil {
		t.Fatal(err)
	}
	// Subscribe and verify replay contains before-crash
	ch, unsub, err := manager.Subscribe("server-a")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case msg := <-ch:
		if string(msg) != "before-crash" {
			t.Fatalf("replay before crash = %q want %q", string(msg), "before-crash")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for replay before crash")
	}
	unsub()

	// Simulate crash: close manager (which closes producers and channels)
	manager.Close()
	if manager.producerCount() != 0 {
		t.Fatalf("manager should have 0 producers after Close, got %d", manager.producerCount())
	}
	if !session.isClosed() {
		t.Fatal("session should be closed after manager Close")
	}

	// Recovery: create new manager with same runtime (simulating daemon restart)
	// It should be able to Ensure again without leaking previous producer
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	manager2 := newConsoleManager(ctx2, rt)
	defer manager2.Close()
	if err := manager2.Ensure("server-a"); err != nil {
		t.Fatalf("Ensure after crash should succeed, got %v", err)
	}
	if manager2.producerCount() != 1 {
		t.Fatalf("after recovery, producerCount=%d want 1", manager2.producerCount())
	}
	newSession := rt.session("server-a")
	if newSession == session {
		t.Fatal("recovered session should be new instance")
	}
	if err := newSession.emit("after-crash"); err != nil {
		t.Fatal(err)
	}
	ch2, unsub2, err := manager2.Subscribe("server-a")
	if err != nil {
		t.Fatal(err)
	}
	defer unsub2()
	// New subscriber should see replay of after-crash (but not before-crash, since replay is per-producer)
	select {
	case msg := <-ch2:
		if string(msg) != "after-crash" {
			t.Fatalf("replay after crash = %q want %q", string(msg), "after-crash")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for replay after crash")
	}
	// Verify Stop also cleans up correctly (crash during active console)
	manager2.Stop("server-a")
	if manager2.producerCount() != 0 {
		t.Fatalf("after Stop, producerCount=%d want 0", manager2.producerCount())
	}
}

func TestConsoleManagerHandlesMultipleCrashRecoveries(t *testing.T) {
	rt := newConsoleRuntime()
	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		manager := newConsoleManager(ctx, rt)
		if err := manager.Ensure("server-a"); err != nil {
			t.Fatalf("iteration %d Ensure failed: %v", i, err)
		}
		session := rt.session("server-a")
		if err := session.emit("msg"); err != nil {
			t.Fatalf("iteration %d emit failed: %v", i, err)
		}
		manager.Close()
		cancel()
		if manager.producerCount() != 0 {
			t.Fatalf("iteration %d leaked producers", i)
		}
		if !session.isClosed() {
			t.Fatalf("iteration %d session not closed", i)
		}
	}
}
