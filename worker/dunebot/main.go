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
	"github.com/containifyci/temporal-worker/pkg/helloworld"
	"github.com/containifyci/temporal-worker/pkg/workflows/github"
)

var (
	version          = "dev"
	commit           = "none"
	date             = "unknown"
	temporalHostPort = os.Getenv("TEMPORAL_HOST")
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
			"temporal-worker-dunebot", "containifyci", "temporal-worker", version,
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
	// logger := slog.New(prettyHandler)
	// logger := slog.New(slog.NewJSONHandler(prettyHandler, &slog.HandlerOptions{
	// 	Level: slog.LevelDebug,
	// }))
	slog.SetDefault(logger)

	if temporalHostPort == "" {
		temporalHostPort = "localhost:7233"
	}

	// The client and worker are heavyweight objects that should be created once per process.
	// c, err := client.Dial(client.Options{})
	c, err := client.Dial(client.Options{
		Logger:   log.NewStructuredLogger(logger),
		HostPort: temporalHostPort,
		// MetricsHandler: sdktally.NewMetricsHandler(newPrometheusScope(prometheus.Configuration{
		// 	ListenAddress: "0.0.0.0:8090",
		// 	TimerType:     "histogram",
		// })),
	})
	if err != nil {
		logger.Error("Unable to create client", "error", err)
		os.Exit(1)
	}
	defer c.Close()

	w := worker.New(c, "hello-world", worker.Options{
		MaxConcurrentWorkflowTaskExecutionSize: 2,
		MaxConcurrentActivityExecutionSize:     4,
		EnableSessionWorker:                    true,
		StickyScheduleToStartTimeout:           10 * time.Minute,
	})

	//TODO set the needed DuneBot secret
	cfg, err := config.Load()
	cfg.AppConfig = config.ApplicationConfig{
		ReviewerConfig: config.ReviewerConfig{
			Type: "direct",
		},
	}
	if err != nil {
		panic(err)
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

	err = w.Run(worker.InterruptCh())
	if err != nil {
		logger.Error("Unable to start worker", "error", err)
	}
}
