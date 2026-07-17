package logging

import (
	"testing"
)

func TestZapLogger(t *testing.T) {
	logger, err := NewZapLogger()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Test logging at different levels
	logger.Debug("Debug message", Field{Key: "key", Value: "value"})
	logger.Info("Info message", Field{Key: "key", Value: "value"})
	logger.Warn("Warn message", Field{Key: "key", Value: "value"})
	logger.Error("Error message", Field{Key: "key", Value: "value"})

	// Test WithFields
	childLogger := logger.WithFields(Field{Key: "parent", Value: "value"})
	childLogger.Info("Child logger message")
}
