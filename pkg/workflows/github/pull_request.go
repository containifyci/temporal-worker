package github

import (
	"context"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/gregjones/httpcache"
	"github.com/palantir/go-githubapp/githubapp"

	"github.com/containifyci/dunebot/pkg/config"
	"github.com/containifyci/dunebot/pkg/review"
)

type (
	PullRequestReviewActivities struct {
		Config      config.Config
		CC          githubapp.ClientCreator
	}
)

func NewClientCreator(cfg *config.Config) githubapp.ClientCreator {
	cc, err := githubapp.NewDefaultCachingClientCreator(
		//TODO maybe find an better way then to copy the config
		cfg.Github.ToGithubAppConfig(),
		githubapp.WithClientUserAgent(cfg.AppConfig.UserAgent),
		githubapp.WithClientTimeout(cfg.AppConfig.ClientTimeOutDuration()),
		githubapp.WithClientCaching(false, func() httpcache.Cache { return httpcache.NewMemoryCache() }),
		// githubapp.WithClientMiddleware(
		// 	githubapp.ClientMetrics(metricsRegistry),
		// ),
	)
	if err != nil {
		panic(err)
	}
	return cc
}

// Workflow is a Hello World workflow definition.
func PullRequestReviewWorkflow(ctx workflow.Context, input review.PullRequestReview) (string, error) {
	var a PullRequestReviewActivities
	ao := workflow.ActivityOptions{
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    30 * time.Second,
			BackoffCoefficient: 1.5,
			MaximumInterval:    5 * time.Minute,
			MaximumAttempts:    5,
		},
		StartToCloseTimeout: 10 * time.Minute,
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	logger := workflow.GetLogger(ctx)
	logger.Info("PullRequestReviewWorkflow workflow started", "pull_request", input)

	var result string
	err := workflow.ExecuteActivity(ctx, a.PullRequestReviewActivity, input).Get(ctx, &result)
	if err != nil {
		logger.Error("Activity failed.", "Error", err)
		return "", err
	}

	logger.Info("HelloWorld workflow completed.", "result", result)

	return result, nil
}

func (a PullRequestReviewActivities) PullRequestReviewActivity(ctx context.Context, input review.PullRequestReview) (string, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("PullRequestReviewActivity started", "pull_request", input)

	err := review.NewReviewer(logger, nil, a.CC, a.Config).PullRequestReview(ctx, input)

	if err != nil {
		logger.Error("Reviewer failed", "Error", err)
		return "", err
	}

	return "review successful", nil
}
