package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"go.temporal.io/sdk/activity"
)

// GitCommitError is a custom error type for git commit failures
type GitCommitError struct {
	Err error
}

func (e *GitCommitError) Error() string {
	return fmt.Sprintf("git commit error: %v", e.Err)
}

// GitCheckoutBranchInputs contains parameters for checking out a git branch
type GitCheckoutBranchInputs struct {
	RepoPath   string
	BranchName string
	CreateNew  bool
}

// GitCheckoutBranch checks out a git branch, optionally creating a new one
func GitCheckoutBranch(ctx context.Context, i GitCheckoutBranchInputs) error {
	logger := activity.GetLogger(ctx)
	logger.Info("checking out git branch", "branch", i.BranchName, "createNew", i.CreateNew)

	args := []string{"checkout"}
	if i.CreateNew {
		args = append(args, "-b")
	}
	args = append(args, i.BranchName)

	cmd := exec.Command("git", args...)
	cmd.Dir = i.RepoPath

	output, err := cmd.CombinedOutput()
	logger.Info("git checkout command", "command", cmd.String(), "output", string(output))
	if err != nil {
		return fmt.Errorf("git checkout failed: %w\nOutput: %s", err, string(output))
	}
	return nil
}

// GitResetToMainInputs contains parameters for resetting git to main
type GitResetToMainInputs struct {
	RepoPath string
}

// GitResetToMain resets git state to clean main branch
func GitResetToMain(ctx context.Context, i GitResetToMainInputs) error {
	logger := activity.GetLogger(ctx)
	logger.Info("resetting git to main branch", "repoPath", i.RepoPath)

	// Checkout main branch
	cmd := exec.Command("git", "checkout", "main")
	cmd.Dir = i.RepoPath
	output, err := cmd.CombinedOutput()
	logger.Info("git checkout main", "output", string(output))
	if err != nil {
		return fmt.Errorf("failed to checkout main: %w\nOutput: %s", err, string(output))
	}

	// Hard reset to clean state
	cmd = exec.Command("git", "reset", "--hard", "HEAD")
	cmd.Dir = i.RepoPath
	output, err = cmd.CombinedOutput()
	logger.Info("git reset", "output", string(output))
	if err != nil {
		return fmt.Errorf("git reset failed: %w\nOutput: %s", err, string(output))
	}

	// Clean untracked files
	cmd = exec.Command("git", "clean", "-fd")
	cmd.Dir = i.RepoPath
	output, err = cmd.CombinedOutput()
	logger.Info("git clean", "output", string(output))
	if err != nil {
		return fmt.Errorf("git clean failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// CommitAndPushInputs contains parameters for committing and pushing changes
type CommitAndPushInputs struct {
	RepoPath   string
	CommitMsg  string
	BranchName string
}

// CommitAndPush stages all changes, commits, and pushes to the remote branch
func CommitAndPush(ctx context.Context, i CommitAndPushInputs) (string, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("running git commit and push")

	// git add --all
	cmd := exec.Command("git", "add", "--all")
	cmd.Dir = i.RepoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to run git add: %w %s", err, string(output))
	}

	// git status (for logging)
	cmd = exec.Command("git", "status")
	cmd.Dir = i.RepoPath
	statusOutput, _ := cmd.CombinedOutput()
	logger.Info("git status", "output", string(statusOutput))

	// git commit -m
	cmd = exec.Command("git", "commit", "-m", i.CommitMsg)
	cmd.Dir = i.RepoPath
	output, err = cmd.CombinedOutput()
	logger.Info("running git commit", "command", cmd.String(), "output", string(output))
	if err != nil {
		return "", &GitCommitError{Err: fmt.Errorf("failed to run git commit. error: %s %s", err, string(output))}
	}

	// git push
	cmd = exec.Command("git", "push", "origin", i.BranchName)
	cmd.Dir = i.RepoPath
	output, err = cmd.CombinedOutput()
	logger.Info("running git push", "command", cmd.String(), "output", string(output))
	if err != nil {
		return "", fmt.Errorf("failed to run git push. error: %s %s", err, string(output))
	}

	return "Git pushed", nil
}

// CloneRepoForUpgrade clones a repository for major upgrade processing
// This version supports GitHub authentication via GITHUB_TOKEN and GITHUB_ORGANIZATION
func CloneRepoForUpgrade(ctx context.Context, organization, repository, branchName string) (string, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("cloning repository for upgrade", "org", organization, "repo", repository, "branch", branchName)

	// Create temp directory
	tempDir, err := os.MkdirTemp("", repository)
	if err != nil {
		return "", err
	}

	githubToken := os.Getenv("GITHUB_TOKEN")

	// Configure git for cloning
	if githubToken != "" {
		gitConfigParam := fmt.Sprintf("'url.https://oauth:%s@github.com/%s/.insteadOf=https://github.com/%s/'", githubToken, organization, organization)
		err = os.Setenv("GIT_CONFIG_PARAMETERS", gitConfigParam)
		if err != nil {
			return "", fmt.Errorf("failed to set environment variable: %w", err)
		}
	}

	// Clone repo
	repoURL := fmt.Sprintf("https://github.com/%s/%s.git", organization, repository)
	args := []string{"clone", "--depth", "1", repoURL, tempDir}
	if branchName != "" {
		args = []string{"clone", "--depth", "1", "--single-branch", "-b", branchName, repoURL, tempDir}
	}

	cmd := exec.Command("git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return "", fmt.Errorf("failed to clone repository: %w\nOutput: %s", err, string(output))
	}

	// Configure git local config
	if githubToken != "" {
		gitConfigURL := fmt.Sprintf("https://oauth:%s@github.com/%s/", githubToken, organization)
		gitConfigCommands := [][]string{
			{"config", "--local", "url." + gitConfigURL + ".insteadOf", "https://github.com/" + organization + "/"},
			{"config", "--local", "user.name", "'Go Major Upgrade Bot'"},
			{"config", "--local", "user.email", "'ci@containifyci.com'"},
		}
		for _, command := range gitConfigCommands {
			cmd := exec.Command("git", command...)
			cmd.Dir = tempDir
			output, err := cmd.CombinedOutput()
			if err != nil {
				_ = os.RemoveAll(tempDir)
				return "", fmt.Errorf("failed to run git config. error: %s", string(output))
			}
		}
	}

	return tempDir, nil
}

// CheckGitHasChanges checks if there are uncommitted changes in the working directory
func CheckGitHasChanges(ctx context.Context, workDir string) bool {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = workDir
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) != ""
}