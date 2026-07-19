package orchestrator

import (
	"testing"

	"gamepanel/forge/internal/services/clustermanager"
)

// Compile-time interface checks
var _ ServerLifecycle = (*clustermanager.Service)(nil)
var _ PowerOperations = (*clustermanager.Service)(nil)
var _ CapacityViewer = (*clustermanager.Service)(nil)
var _ NodeReconciler = (*clustermanager.Service)(nil)

func TestInterfaceCompliance(t *testing.T) {
	// Intentionally empty - compile-time checks above verify interface compliance
}
