package nautilus

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// NautilusProcess manages the NautilusTrader Python subprocess.
type NautilusProcess struct {
	mu         sync.Mutex
	cmd        *exec.Cmd
	pythonPath string
	port       int
	running    bool
	logger     *slog.Logger
}

// NewProcess creates a new process manager.
func NewProcess(pythonPath string, port int, logger *slog.Logger) *NautilusProcess {
	return &NautilusProcess{
		pythonPath: pythonPath,
		port:       port,
		logger:     logger,
	}
}

// Start launches the Nautilus gRPC server as a subprocess.
func (p *NautilusProcess) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return nil
	}

	p.cmd = exec.Command(p.pythonPath, "-m", "nautilus", "--port", fmt.Sprintf("%d", p.port))
	p.cmd.Dir = "."

	stdout, err := p.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := p.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := p.cmd.Start(); err != nil {
		return fmt.Errorf("start nautilus process: %w", err)
	}

	p.running = true
	go p.captureOutput("stdout", stdout)
	go p.captureOutput("stderr", stderr)

	p.logger.Info("nautilus process started", "pid", p.cmd.Process.Pid, "port", p.port)
	return nil
}

// Stop sends SIGTERM and waits up to 5 seconds, then SIGKILL.
func (p *NautilusProcess) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}

	p.logger.Info("stopping nautilus process", "pid", p.cmd.Process.Pid)

	// Send SIGTERM
	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		p.logger.Warn("failed to send SIGTERM", "error", err)
	}

	// Wait with timeout
	done := make(chan error, 1)
	go func() {
		done <- p.cmd.Wait()
	}()

	select {
	case <-done:
		p.running = false
		return nil
	case <-time.After(5 * time.Second):
		p.logger.Warn("nautilus process did not stop gracefully, sending SIGKILL")
		if err := p.cmd.Process.Kill(); err != nil {
			return fmt.Errorf("kill nautilus process: %w", err)
		}
		<-done
		p.running = false
		return nil
	}
}

// IsRunning returns whether the process is currently running.
func (p *NautilusProcess) IsRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.running
}

// Restart stops then starts the process with exponential backoff (max 3 retries).
func (p *NautilusProcess) Restart() error {
	_ = p.Stop()

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			p.logger.Info("retrying nautilus start", "attempt", attempt+1, "backoff", backoff)
			time.Sleep(backoff)
		}
		if err := p.Start(); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return fmt.Errorf("failed to restart after 3 attempts: %w", lastErr)
}

func (p *NautilusProcess) captureOutput(name string, r io.Reader) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			line := string(buf[:n])
			if name == "stderr" {
				fmt.Fprint(os.Stderr, line)
			}
			p.logger.Debug("nautilus", "stream", name, "output", line)
		}
		if err != nil {
			break
		}
	}
}
