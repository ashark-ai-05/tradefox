// Package plugin provides infrastructure for running and managing VisualHFT
// plugins, including goroutine supervision with panic recovery and automatic
// restart logic.
package plugin

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"runtime"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/core/enums"
)

const defaultMaxRetries = 5

// Supervisor wraps a plugin's main goroutine with panic recovery and restart
// logic. If the supervised function panics or returns a non-nil error, the
// Supervisor recovers, logs the incident with a stack trace, and restarts the
// function after exponential backoff. A clean return (nil error) causes the
// Supervisor to exit normally.
type Supervisor struct {
	name           string
	runFn          func(ctx context.Context) error
	maxRetries     int
	logger         *slog.Logger
	onStatusChange func(status enums.PluginStatus)
}

// NewSupervisor creates a Supervisor that will execute runFn with panic
// recovery and automatic restart. The default maximum retry count is 5.
func NewSupervisor(name string, runFn func(ctx context.Context) error, logger *slog.Logger) *Supervisor {
	return &Supervisor{
		name:       name,
		runFn:      runFn,
		maxRetries: defaultMaxRetries,
		logger:     logger,
	}
}

// SetMaxRetries overrides the default max retries (5).
func (s *Supervisor) SetMaxRetries(n int) {
	s.maxRetries = n
}

// SetStatusCallback sets a function called when the supervisor changes the
// plugin status (e.g. to PluginStarting or PluginStoppedFailed).
func (s *Supervisor) SetStatusCallback(fn func(enums.PluginStatus)) {
	s.onStatusChange = fn
}

// Run executes runFn in a supervised loop. If runFn panics, the supervisor
// recovers, logs the panic with stack trace, and restarts after exponential
// backoff. If runFn returns a non-nil error, it is treated like a panic
// (restart with backoff). If runFn returns nil, the supervisor considers it a
// clean stop and exits. Respects ctx cancellation -- when ctx is cancelled,
// the supervisor stops. Returns nil on clean stop or context cancellation,
// error if max retries are exceeded.
func (s *Supervisor) Run(ctx context.Context) error {
	attempt := 0

	for attempt < s.maxRetries {
		// Check context before each attempt.
		if ctx.Err() != nil {
			return nil
		}

		err := s.runOnce(ctx)

		// Clean exit: runFn returned nil without panic.
		if err == nil {
			return nil
		}

		// Context was cancelled during execution.
		if ctx.Err() != nil {
			return nil
		}

		// Panic or error: increment attempt and possibly restart.
		attempt++
		s.logger.Error("supervisor: plugin failed",
			"plugin", s.name,
			"attempt", attempt,
			"maxRetries", s.maxRetries,
			"error", err,
		)

		if attempt >= s.maxRetries {
			s.notifyStatus(enums.PluginStoppedFailed)
			return fmt.Errorf("supervisor: plugin %q exceeded max retries (%d): %w", s.name, s.maxRetries, err)
		}

		// Signal that we are restarting.
		s.notifyStatus(enums.PluginStarting)

		// Exponential backoff: 2^attempt seconds + random jitter [0, 1000ms).
		backoff := (1 << attempt) * time.Second
		jitter := time.Duration(rand.Int63n(int64(time.Second))) // 0-999ms
		backoff += jitter

		select {
		case <-time.After(backoff):
			// Continue to next attempt.
		case <-ctx.Done():
			return nil
		}
	}

	// Should not be reached because the loop body handles max retries,
	// but included for safety.
	return nil
}

// runOnce calls runFn, recovering from panics. It returns nil for a clean
// exit, or an error describing the panic / runFn error.
func (s *Supervisor) runOnce(ctx context.Context) (retErr error) {
	defer func() {
		if r := recover(); r != nil {
			// Capture the stack trace.
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			stack := string(buf[:n])

			s.logger.Error("supervisor: recovered panic",
				"plugin", s.name,
				"panic", r,
				"stack", stack,
			)

			retErr = fmt.Errorf("panic: %v", r)
		}
	}()

	return s.runFn(ctx)
}

// notifyStatus calls the onStatusChange callback if one has been set.
func (s *Supervisor) notifyStatus(status enums.PluginStatus) {
	if s.onStatusChange != nil {
		s.onStatusChange(status)
	}
}
