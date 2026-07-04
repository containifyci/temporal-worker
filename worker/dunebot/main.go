package main

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/log"
	"go.temporal.io/sdk/worker"

	"github.com/dusted-go/logging/prettylog"

	"github.com/containifyci/dunebot/pkg/config"

	"github.com/containifyci/go-self-update/pkg/systemd"
	"github.com/containifyci/go-self-update/pkg/updater"
	gitactivity "github.com/containifyci/temporal-worker/pkg/activities/git"
	golangactivity "github.com/containifyci/temporal-worker/pkg/activities/golang"
	"github.com/containifyci/temporal-worker/pkg/helloworld"
	"github.com/containifyci/temporal-worker/pkg/workflows/github"
	golangmajor "github.com/containifyci/temporal-worker/pkg/workflows/golangmajor"
)

var (
	version          = "dev"
	commit           = "none"
	date             = "unknown"
	temporalHostPort = os.Getenv("TEMPORAL_HOST")
)

const (
	// DuneBot task queue name
	dunebotQueue = "dunebot"
)

func main() {
	fmt.Printf("temporal-worker %s, commit %s, built at %s\n", version, commit, date)
	// Check for command-line arguments
	command := "start"
	if len(os.Args) >= 2 {
		command = os.Args[1]
	}

	// Get the command
	switch command {
	case "update":
		u := updater.NewUpdater(
			"temporal-dunebot-worker", "containifyci", "temporal-worker", version,
			updater.WithUpdateHook(systemd.SystemdRestartHook("temporal-worker")),
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

func start() {
	logOpts := slog.HandlerOptions{
		Level:       slog.LevelDebug,
		AddSource:   true,
		ReplaceAttr: nil,
	}

	prettyHandler := prettylog.NewHandler(&logOpts)
	logger := slog.New(prettyHandler)
	slog.SetDefault(logger)

	if temporalHostPort == "" {
		temporalHostPort = "localhost:7233"
	}

	// The client and worker are heavyweight objects that should be created once per process.
	c, err := client.Dial(client.Options{
		Logger:   log.NewStructuredLogger(logger),
		HostPort: temporalHostPort,
	})
	if err != nil {
		logger.Error("Unable to create client", "error", err)
		os.Exit(1)
	}
	defer c.Close()

	w := worker.New(c, dunebotQueue, worker.Options{
		MaxConcurrentWorkflowTaskExecutionSize: 2,
		MaxConcurrentActivityExecutionSize:     4,
		EnableSessionWorker:                    true,
		StickyScheduleToStartTimeout:           10 * time.Minute,
	})

	//TODO set the needed DuneBot secret
	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed loading config", "error", err)
		os.Exit(1)
	}
	cfg.AppConfig = config.ApplicationConfig{
		ReviewerConfig: config.ReviewerConfig{
			Type: "direct",
		},
	}

	cc := github.NewClientCreator(cfg)

	// Register existing workflows and activities
	w.RegisterWorkflow(helloworld.Workflow)
	w.RegisterWorkflow(github.PullRequestQueueWorkflow)
	w.RegisterWorkflow(github.PullRequestReviewWorkflow)
	w.RegisterActivity(helloworld.Activity)
	w.RegisterActivity(github.PullRequestReviewActivities{
		CC:     cc,
		Config: *cfg,
	}.PullRequestReviewActivity)

	// Register Go Major Upgrade workflows and activities
	w.RegisterWorkflow(golangmajor.GoMajorSweepWorkflow)
	w.RegisterWorkflow(golangmajor.GoMajorUpgradeRepoWorkflow)
	w.RegisterActivity(golangactivity.SearchGoRepositories)
	w.RegisterActivity(golangactivity.FetchDependabotConfigFromGitHub)
	w.RegisterActivity(golangactivity.DetectMajorUpgrades)
	w.RegisterActivity(golangactivity.UpgradeDependency)
	w.RegisterActivity(golangactivity.CountOpenMajorUpgradePRs)
	w.RegisterActivity(golangactivity.CheckPRExistsForBranch)
	w.RegisterActivity(golangactivity.PRCreate)
	w.RegisterActivity(golangactivity.PRAddLabels)
	w.RegisterActivity(golangactivity.PRComment)
	w.RegisterActivity(gitactivity.CloneRepoForUpgrade)
	w.RegisterActivity(gitactivity.GitCheckoutBranch)
	w.RegisterActivity(gitactivity.CommitAndPush)
	w.RegisterActivity(gitactivity.GitResetToMain)

	logger.Info("Registered workflows and activities",
		"queue", dunebotQueue,
		"workflows", []string{"helloworld.Workflow", "github.PullRequestQueueWorkflow", "github.PullRequestReviewWorkflow", "golangmajor.GoMajorSweepWorkflow", "golangmajor.GoMajorUpgradeRepoWorkflow"},
	)

	err = w.Run(worker.InterruptCh())
	if err != nil {
		logger.Error("Unable to start worker", "error", err)
	}
}
