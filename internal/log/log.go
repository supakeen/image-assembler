// Package log provides context-propagated structured logging with a custom
// Trace level below Debug. By default all logging is discarded (zero overhead);
// the CLI enables it via --log-level. This separation keeps diagnostic output
// independent of the user-facing monitor system.
package log

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// LevelTrace is below slog.LevelDebug and used for very high-volume output
// like IPC message traffic and registry filesystem walks.
const LevelTrace = slog.Level(-8)

type ctxKey struct{}

// WithLogger stores a logger in the context for downstream extraction via
// FromContext. This is the sole propagation mechanism — no global state.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, logger)
}

// FromContext returns the logger from ctx, or Discard() if none was set.
// Safe to call unconditionally — never returns nil.
func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok && l != nil {
		return l
	}
	return Discard()
}

var discardLogger *slog.Logger

// Discard returns a logger whose handler rejects all levels, so calls like
// logger.Info(...) are effectively free. Used as the default when no
// --log-level is specified.
func Discard() *slog.Logger {
	if discardLogger == nil {
		discardLogger = slog.New(discardHandler{})
	}
	return discardLogger
}

// ParseLevel extends slog's built-in level parsing with "trace" support.
func ParseLevel(s string) (slog.Level, error) {
	if strings.EqualFold(s, "trace") {
		return LevelTrace, nil
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(s)); err != nil {
		return 0, fmt.Errorf("unknown log level %q (valid: trace, debug, info, warn)", s)
	}
	return level, nil
}

// ReplaceLevelAttr is a slog.HandlerOptions.ReplaceAttr function that renders
// LevelTrace as "TRACE" in JSON output instead of the numeric default.
func ReplaceLevelAttr(_ []string, a slog.Attr) slog.Attr {
	if a.Key == slog.LevelKey {
		if level, ok := a.Value.Any().(slog.Level); ok && level == LevelTrace {
			a.Value = slog.StringValue("TRACE")
		}
	}
	return a
}

type discardHandler struct{}

func (discardHandler) Enabled(context.Context, slog.Level) bool  { return false }
func (discardHandler) Handle(context.Context, slog.Record) error { return nil }
func (d discardHandler) WithAttrs([]slog.Attr) slog.Handler      { return d }
func (d discardHandler) WithGroup(string) slog.Handler            { return d }
