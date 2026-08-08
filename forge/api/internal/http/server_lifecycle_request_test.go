package http

import (
	"encoding/json"
	"testing"
)

func TestUpdateServerRequestDistinguishesOmittedAndZeroValues(t *testing.T) {
	var omitted UpdateServerRequest
	if err := json.Unmarshal([]byte(`{"name":"renamed"}`), &omitted); err != nil {
		t.Fatal(err)
	}
	if omitted.MemoryMB != nil || omitted.OOMDisabled != nil {
		t.Fatal("omitted patch fields must remain nil")
	}

	var explicit UpdateServerRequest
	if err := json.Unmarshal([]byte(`{"memoryMb":0,"oomDisabled":false,"description":""}`), &explicit); err != nil {
		t.Fatal(err)
	}
	if explicit.MemoryMB == nil || *explicit.MemoryMB != 0 {
		t.Fatal("explicit zero memory value was not preserved")
	}
	if explicit.OOMDisabled == nil || *explicit.OOMDisabled {
		t.Fatal("explicit false oomDisabled value was not preserved")
	}
	if explicit.Description == nil || *explicit.Description != "" {
		t.Fatal("explicit empty description was not preserved")
	}
}

func TestCreateServerRequestDistinguishesOmittedAndExplicitZeroResources(t *testing.T) {
	var omitted CreateServerRequest
	if err := json.Unmarshal([]byte(`{"name":"server"}`), &omitted); err != nil {
		t.Fatal(err)
	}
	if omitted.MemoryMB != nil || omitted.CPU != nil || omitted.IOWeight != nil {
		t.Fatal("omitted create resources must remain nil so defaults are intentional")
	}

	var explicit CreateServerRequest
	if err := json.Unmarshal([]byte(`{"memoryMb":0,"cpu":0,"ioWeight":0}`), &explicit); err != nil {
		t.Fatal(err)
	}
	if explicit.MemoryMB == nil || *explicit.MemoryMB != 0 || explicit.CPU == nil || *explicit.CPU != 0 || explicit.IOWeight == nil || *explicit.IOWeight != 0 {
		t.Fatal("explicit zero create resources were not preserved")
	}
}
