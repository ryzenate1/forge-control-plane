package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestServerModel(t *testing.T) {
	now := time.Now()

	server := Server{
		ID:        "1",
		Name:      "Test Server",
		NodeID:    "node1",
		Status:    ServerStatusRunning,
		CreatedAt: now,
		UpdatedAt: now,
	}

	assert.Equal(t, "1", server.ID)
	assert.Equal(t, "Test Server", server.Name)
	assert.Equal(t, "node1", server.NodeID)
	assert.Equal(t, ServerStatusRunning, server.Status)
	assert.Equal(t, now, server.CreatedAt)
	assert.Equal(t, now, server.UpdatedAt)
}

func TestServerStatus(t *testing.T) {
	testCases := []struct {
		status      ServerStatus
		stringValue string
	}{
		{ServerStatusStarting, "starting"},
		{ServerStatusRunning, "running"},
		{ServerStatusStopping, "stopping"},
		{ServerStatusStopped, "stopped"},
		{ServerStatusCrashed, "crashed"},
	}

	for _, tc := range testCases {
		assert.Equal(t, tc.stringValue, string(tc.status))
	}
}
