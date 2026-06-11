package log

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestDiscard(t *testing.T) {
	l := Discard()
	l.Info("should not panic")
	if l.Enabled(context.Background(), slog.LevelError) {
		t.Error("discard logger should not be enabled for any level")
	}
}

func TestFromContextDefault(t *testing.T) {
	l := FromContext(context.Background())
	if l == nil {
		t.Fatal("FromContext should never return nil")
	}
	if l.Enabled(context.Background(), slog.LevelError) {
		t.Error("default logger should be discard")
	}
}

func TestWithLogger(t *testing.T) {
	var buf bytes.Buffer
	h := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(h)

	ctx := WithLogger(context.Background(), logger)
	got := FromContext(ctx)
	got.Info("test message", "key", "value")

	if !strings.Contains(buf.String(), "test message") {
		t.Errorf("expected log output, got: %s", buf.String())
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"trace", LevelTrace},
		{"TRACE", LevelTrace},
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
	}

	for _, tt := range tests {
		got, err := ParseLevel(tt.input)
		if err != nil {
			t.Errorf("ParseLevel(%q) error: %v", tt.input, err)
		}
		if got != tt.want {
			t.Errorf("ParseLevel(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}

	_, err := ParseLevel("bogus")
	if err == nil {
		t.Error("ParseLevel(bogus) should return error")
	}
}

func TestReplaceLevelAttr(t *testing.T) {
	a := slog.Attr{Key: slog.LevelKey, Value: slog.AnyValue(LevelTrace)}
	got := ReplaceLevelAttr(nil, a)
	if got.Value.String() != "TRACE" {
		t.Errorf("expected TRACE, got %s", got.Value.String())
	}

	a2 := slog.Attr{Key: slog.LevelKey, Value: slog.AnyValue(slog.LevelInfo)}
	got2 := ReplaceLevelAttr(nil, a2)
	if got2.Value.String() == "TRACE" {
		t.Error("INFO level should not be replaced with TRACE")
	}
}

func TestTraceLevelOutput(t *testing.T) {
	var buf bytes.Buffer
	h := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level:       LevelTrace,
		ReplaceAttr: ReplaceLevelAttr,
	})
	logger := slog.New(h)
	logger.Log(context.Background(), LevelTrace, "trace message")

	if !strings.Contains(buf.String(), "TRACE") {
		t.Errorf("expected TRACE in output, got: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "trace message") {
		t.Errorf("expected trace message in output, got: %s", buf.String())
	}
}
