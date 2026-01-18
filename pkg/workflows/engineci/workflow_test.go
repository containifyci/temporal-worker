package engineci

import (
	"testing"
	"time"

	"github.com/containifyci/temporal-worker/pkg/activities/filesystem"
	"github.com/containifyci/temporal-worker/pkg/activities/git"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/sdk/testsuite"
)

type WorkflowTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite
}

func TestWorkflowTestSuite(t *testing.T) {
	suite.Run(t, new(WorkflowTestSuite))
}

func (s *WorkflowTestSuite) TestEngineCIRepoWorkflow_SingleJob() {
	env := s.NewTestWorkflowEnvironment()

	// Mock activities
	env.OnActivity(git.CloneRepo, mock.Anything, "https://github.com/test/repo", "main", "/tmp/ci-repo").
		Return("/tmp/ci-repo", nil)
	env.OnActivity(RunEngineCI, mock.Anything, "/tmp/ci-repo", []string{"run", "-t", "all"}, map[string]string{}).
		Return(&EngineCIDetails{ExitCode: 0, Last50Lines: "Success"}, nil)
	env.OnActivity(filesystem.CleanupDirectory, mock.Anything, "/tmp/ci-repo").
		Return(nil)

	// Register workflow
	env.RegisterWorkflow(EngineCIRepoWorkflow)

	// Start workflow
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(EngineCISignal, EngineCIWorkflowInput{
			GitRepoURL: "https://github.com/test/repo",
			GitRef:     "main",
			RepoName:   "repo",
			EngineArgs: []string{"run", "-t", "all"},
			Env:        map[string]string{},
		})
	}, 100*time.Millisecond)

	env.ExecuteWorkflow(EngineCIRepoWorkflow)

	s.True(env.IsWorkflowCompleted())
	s.NoError(env.GetWorkflowError())
}

func (s *WorkflowTestSuite) TestEngineCIRepoWorkflow_MultipleJobs() {
	env := s.NewTestWorkflowEnvironment()

	// Mock activities for multiple jobs
	env.OnActivity(git.CloneRepo, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return("/tmp/ci-repo", nil).Times(2)
	env.OnActivity(RunEngineCI, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&EngineCIDetails{ExitCode: 0, Last50Lines: "Success"}, nil).Times(2)
	env.OnActivity(filesystem.CleanupDirectory, mock.Anything, mock.Anything).
		Return(nil).Times(2)

	// Register workflow
	env.RegisterWorkflow(EngineCIRepoWorkflow)

	// Send two signals
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(EngineCISignal, EngineCIWorkflowInput{
			GitRepoURL: "https://github.com/test/repo",
			GitRef:     "main",
			RepoName:   "repo",
			EngineArgs: []string{"run", "-t", "all"},
			Env:        map[string]string{},
		})
	}, 100*time.Millisecond)

	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(EngineCISignal, EngineCIWorkflowInput{
			GitRepoURL: "https://github.com/test/repo",
			GitRef:     "feature",
			RepoName:   "repo",
			EngineArgs: []string{"run", "-t", "test"},
			Env:        map[string]string{},
		})
	}, 200*time.Millisecond)

	env.ExecuteWorkflow(EngineCIRepoWorkflow)

	s.True(env.IsWorkflowCompleted())
	s.NoError(env.GetWorkflowError())
}

func (s *WorkflowTestSuite) TestEngineCIRepoWorkflow_FailedJobNoCleanup() {
	env := s.NewTestWorkflowEnvironment()

	// Mock activities - RunEngineCI returns non-zero exit code
	env.OnActivity(git.CloneRepo, mock.Anything, "https://github.com/test/repo", "main", "/tmp/ci-repo").
		Return("/tmp/ci-repo", nil)
	env.OnActivity(RunEngineCI, mock.Anything, "/tmp/ci-repo", []string{"run", "-t", "all"}, map[string]string{}).
		Return(&EngineCIDetails{ExitCode: 1, Last50Lines: "Build failed"}, nil)
	// CleanupDirectory should NOT be called when job fails
	env.OnActivity(filesystem.CleanupDirectory, mock.Anything, mock.Anything).
		Return(nil).Times(0)

	// Register workflow
	env.RegisterWorkflow(EngineCIRepoWorkflow)

	// Start workflow
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(EngineCISignal, EngineCIWorkflowInput{
			GitRepoURL: "https://github.com/test/repo",
			GitRef:     "main",
			RepoName:   "repo",
			EngineArgs: []string{"run", "-t", "all"},
			Env:        map[string]string{},
		})
	}, 100*time.Millisecond)

	env.ExecuteWorkflow(EngineCIRepoWorkflow)

	s.True(env.IsWorkflowCompleted())
	s.NoError(env.GetWorkflowError())
}

func (s *WorkflowTestSuite) TestEngineCIRepoWorkflow_IdleTimeout() {
	env := s.NewTestWorkflowEnvironment()

	// Register workflow
	env.RegisterWorkflow(EngineCIRepoWorkflow)

	// Don't send any signals - workflow should timeout after IdleTimeout
	env.ExecuteWorkflow(EngineCIRepoWorkflow)

	s.True(env.IsWorkflowCompleted())
	s.NoError(env.GetWorkflowError())
}
