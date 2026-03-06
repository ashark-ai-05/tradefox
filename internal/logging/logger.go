package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// Setup configures the global slog logger with both file and console output.
// level: minimum log level (e.g., slog.LevelInfo)
// logFile: path to log file. If empty, logs to stderr only.
// Returns a cleanup function to close the log file.
func Setup(level slog.Level, logFile string) (cleanup func(), err error) {
	var writers []io.Writer
	writers = append(writers, os.Stderr)

	var file *os.File
	if logFile != "" {
		dir := filepath.Dir(logFile)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}
		file, err = os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, err
		}
		writers = append(writers, file)
	}

	multi := io.MultiWriter(writers...)
	handler := slog.NewJSONHandler(multi, &slog.HandlerOptions{
		Level: level,
	})
	logger := slog.New(handler)
	slog.SetDefault(logger)

	cleanup = func() {
		if file != nil {
			file.Close()
		}
	}
	return cleanup, nil
}

// ParseLevel converts a string level name to slog.Level.
// Supports: "debug", "info", "warn", "warning", "error". Defaults to info.
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
