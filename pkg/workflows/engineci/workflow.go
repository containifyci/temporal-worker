package engineci

import (
	"time"

	"github.com/containifyci/temporal-worker/pkg/activities/filesystem"
	"github.com/containifyci/temporal-worker/pkg/activities/git"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// EngineCIRepoWorkflow processes Engine-CI jobs for a single repository sequentially
// It uses signals to queue jobs and exits after an idle timeout
func EngineCIRepoWorkflow(ctx workflow.Context) error {
	logger := workflow.GetLogger(ctx)
	logger.Info("Started Engine-CI queue workflow")

	// Signal channel for incoming jobs
	signalCh := workflow.GetSignalChannel(ctx, EngineCISignal)
	var jobQueue []EngineCIWorkflowInput

	// Global activity options
	ao := workflow.ActivityOptions{
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    30 * time.Second,
			BackoffCoefficient: 1.5,
			MaximumInterval:    10 * time.Minute,
			MaximumAttempts:    2,
		},
		StartToCloseTimeout: 45 * time.Minute,
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	for {
		// Setup a timer for the idle timeout
		timerCtx, cancelTimer := workflow.WithCancel(ctx)
		timerFuture := workflow.NewTimer(timerCtx, IdleTimeout)

		// Wait for a signal or timeout
		selector := workflow.NewSelector(timerCtx)
		selector.AddReceive(signalCh, func(c workflow.ReceiveChannel, more bool) {
			var job EngineCIWorkflowInput
			c.Receive(ctx, &job)
			jobQueue = append(jobQueue, job)
			logger.Info("Received Engine-CI job", "repo", job.RepoName, "queueSize", len(jobQueue))
			// Reset idle timer since we received a job
			cancelTimer()
		})

		// Listen for timeout
		selector.AddFuture(timerFuture, func(f workflow.Future) {
			logger.Info("No Engine-CI job received within timeout, exiting workflow.")
		})

		// Wait for a job signal to arrive
		selector.Select(ctx)

		// If the timer fired (no jobs received), exit workflow
		if len(jobQueue) == 0 {
			logger.Info("Shutting down workflow due to inactivity.")
			cancelTimer()
			return nil
		}

		// Process jobs sequentially
		for len(jobQueue) > 0 {
			job := jobQueue[0]
			jobQueue = jobQueue[1:] // Dequeue

			logger.Info("Engine-CI job started", "repo", job.RepoName, "ref", job.GitRef)

			// Per-job activity options with longer timeout
			jobOptions := workflow.ActivityOptions{
				RetryPolicy: &temporal.RetryPolicy{
					InitialInterval:    30 * time.Second,
					BackoffCoefficient: 1.5,
					MaximumInterval:    10 * time.Minute,
					MaximumAttempts:    3,
				},
				StartToCloseTimeout: 15 * time.Minute,
			}
			jobCtx := workflow.WithActivityOptions(ctx, jobOptions)

			// Step 1: Clone repository
			var workDir string
			targetDir := GetCloneDirectory(job.GitRepoURL)
			err := workflow.ExecuteActivity(jobCtx, git.CloneRepo, job.GitRepoURL, job.GitRef, targetDir).Get(ctx, &workDir)
			if err != nil {
				logger.Error("Git clone failed", "repo", job.RepoName, "error", err)
				continue
			}

			// Step 2: Run Engine-CI
			var details *EngineCIDetails
			err = workflow.ExecuteActivity(jobCtx, RunEngineCI, workDir, job.EngineArgs, job.Env).Get(ctx, &details)
			if err != nil {
				logger.Error("Engine-CI execution failed", "repo", job.RepoName, "error", err)
				// Don't cleanup on error - preserve directory for debugging
				continue
			}

			// Step 3: Cleanup if successful (exit code 0)
			if details.ExitCode == 0 {
				logger.Info("Engine-CI succeeded, cleaning up", "repo", job.RepoName)
				var cleanupErr error
				err = workflow.ExecuteActivity(jobCtx, filesystem.CleanupDirectory, workDir).Get(ctx, &cleanupErr)
				if err != nil {
					logger.Warn("Cleanup failed (non-critical)", "repo", job.RepoName, "error", err)
				}
			} else {
				logger.Error("Engine-CI failed, preserving directory for debugging",
					"repo", job.RepoName,
					"exitCode", details.ExitCode,
					"workDir", workDir,
					"last50Lines", details.Last50Lines)
			}

			logger.Info("Engine-CI job completed", "repo", job.RepoName, "remainingJobs", len(jobQueue))
		}

		logger.Info("No more Engine-CI jobs, waiting for new signals")
	}
}
