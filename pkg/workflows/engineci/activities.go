package engineci

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/log"
)

// logWriter writes to logger and buffers output
type logWriter struct {
	logger log.Logger
	prefix string
	buffer *bytes.Buffer
}

func (w *logWriter) Write(p []byte) (n int, err error) {
	// Log the output in real-time
	w.logger.Info(w.prefix, "output", string(p))

	// Also buffer it for Last50Lines
	return w.buffer.Write(p)
}

// RunEngineCI executes the engine-ci binary in the specified working directory
func RunEngineCI(ctx context.Context, workDir string, args []string, env map[string]string) (*EngineCIDetails, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("RunEngineCI started", "workDir", workDir, "args", args)

	// Build command
	cmd := exec.Command("engine-ci", args...)
	cmd.Dir = workDir

	// Set environment variables
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Create buffer and logger writer for real-time output streaming
	var outputBuf bytes.Buffer
	writer := &logWriter{
		logger: logger,
		prefix: "[engine-ci]",
		buffer: &outputBuf,
	}

	// Set stdout and stderr to our custom writer
	cmd.Stdout = writer
	cmd.Stderr = writer

	// Execute and capture output (streams in real-time)
	err := cmd.Run()
	outStr := outputBuf.String()

	// Determine exit code
	exitCode := 0
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		} else {
			// Command failed to execute (binary not found, permission denied, etc.)
			return nil, fmt.Errorf("failed to execute engine-ci: %w", err)
		}
	}

	// Extract last 50 lines
	last50 := outStr
	lines := strings.Split(outStr, "\n")
	if len(lines) > 50 {
		last50 = strings.Join(lines[len(lines)-50:], "\n")
	}

	details := &EngineCIDetails{
		ExitCode:    exitCode,
		Last50Lines: last50,
	}

	if exitCode != 0 {
		logger.Error("Engine-CI execution failed", "exitCode", exitCode, "output", last50)
	} else {
		logger.Info("Engine-CI execution successful")
	}

	return details, nil
}
