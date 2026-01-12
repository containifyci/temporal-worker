package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/containifyci/temporal-worker/pkg/workflows/engineci"
	"github.com/containifyci/temporal-worker/pkg/workflows/github"
	"go.temporal.io/sdk/client"
)

type arrayFlags []string

func (a *arrayFlags) String() string {
	return strings.Join(*a, ",")
}

func (a *arrayFlags) Set(value string) error {
	*a = append(*a, value)
	return nil
}

func main() {
	// Define flags
	var (
		githubPR  bool
		engineCI  bool
		repo      string
		ref       string
		argsStr   string
		envFlags  arrayFlags
	)

	flag.BoolVar(&githubPR, "github-pr", false, "Run GitHub PR workflow mode")
	flag.BoolVar(&engineCI, "engine-ci", false, "Run Engine-CI workflow mode")
	flag.StringVar(&repo, "repo", "", "Git repository URL (for Engine-CI mode)")
	flag.StringVar(&ref, "ref", "main", "Git reference/branch (for Engine-CI mode)")
	flag.StringVar(&argsStr, "args", "run,-t,all", "Comma-separated Engine-CI arguments (for Engine-CI mode)")
	flag.Var(&envFlags, "env", "Environment variables in key=value format (repeatable, for Engine-CI mode)")

	flag.Parse()

	// Create Temporal client
	c, err := client.Dial(client.Options{})
	if err != nil {
		log.Fatalln("Unable to create client", err)
	}
	defer c.Close()

	// Determine mode
	if engineCI {
		runEngineCIMode(c, repo, ref, argsStr, envFlags)
	} else if githubPR {
		runGitHubPRMode(c)
	} else {
		log.Println("No mode specified. Use --github-pr or --engine-ci")
		flag.Usage()
	}
}

func runEngineCIMode(c client.Client, repo, ref, argsStr string, envFlags arrayFlags) {
	if repo == "" {
		log.Fatalln("--repo is required for Engine-CI mode")
	}

	// Parse args
	args := strings.Split(argsStr, ",")

	// Parse environment variables
	env := make(map[string]string)
	for _, e := range envFlags {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			env[parts[0]] = parts[1]
		}
	}

	// Extract repo name for workflow ID
	repoName := engineci.SanitizeRepoName(repo)
	workflowID := fmt.Sprintf("engine-ci-%s", repoName)

	// Create workflow input
	input := engineci.EngineCIWorkflowInput{
		GitRepoURL: repo,
		GitRef:     ref,
		RepoName:   repoName,
		EngineArgs: args,
		Env:        env,
	}

	// Start or signal workflow
	we, err := c.SignalWithStartWorkflow(
		context.Background(),
		workflowID,
		engineci.EngineCISignal,
		input,
		client.StartWorkflowOptions{
			ID:        workflowID,
			TaskQueue: "engine-ci-queue",
		},
		engineci.EngineCIRepoWorkflow,
	)
	if err != nil {
		log.Fatalln("Unable to start or signal workflow", err)
	}

	log.Printf("Engine-CI workflow started/signaled: WorkflowID=%s, RunID=%s", we.GetID(), we.GetRunID())
}

func runGitHubPRMode(c client.Client) {
	workflowOptions := client.StartWorkflowOptions{
		ID:        "queue_workflowID",
		TaskQueue: "hello-world",
	}

	_, err := c.SignalWithStartWorkflow(
		context.Background(),
		workflowOptions.ID,
		github.PullRequestReviewSignal,
		"pr 1",
		workflowOptions,
		github.PullRequestQueueWorkflow,
	)
	if err != nil {
		log.Fatalln("Unable to execute workflow", err)
	}

	log.Println("GitHub PR workflow started")
}
