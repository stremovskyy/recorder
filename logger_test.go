package recorder

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"log/slog"
)

func TestGetTraceID(t *testing.T) {
	ctx := context.WithValue(context.Background(), "trace_id", "abc123")
	if got := getTraceID(ctx); got != "abc123" {
		t.Fatalf("expected trace id abc123, got %s", got)
	}

	if got := getTraceID(context.Background()); got != "" {
		t.Fatalf("expected empty trace id, got %s", got)
	}

	ctx = context.WithValue(context.Background(), "trace_id", 123)
	if got := getTraceID(ctx); got != "" {
		t.Fatalf("expected empty trace id for non-string value, got %s", got)
	}
}

func TestLoggerWithContextAddsTraceID(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	base := &slogLogger{logger: slog.New(handler)}

	ctx := context.WithValue(context.Background(), "trace_id", "abc123")
	logger := base.With("component", "test").WithContext(ctx)
	logger.Info("hello", "key", "value")

	output := buf.String()
	if !strings.Contains(output, "\"trace_id\":\"abc123\"") {
		t.Fatalf("expected trace id in log output, got %s", output)
	}
	if !strings.Contains(output, "\"component\":\"test\"") {
		t.Fatalf("expected component attribute in output, got %s", output)
	}
}

func TestNewLoggerConstructors(t *testing.T) {
	if logger := NewLogger(slog.LevelInfo); logger == nil {
		t.Fatal("expected logger instance")
	}
	if logger := NewDefaultLogger(); logger == nil {
		t.Fatal("expected default logger instance")
	}
}
