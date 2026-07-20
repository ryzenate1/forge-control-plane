package protocol

import "testing"

func TestCommandValidation(t *testing.T) {
	command := Command{ID: "command-1", IdempotencyKey: "key-1", Type: "server.start", ServerID: "server-1", DesiredGeneration: 1, ProtocolVersion: CurrentVersion, Payload: []byte(`{}`)}
	if err := command.Validate(); err != nil {
		t.Fatal(err)
	}
	command.ProtocolVersion++
	if err := command.Validate(); err == nil {
		t.Fatal("expected unsupported version error")
	}
}
