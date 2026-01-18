package main

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/log"
	"go.temporal.io/sdk/worker"

	"github.com/dusted-go/logging/prettylog"

	"github.com/containifyci/go-self-update/pkg/systemd"
	"github.com/containifyci/go-self-update/pkg/updater"
	"github.com/containifyci/temporal-worker/pkg/activities/filesystem"
	"github.com/containifyci/temporal-worker/pkg/activities/git"
	"github.com/containifyci/temporal-worker/pkg/workflows/engineci"
)

var (
	version          = "dev"
	commit           = "none"
	date             = "unknown"
	temporalHostPort = os.Getenv("TEMPORAL_HOST")
)

const (
	// Engine-CI specific queue name
	engineCIQueue = "engine-ci-queue"

	// Concurrency limits for Engine-CI workflows
	maxConcurrentWorkflows  = 2
	maxConcurrentActivities = 4
)

func main() {
	fmt.Printf("temporal-worker-engine-ci %s, commit %s, built at %s\n", version, commit, date)
	// Check for command-line arguments
	command := "start"
	if len(os.Args) >= 2 {
		command = os.Args[1]
	}

	// Get the command
	switch command {
	case "update":
		u := updater.NewUpdater(
			"temporal-worker-engine-ci", "containifyci", "temporal-worker", version,
			updater.WithUpdateHook(systemd.SystemdRestartHook("temporal-worker-engine-ci")),
		)
		updated, err := u.SelfUpdate()
		if err != nil {
			fmt.Printf("Update failed %+v\n", err)
		}
		if updated {
			fmt.Println("Update completed successfully!")
			return
		}
		fmt.Println("Already up-to-date")
	default:
		start()
	}
}

// downloadEngineCIIfNeeded downloads engine-ci from GitHub releases if it's not in PATH
func downloadEngineCIIfNeeded(logger *slog.Logger) error {
	// Check if engine-ci is already available
	if _, err := exec.LookPath("engine-ci"); err == nil {
		return nil // Already available
	}

	logger.Info("engine-ci not found in PATH, attempting to download from GitHub releases")

	// Determine OS and architecture
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	// Map Go architecture names to engine-ci binary names
	archMap := map[string]string{
		"amd64": "x86_64",
		"arm64": "arm64",
		"386":   "i386",
	}

	binaryArch, ok := archMap[goarch]
	if !ok {
		return fmt.Errorf("unsupported architecture: %s", goarch)
	}

	// Construct download URL (following GitHub releases naming convention)
	// Example: https://github.com/containifyci/engine-ci/releases/latest/download/engine-ci_linux_x86_64
	binaryName := fmt.Sprintf("engine-ci_%s_%s", goos, binaryArch)
	downloadURL := fmt.Sprintf("https://github.com/containifyci/engine-ci/releases/latest/download/%s", binaryName)

	logger.Info("Downloading engine-ci", "url", downloadURL)

	// Create local bin directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	localBinDir := filepath.Join(homeDir, ".local", "bin")
	if err := os.MkdirAll(localBinDir, 0755); err != nil {
		return fmt.Errorf("failed to create local bin directory: %w", err)
	}

	// Download the binary
	resp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("failed to download engine-ci: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download engine-ci: HTTP %d", resp.StatusCode)
	}

	// Save to local bin directory
	engineCIPath := filepath.Join(localBinDir, "engine-ci")
	outFile, err := os.OpenFile(engineCIPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("failed to create engine-ci file: %w", err)
	}
	defer func() { _ = outFile.Close() }()

	if _, err := io.Copy(outFile, resp.Body); err != nil {
		return fmt.Errorf("failed to write engine-ci binary: %w", err)
	}

	logger.Info("Successfully downloaded engine-ci", "path", engineCIPath)

	// Add to PATH for current process
	currentPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", fmt.Sprintf("%s%c%s", localBinDir, os.PathListSeparator, currentPath)); err != nil {
		return fmt.Errorf("failed to set PATH: %w", err)
	}

	logger.Info("Added to PATH", "directory", localBinDir)

	return nil
}

// checkRequiredTools verifies that required tools are available and prints their versions
func checkRequiredTools(logger *slog.Logger) error {
	tools := []string{"git", "engine-ci"}
	for _, tool := range tools {
		path, err := exec.LookPath(tool)
		if err != nil {
			return fmt.Errorf("%s not found in PATH", tool)
		}

		// Get version
		cmd := exec.Command(tool, "--version")
		output, err := cmd.CombinedOutput()
		if err != nil {
			logger.Warn("Could not get version", "tool", tool, "error", err)
		} else {
			logger.Info("Tool ready", "tool", tool, "path", path, "version", strings.TrimSpace(string(output)))
		}
	}
	return nil
}

func start() {
	logOpts := slog.HandlerOptions{
		Level:       slog.LevelDebug,
		AddSource:   true,
		ReplaceAttr: nil,
	}

	prettyHandler := prettylog.NewHandler(&logOpts)
	logger := slog.New(prettyHandler)
	slog.SetDefault(logger)

	logger.Info("Starting Engine-CI Worker", "queue", engineCIQueue)

	// Download engine-ci if not available
	if err := downloadEngineCIIfNeeded(logger); err != nil {
		logger.Warn("Failed to auto-download engine-ci", "error", err)
		// Continue anyway - checkRequiredTools will fail if it's truly missing
	}

	// Check required tools before starting worker
	if err := checkRequiredTools(logger); err != nil {
		logger.Error("Required tools missing", "error", err)
		os.Exit(1)
	}

	if temporalHostPort == "" {
		temporalHostPort = "localhost:7233"
	}

	// Create Temporal client
	c, err := client.Dial(client.Options{
		Logger:   log.NewStructuredLogger(logger),
		HostPort: temporalHostPort,
	})
	if err != nil {
		logger.Error("Unable to create client", "error", err)
		os.Exit(1)
	}
	defer c.Close()

	// Create worker with Engine-CI specific settings
	w := worker.New(c, engineCIQueue, worker.Options{
		MaxConcurrentWorkflowTaskExecutionSize: maxConcurrentWorkflows,
		MaxConcurrentActivityExecutionSize:     maxConcurrentActivities,
		StickyScheduleToStartTimeout:           10 * time.Minute,
	})

	logger.Info("Worker configuration",
		"maxConcurrentWorkflows", maxConcurrentWorkflows,
		"maxConcurrentActivities", maxConcurrentActivities,
		"stickyExecutionTimeout", "10m")

	// Register Engine-CI workflows and activities
	w.RegisterWorkflow(engineci.EngineCIRepoWorkflow)
	w.RegisterActivity(git.CloneRepo)
	w.RegisterActivity(engineci.RunEngineCI)
	w.RegisterActivity(filesystem.CleanupDirectory)

	logger.Info("Registered Engine-CI workflows and activities")

	// Start worker
	logger.Info("Engine-CI Worker started successfully")
	err = w.Run(worker.InterruptCh())
	if err != nil {
		logger.Error("Unable to start worker", "error", err)
	}
}
