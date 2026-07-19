package logging

import (
    "go.uber.org/zap"
    "go.uber.org/zap/zapcore"
)

type noopLogger struct{}

func (n *noopLogger) Debug(msg string, fields ...Field)  {}
func (n *noopLogger) Info(msg string, fields ...Field)   {}
func (n *noopLogger) Warn(msg string, fields ...Field)   {}
func (n *noopLogger) Error(msg string, fields ...Field)  {}
func (n *noopLogger) WithFields(fields ...Field) Logger  { return n }

func NewNoopLogger() *noopLogger { return &noopLogger{} }

// Logger defines an interface for structured logging
type Logger interface {
    Debug(msg string, fields ...Field)
    Info(msg string, fields ...Field)
    Warn(msg string, fields ...Field)
    Error(msg string, fields ...Field)
    WithFields(fields ...Field) Logger
}

// Field represents a key-value pair for structured logging
type Field struct {
    Key   string
    Value interface{}
}

// ZapLogger implements Logger using zap

type ZapLogger struct {
    logger *zap.Logger
}

// NewZapLogger creates a new ZapLogger
func NewZapLogger() (*ZapLogger, error) {
    config := zap.NewProductionConfig()
    config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

    logger, err := config.Build()
    if err != nil {
        return nil, err
    }

    return &ZapLogger{logger: logger}, nil
}

// Debug logs a debug message
func (z *ZapLogger) Debug(msg string, fields ...Field) {
    z.logger.Debug(msg, convertFields(fields)...)
}

// Info logs an info message
func (z *ZapLogger) Info(msg string, fields ...Field) {
    z.logger.Info(msg, convertFields(fields)...)
}

// Warn logs a warning message
func (z *ZapLogger) Warn(msg string, fields ...Field) {
    z.logger.Warn(msg, convertFields(fields)...)
}

// Error logs an error message
func (z *ZapLogger) Error(msg string, fields ...Field) {
    z.logger.Error(msg, convertFields(fields)...)
}

// WithFields creates a new logger with additional fields
func (z *ZapLogger) WithFields(fields ...Field) Logger {
    return &ZapLogger{
        logger: z.logger.With(convertFields(fields)...),
    }
}

// convertFields converts our Field type to zap.Field
func convertFields(fields []Field) []zap.Field {
    zapFields := make([]zap.Field, len(fields))
    for i, field := range fields {
        zapFields[i] = zap.Any(field.Key, field.Value)
    }
    return zapFields
}
