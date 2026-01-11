package engineci

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/containifyci/temporal-worker/pkg/activities/filesystem"
	"github.com/containifyci/temporal-worker/pkg/activities/git"

	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/worker"
)

func TestEngineCIWorkflow_E2E_SingleJob(t *testing.T) {
	IdleTimeout = 5 * time.Second
	t.Cleanup(func() {
		IdleTimeout = 1 * time.Minute
	})
	// Skip if engine-ci or git not available
	if _, err := exec.LookPath("engine-ci"); err != nil {
		t.Skip("engine-ci not found in PATH, skipping e2e test")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH, skipping e2e test")
	}

	// Start embedded Temporal dev server
	// "" will let use a random port in local env
	hostPort := ""
	server, err := testsuite.StartDevServer(context.Background(), testsuite.DevServerOptions{
		ClientOptions: &client.Options{HostPort: hostPort},
	})
	require.NoError(t, err)
	require.NotNil(t, server)

	var (
		c       client.Client
		w       worker.Worker
		wInChan <-chan interface{}
	)

	taskQ := "hello-world"

	// Start worker in background
	ch := make(chan interface{})
	go func() {
		c = server.Client()
		w = worker.New(c, taskQ, worker.Options{
			MaxConcurrentWorkflowTaskExecutionSize: 2,
			MaxConcurrentActivityExecutionSize:     4,
			StickyScheduleToStartTimeout:           10 * time.Minute,
		})
		wInChan = worker.InterruptCh()

		ch <- struct{}{}

		_ = w.Run(wInChan)
	}()

	<-ch

	require.NotNil(t, c)
	require.NotNil(t, w)
	require.NotNil(t, wInChan)

	// Register workflows and activities
	w.RegisterWorkflow(EngineCIRepoWorkflow)
	w.RegisterActivity(git.CloneRepo)
	w.RegisterActivity(RunEngineCI)
	w.RegisterActivity(filesystem.CleanupDirectory)

	// Prepare test input - use this repo for testing
	input := EngineCIWorkflowInput{
		GitRepoURL: "https://github.com/containifyci/go-self-update",
		GitRef:     "main",
		RepoName:   "go-self-update",
		EngineArgs: []string{"run", "-t", "all"},
		Env:        map[string]string{"CI": "true"},
	}

	workflowID := "test-engine-ci-go-self-update"

	// Start workflow with signal
	we, err := c.SignalWithStartWorkflow(
		context.Background(),
		workflowID,
		EngineCISignal,
		input,
		client.StartWorkflowOptions{
			ID:        workflowID,
			TaskQueue: taskQ,
		},
		EngineCIRepoWorkflow,
	)
	require.NoError(t, err)
	require.NotNil(t, we)

	// Wait for workflow completion with timeout
	// The workflow will exit after idle timeout (1 minute)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	err = we.Get(ctx, nil)
	// Workflow should complete without error (exits due to idle timeout)
	require.NoError(t, err)

	// Stop worker
	w.Stop()

	// Stop server
	err = server.Stop()
	require.NoError(t, err)
}

func TestEngineCIWorkflow_E2E_MultipleJobs(t *testing.T) {
	IdleTimeout = 5 * time.Second
	t.Cleanup(func() {
		IdleTimeout = 1 * time.Minute
	})
	// Skip if engine-ci or git not available
	if _, err := exec.LookPath("engine-ci"); err != nil {
		t.Skip("engine-ci not found in PATH, skipping e2e test")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH, skipping e2e test")
	}

	// Start embedded Temporal dev server
	hostPort := ""
	server, err := testsuite.StartDevServer(context.Background(), testsuite.DevServerOptions{
		ClientOptions: &client.Options{HostPort: hostPort},
	})
	require.NoError(t, err)
	require.NotNil(t, server)

	var (
		c       client.Client
		w       worker.Worker
		wInChan <-chan interface{}
	)

	taskQ := "hello-world"

	// Start worker in background
	ch := make(chan interface{})
	go func() {
		c = server.Client()
		w = worker.New(c, taskQ, worker.Options{
			MaxConcurrentWorkflowTaskExecutionSize: 2,
			MaxConcurrentActivityExecutionSize:     4,
			StickyScheduleToStartTimeout:           10 * time.Minute,
		})
		wInChan = worker.InterruptCh()

		ch <- struct{}{}

		_ = w.Run(wInChan)
	}()

	<-ch

	require.NotNil(t, c)
	require.NotNil(t, w)
	require.NotNil(t, wInChan)

	// Register workflows and activities
	w.RegisterWorkflow(EngineCIRepoWorkflow)
	w.RegisterActivity(git.CloneRepo)
	w.RegisterActivity(RunEngineCI)
	w.RegisterActivity(filesystem.CleanupDirectory)

	// Prepare first job
	input1 := EngineCIWorkflowInput{
		GitRepoURL: "https://github.com/containifyci/go-self-update",
		GitRef:     "main",
		RepoName:   "go-self-update",
		EngineArgs: []string{"run", "-t", "test"},
		Env:        map[string]string{},
	}

	workflowID := "test-engine-ci-multiple-jobs"

	// Start workflow with first signal
	we, err := c.SignalWithStartWorkflow(
		context.Background(),
		workflowID,
		EngineCISignal,
		input1,
		client.StartWorkflowOptions{
			ID:        workflowID,
			TaskQueue: taskQ,
		},
		EngineCIRepoWorkflow,
	)
	require.NoError(t, err)
	require.NotNil(t, we)

	// Wait a bit for first job to start
	time.Sleep(2 * time.Second)

	// Send second signal - should queue
	input2 := EngineCIWorkflowInput{
		GitRepoURL: "https://github.com/containifyci/go-self-update",
		GitRef:     "main",
		RepoName:   "go-self-update",
		EngineArgs: []string{"run", "-t", "lint"},
		Env:        map[string]string{},
	}

	err = c.SignalWorkflow(context.Background(), workflowID, "", EngineCISignal, input2)
	require.NoError(t, err)

	// Wait for both jobs to complete + idle timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	err = we.Get(ctx, nil)
	// Workflow should complete without error
	require.NoError(t, err)

	// Stop worker
	w.Stop()

	// Stop server
	err = server.Stop()
	require.NoError(t, err)
}
