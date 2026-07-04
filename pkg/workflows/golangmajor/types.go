package golangmajor

import (
	"os"
)

// GoMajorSweepWorkflowInputs contains the parameters for the sweep workflow
type GoMajorSweepWorkflowInputs struct {
	Organization   string
	Language       string
	MaxConcurrency int
	DryRun         bool
}

// GoMajorSweepWorkflowOutputs contains the results of the sweep workflow
type GoMajorSweepWorkflowOutputs struct {
	TotalReposProcessed int
	TotalPRsCreated     int
	FailedRepos         []string
	SkippedRepos        []SkippedRepo
	Errors              []string
}

// SkippedRepo contains information about a skipped repository
type SkippedRepo struct {
	Repository string
	Reason     string
}

// GoMajorUpgradeRepoWorkflowInputs contains the parameters for upgrading a single repository
type GoMajorUpgradeRepoWorkflowInputs struct {
	Repository            string
	Organization          string
	OpenPullRequestsLimit int    // Maximum number of concurrent open major upgrade PRs (default: 5)
	Directory             string // Directory containing go.mod (e.g., "/", "/services/api")
	CommitMessagePrefix   string // Prefix for commit messages (e.g., "[PF-291]")
	DependencyModule      string // Optional: filter to upgrade only dependencies whose current (pre-upgrade) module name matches this value
}

// GoMajorUpgradeRepoWorkflowOutputs contains the results of upgrading a single repository
type GoMajorUpgradeRepoWorkflowOutputs struct {
	Repository   string
	PRsCreated   []string
	UpgradeCount int
	Error        string
}

// Defaults sets default values for GoMajorSweepWorkflowInputs
func (i *GoMajorSweepWorkflowInputs) Defaults() {
	if i.Organization == "" {
		i.Organization = os.Getenv("GITHUB_ORGANIZATION")
		if i.Organization == "" {
			i.Organization = "containifyci"
		}
	}
	if i.Language == "" {
		i.Language = "Go"
	}
	if i.MaxConcurrency == 0 {
		i.MaxConcurrency = 10
	}
}

// Defaults sets default values for GoMajorUpgradeRepoWorkflowInputs
func (i *GoMajorUpgradeRepoWorkflowInputs) Defaults() {
	if i.Organization == "" {
		i.Organization = os.Getenv("GITHUB_ORGANIZATION")
		if i.Organization == "" {
			i.Organization = "containifyci"
		}
	}
	if i.OpenPullRequestsLimit == 0 {
		i.OpenPullRequestsLimit = 5
	}
	if i.Directory == "" {
		i.Directory = "/"
	}
}