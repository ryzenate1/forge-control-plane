package compose

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeHash(t *testing.T) {
	a := computeHash("version: '3'\nservices:\n  web:\n    image: nginx")
	b := computeHash("version: '3'\nservices:\n  web:\n    image: nginx")
	c := computeHash("version: '3'\nservices:\n  web:\n    image: nginx:alpine")

	assert.Equal(t, a, b, "same yaml should produce same hash")
	assert.NotEqual(t, a, c, "different yaml should produce different hash")
}

func TestMapsEqual(t *testing.T) {
	assert.True(t, mapsEqual(nil, nil))
	assert.True(t, mapsEqual(map[string]string{"a": "1"}, map[string]string{"a": "1"}))
	assert.False(t, mapsEqual(map[string]string{"a": "1"}, map[string]string{"a": "2"}))
	assert.False(t, mapsEqual(map[string]string{"a": "1"}, map[string]string{"a": "1", "b": "2"}))
	assert.False(t, mapsEqual(map[string]string{"a": "1"}, nil))
}

func TestContainsAny(t *testing.T) {
	assert.True(t, containsAny("container not found", "not found"))
	assert.True(t, containsAny("No such container: abc", "No such container"))
	assert.False(t, containsAny("success", "not found"))
	assert.True(t, containsAny("HTTP 404 Not Found", "404"))
}

func TestIsNotFoundError(t *testing.T) {
	err := &testError{msg: "No such container: cps-abc123"}
	assert.True(t, isNotFoundError(err))

	assert.False(t, isNotFoundError(&testError{msg: "success"}))
	assert.False(t, isNotFoundError(nil))
}

type testError struct {
	msg string
}

func (e *testError) Error() string { return e.msg }

func TestCreateStackID(t *testing.T) {
	s := &Service{}
	id1 := s.createStackID()
	id2 := s.createStackID()

	assert.Contains(t, id1, "cps-")
	assert.Contains(t, id2, "cps-")
	assert.Equal(t, len("cps-")+12, len(id1))
}

func TestDeployComposeStack_Validation(t *testing.T) {
	s := &Service{}
	ctx := context.Background()

	_, err := s.DeployComposeStack(ctx, DeployComposeRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name")

	_, err = s.DeployComposeStack(ctx, DeployComposeRequest{
		Name:        "test",
		ComposeYAML: "version: '3'",
		UserID:      "user-1",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "node selection")
}

func TestStackStatusConstants(t *testing.T) {
	assert.Equal(t, StackStatus("deploying"), StackStatusDeploying)
	assert.Equal(t, StackStatus("running"), StackStatusRunning)
	assert.Equal(t, StackStatus("stopped"), StackStatusStopped)
	assert.Equal(t, StackStatus("degraded"), StackStatusDegraded)
	assert.Equal(t, StackStatus("updating"), StackStatusUpdating)
	assert.Equal(t, StackStatus("rolling_back"), StackStatusRollingBack)
	assert.Equal(t, StackStatus("deleting"), StackStatusDeleting)
	assert.Equal(t, StackStatus("deleted"), StackStatusDeleted)
	assert.Equal(t, StackStatus("failed"), StackStatusFailed)
}

func TestComposeStackConversion(t *testing.T) {
	now := time.Now().UTC()
	stack := &ComposeStack{
		ID:            "cps-test123",
		UserID:        "user-1",
		Name:          "my-stack",
		NodeID:        "node-1",
		Status:        StackStatusRunning,
		ComposeYAML:   "version: '3'",
		ComposeHash:   "abc123",
		EnvVars:       map[string]string{"DB_HOST": "localhost"},
		MemoryMB:      512,
		CPUShares:     100,
		DiskMB:        1024,
		Error:         "",
		ReservationID: "res-1",
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	storeStack := toStoreComposeStack(stack)
	require.NotNil(t, storeStack)
	assert.Equal(t, stack.ID, storeStack.ID)
	assert.Equal(t, stack.UserID, storeStack.UserID)
	assert.Equal(t, stack.Name, storeStack.Name)
	assert.Equal(t, stack.NodeID, storeStack.NodeID)
	assert.Equal(t, string(stack.Status), storeStack.Status)
	assert.Equal(t, stack.ComposeYAML, storeStack.ComposeYAML)
	assert.Equal(t, stack.ComposeHash, storeStack.ComposeHash)
	assert.Equal(t, stack.MemoryMB, storeStack.MemoryMB)
	assert.Equal(t, stack.CPUShares, storeStack.CPUShares)
	assert.Equal(t, stack.DiskMB, storeStack.DiskMB)
	assert.Equal(t, stack.Error, storeStack.Error)
	assert.Equal(t, stack.ReservationID, storeStack.ReservationID)

	roundTripped := fromStoreComposeStack(*storeStack)
	assert.Equal(t, stack.ID, roundTripped.ID)
	assert.Equal(t, stack.Status, roundTripped.Status)
	assert.Equal(t, stack.ComposeYAML, roundTripped.ComposeYAML)
	assert.Equal(t, stack.EnvVars, roundTripped.EnvVars)
}

func TestUpdateComposeStack_NoChange(t *testing.T) {
	t.Skip("requires database store")
}

func TestDeleteComposeStack_AlreadyDeleted(t *testing.T) {
	t.Skip("requires database store")
}
