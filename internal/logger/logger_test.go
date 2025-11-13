package logger

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNoopLogger(t *testing.T) {
	logger := &NoopLogger{}

	// Should not panic
	logger.Debug("test")
	logger.Info("test")
	logger.Warn("test")
	logger.Error("test")

	// With arguments
	logger.Debug("test", "key", "value")
	logger.Info("test", "key", "value")
	logger.Warn("test", "key", "value")
	logger.Error("test", "key", "value")
}

func TestSlogAdapter(t *testing.T) {
	tests := []struct {
		name       string
		logFunc    func(Logger, string, ...any)
		message    string
		args       []any
		wantLevel  string
		wantMsg    string
		wantFields map[string]string
	}{
		{
			name:      "Debug level",
			logFunc:   func(l Logger, msg string, args ...any) { l.Debug(msg, args...) },
			message:   "debug message",
			args:      []any{"key", "value"},
			wantLevel: "DEBUG",
			wantMsg:   "debug message",
			wantFields: map[string]string{
				"key": "value",
			},
		},
		{
			name:      "Info level",
			logFunc:   func(l Logger, msg string, args ...any) { l.Info(msg, args...) },
			message:   "info message",
			args:      []any{"status", "active"},
			wantLevel: "INFO",
			wantMsg:   "info message",
			wantFields: map[string]string{
				"status": "active",
			},
		},
		{
			name:      "Warn level",
			logFunc:   func(l Logger, msg string, args ...any) { l.Warn(msg, args...) },
			message:   "warning message",
			args:      []any{"code", "123"},
			wantLevel: "WARN",
			wantMsg:   "warning message",
			wantFields: map[string]string{
				"code": "123",
			},
		},
		{
			name:      "Error level",
			logFunc:   func(l Logger, msg string, args ...any) { l.Error(msg, args...) },
			message:   "error message",
			args:      []any{"error", "connection failed"},
			wantLevel: "ERROR",
			wantMsg:   "error message",
			wantFields: map[string]string{
				"error": "connection failed",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			})
			slogger := slog.New(handler)
			logger := NewSlogAdapter(slogger)

			tt.logFunc(logger, tt.message, tt.args...)

			output := buf.String()
			assert.Contains(t, output, tt.wantLevel)
			assert.Contains(t, output, tt.wantMsg)
			for key, value := range tt.wantFields {
				// slog quotes string values
				assert.Contains(t, output, key+"=")
				assert.Contains(t, output, value)
			}
		})
	}
}

func TestSlogAdapterJSONHandler(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	slogger := slog.New(handler)
	logger := NewSlogAdapter(slogger)

	logger.Info("query executed",
		"sql", "SELECT * FROM users WHERE id = ?",
		"duration_ms", 15,
		"rows", 1)

	output := buf.String()
	assert.Contains(t, output, `"msg":"query executed"`)
	assert.Contains(t, output, `"sql":"SELECT * FROM users WHERE id = ?"`)
	assert.Contains(t, output, `"duration_ms":15`)
	assert.Contains(t, output, `"rows":1`)
}

func TestSlogAdapterMultipleFields(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, nil)
	slogger := slog.New(handler)
	logger := NewSlogAdapter(slogger)

	logger.Info("complex log",
		"string", "value",
		"int", 42,
		"bool", true,
		"nil", nil)

	output := buf.String()
	assert.Contains(t, output, "string=value")
	assert.Contains(t, output, "int=42")
	assert.Contains(t, output, "bool=true")
	assert.Contains(t, output, "nil=<nil>")
}

func BenchmarkNoopLogger(b *testing.B) {
	logger := &NoopLogger{}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		logger.Info("query executed",
			"sql", "SELECT * FROM users",
			"duration_ms", 15,
			"rows", 100)
	}
}

func BenchmarkSlogAdapter(b *testing.B) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, nil)
	slogger := slog.New(handler)
	logger := NewSlogAdapter(slogger)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		logger.Info("query executed",
			"sql", "SELECT * FROM users",
			"duration_ms", 15,
			"rows", 100)
	}
}
