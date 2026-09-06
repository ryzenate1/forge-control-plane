package server

import "testing"

func TestValidAdminContainerName(t *testing.T) {
	valid := []string{"nginx-1", "smoke_track2", "my.app", "a", "ABC123", "x.y_z-9"}
	for _, name := range valid {
		if !validAdminContainerName(name) {
			t.Errorf("expected container name %q to be accepted", name)
		}
	}
	invalid := []string{
		"", "../escape", "/absolute", "has space", "semi;colon", "dq\"q",
		"sq'q", "back`t", "dollar$var", "pipe|p", "amp&x", "paren(x)",
		".leading-dot", "-leading-dash", "trailing/",
		string(make([]byte, 129)),
	}
	for _, name := range invalid {
		if validAdminContainerName(name) {
			t.Errorf("expected container name %q to be rejected", name)
		}
	}
}
