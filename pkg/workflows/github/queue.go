package github

import (
	"time"

	"github.com/containifyci/dunebot/pkg/review"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const PullRequestReviewSignal = "pull_request_review"

// QueueWorkflow processes tasks one by one.
func PullRequestQueueWorkflow(ctx workflow.Context) error {
	var a PullRequestReviewActivities
	logger := workflow.GetLogger(ctx)
	logger.Info("Started queue workflow")

	// Signal channel for incoming tasks
	signalCh := workflow.GetSignalChannel(ctx, PullRequestReviewSignal)
	var taskQueue []review.PullRequestReview

	// Idle timeout duration
	idleTimeout := 1 * time.Minute

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
		// timerCtx, cancelTimer := workflow.WithCancel(ctx)
		timerCtx, cancelTimer := workflow.WithCancel(ctx)
		timerFuture := workflow.NewTimer(timerCtx, idleTimeout)
		// Wait for a signal
		selector := workflow.NewSelector(timerCtx)
		selector.AddReceive(signalCh, func(c workflow.ReceiveChannel, more bool) {
			var task review.PullRequestReview
			c.Receive(ctx, &task)
			taskQueue = append(taskQueue, task)
			logger.Info("Received pull request", "pr", task.PullRequest.Number, "queueSize", len(taskQueue))
			// Reset idle timer since we received a pull request
			cancelTimer()
		})

		// Listen for timeout
		selector.AddFuture(timerFuture, func(f workflow.Future) {
			logger.Info("No pull request received within timeout, exiting workflow.")
		})

		// Wait for a task signal to arrive
		selector.Select(ctx)

		// If the timer fired (no tasks received), exit workflow
		if len(taskQueue) == 0 {
			logger.Info("Shutting down workflow due to inactivity.")
			cancelTimer()
			return nil
		}

		// Process tasks sequentially
		for len(taskQueue) > 0 {
			task := taskQueue[0]
			taskQueue = taskQueue[1:] // Dequeue

			logger.Info("Pull Request review started.", "pr", task.PullRequest.Number)

			var result string
			// Execute task with retry policy
			options := workflow.ActivityOptions{
				RetryPolicy: &temporal.RetryPolicy{
					InitialInterval:    30 * time.Second,
					BackoffCoefficient: 1.5,
					MaximumInterval:    10 * time.Minute,
					MaximumAttempts:    3,
				},
				StartToCloseTimeout: 15 * time.Minute,
			}
			actCtx := workflow.WithActivityOptions(ctx, options)
			err := workflow.ExecuteActivity(actCtx, a.PullRequestReviewActivity, task).Get(ctx, &result)
			if err != nil {
				logger.Error("Activity failed.", "Error", err)
			}

			logger.Info("Pull Request Queue workflow completed.", "result", result)
			logger.Info("Completed Pull Request task", "pr", task.PullRequest.Number, "remainingTasks", len(taskQueue))
		}

		logger.Info("No more pull request reviews, waiting for new")
	}
}
