package filesystem

import (
	"context"
	"os"

	"go.temporal.io/sdk/activity"
)

// CleanupDirectory removes the specified directory
// This is a generic activity that can be used by any workflow that needs to clean up directories
func CleanupDirectory(ctx context.Context, dirPath string) error {
	logger := activity.GetLogger(ctx)
	logger.Info("CleanupDirectory started", "dirPath", dirPath)

	if err := os.RemoveAll(dirPath); err != nil {
		logger.Warn("Cleanup failed (non-critical)", "dirPath", dirPath, "error", err)
		return err
	}

	logger.Info("Cleanup successful", "dirPath", dirPath)
	return nil
}
