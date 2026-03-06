package logging

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestSetup_StderrOnly(t *testing.T) {
	cleanup, err := Setup(slog.LevelInfo, "")
	if err != nil {
		t.Fatalf("Setup returned error: %v", err)
	}
	defer cleanup()

	// Verify logging works without panic.
	slog.Info("test message from TestSetup_StderrOnly")
}

func TestSetup_WithFile(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "test.log")

	cleanup, err := Setup(slog.LevelInfo, logFile)
	if err != nil {
		t.Fatalf("Setup returned error: %v", err)
	}
	defer cleanup()

	slog.Info("hello from test", "key", "value")

	// Flush is implicit since slog writes synchronously.
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	content := string(data)
	if len(content) == 0 {
		t.Fatal("log file is empty; expected log output")
	}
	if !contains(content, "hello from test") {
		t.Errorf("log file does not contain expected message; got: %s", content)
	}
	if !contains(content, `"key":"value"`) {
		t.Errorf("log file does not contain expected key-value pair; got: %s", content)
	}
}

func TestSetup_Cleanup(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "cleanup.log")

	cleanup, err := Setup(slog.LevelInfo, logFile)
	if err != nil {
		t.Fatalf("Setup returned error: %v", err)
	}

	// Write something so the file is opened.
	slog.Info("before cleanup")

	// Call cleanup to close the file.
	cleanup()

	// After cleanup, writing to the file-backed logger may fail silently
	// (MultiWriter swallows errors), but the file should be closed.
	// Verify the file existed and was written to before cleanup.
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("log file is empty; expected content before cleanup")
	}
}

func TestSetup_CreateDir(t *testing.T) {
	base := t.TempDir()
	nestedDir := filepath.Join(base, "a", "b", "c")
	logFile := filepath.Join(nestedDir, "app.log")

	cleanup, err := Setup(slog.LevelInfo, logFile)
	if err != nil {
		t.Fatalf("Setup returned error: %v", err)
	}
	defer cleanup()

	// Verify the directory was created.
	info, err := os.Stat(nestedDir)
	if err != nil {
		t.Fatalf("directory was not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected %s to be a directory", nestedDir)
	}

	// Verify logging works.
	slog.Info("nested dir test")

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("log file is empty after logging")
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"Debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"INFO", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"WARN", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"WARNING", slog.LevelWarn},
		{"error", slog.LevelError},
		{"ERROR", slog.LevelError},
		{"unknown", slog.LevelInfo},
		{"", slog.LevelInfo},
		{"trace", slog.LevelInfo},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := ParseLevel(tc.input)
			if got != tc.expected {
				t.Errorf("ParseLevel(%q) = %v; want %v", tc.input, got, tc.expected)
			}
		})
	}
}

// contains is a small helper to check substring presence.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
