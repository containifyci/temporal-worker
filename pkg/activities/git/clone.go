package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"go.temporal.io/sdk/activity"
)

// CloneRepo clones a git repository to the specified directory
// This is a generic activity that can be used by any workflow that needs to clone a git repository
func CloneRepo(ctx context.Context, repoURL, ref, workDir string) (string, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("CloneRepo started", "repo", repoURL, "ref", ref, "workDir", workDir)

	// Remove existing directory if it exists
	if _, err := os.Stat(workDir); err == nil {
		logger.Info("Removing existing clone directory", "workDir", workDir)
		if err := os.RemoveAll(workDir); err != nil {
			return "", fmt.Errorf("failed to remove existing directory: %w", err)
		}
	}

	// Execute git clone
	cmd := exec.Command("git", "clone", "--branch", ref, repoURL, workDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git clone failed: %v: %s", err, string(output))
	}

	logger.Info("Git clone successful", "workDir", workDir)
	return workDir, nil}
