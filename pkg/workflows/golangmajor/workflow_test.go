package golangmajor

import (
	"testing"

	golangactivity "github.com/containifyci/temporal-worker/pkg/activities/golang"
	"github.com/stretchr/testify/assert"
)

func TestGoMajorSweepWorkflowInputsDefaults(t *testing.T) {
	inputs := GoMajorSweepWorkflowInputs{}
	inputs.Defaults()

	assert.Equal(t, "containifyci", inputs.Organization)
	assert.Equal(t, "Go", inputs.Language)
	assert.Equal(t, 10, inputs.MaxConcurrency)
	assert.False(t, inputs.DryRun)
}

func TestGoMajorSweepWorkflowInputsDefaults_WithValues(t *testing.T) {
	inputs := GoMajorSweepWorkflowInputs{
		Organization:   "custom-org",
		Language:       "Python",
		MaxConcurrency: 5,
		DryRun:         true,
	}
	inputs.Defaults()

	assert.Equal(t, "custom-org", inputs.Organization)
	assert.Equal(t, "Python", inputs.Language)
	assert.Equal(t, 5, inputs.MaxConcurrency)
	assert.True(t, inputs.DryRun)
}

func TestGoMajorUpgradeRepoWorkflowInputsDefaults(t *testing.T) {
	inputs := GoMajorUpgradeRepoWorkflowInputs{}
	inputs.Defaults()

	assert.Equal(t, "containifyci", inputs.Organization)
	assert.Equal(t, "/", inputs.Directory)
	assert.Equal(t, 5, inputs.OpenPullRequestsLimit)
}

func TestGoMajorUpgradeRepoWorkflowInputsDefaults_WithValues(t *testing.T) {
	inputs := GoMajorUpgradeRepoWorkflowInputs{
		Repository:            "test-repo",
		Organization:          "custom-org",
		OpenPullRequestsLimit: 10,
		Directory:             "/services/api",
		CommitMessagePrefix:   "[CUSTOM-123]",
	}
	inputs.Defaults()

	assert.Equal(t, "test-repo", inputs.Repository)
	assert.Equal(t, "custom-org", inputs.Organization)
	assert.Equal(t, 10, inputs.OpenPullRequestsLimit)
	assert.Equal(t, "/services/api", inputs.Directory)
	assert.Equal(t, "[CUSTOM-123]", inputs.CommitMessagePrefix)
}

func TestGoMajorUpgradeRepoWorkflowInputsDefaults_CommitMessagePrefix(t *testing.T) {
	// When CommitMessagePrefix is already set, it should not be overridden
	inputs := GoMajorUpgradeRepoWorkflowInputs{
		CommitMessagePrefix: "[PF-291]",
	}
	inputs.Defaults()

	assert.Equal(t, "[PF-291]", inputs.CommitMessagePrefix)
}

func TestGenerateBranchName(t *testing.T) {
	tests := []struct {
		name     string
		upgrade  golangactivity.MajorUpgrade
		expected string
	}{
		{
			name: "GitHub module",
			upgrade: golangactivity.MajorUpgrade{
				FromModule:  "github.com/gin-gonic/gin",
				ToModule:    "github.com/gin-gonic/gin/v2",
				FromVersion: "v1.9.0",
				ToVersion:   "v2.0.0",
			},
			expected: "dependabot/go_modules/major-888c6c87bd",
		},
		{
			name: "Module with /v2",
			upgrade: golangactivity.MajorUpgrade{
				FromModule:  "github.com/go-chi/chi/v2",
				ToModule:    "github.com/go-chi/chi/v3",
				FromVersion: "v2.0.0",
				ToVersion:   "v3.0.0",
			},
			expected: "dependabot/go_modules/major-a5b3aa91d9",
		},
		{
			name: "Golang.org module",
			upgrade: golangactivity.MajorUpgrade{
				FromModule:  "golang.org/x/mod",
				ToModule:    "golang.org/x/mod/v1",
				FromVersion: "v0.1.0",
				ToVersion:   "v1.0.0",
			},
			expected: "dependabot/go_modules/major-cef56ae71e",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateBranchName(tt.upgrade)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateDependabotStylePRBody(t *testing.T) {
	upgrade := golangactivity.MajorUpgrade{
		FromModule:   "github.com/gin-gonic/gin",
		ToModule:     "github.com/gin-gonic/gin/v2",
		FromVersion:  "v1.9.0",
		ToVersion:    "v2.0.0",
		ReleasesURL:  "https://github.com/gin-gonic/gin/releases",
		ChangelogURL: "https://github.com/gin-gonic/gin/blob/main/CHANGELOG.md",
		CompareURL:   "https://github.com/gin-gonic/gin/compare/v1.9.0...v2.0.0",
	}

	body := generateDependabotStylePRBody(upgrade, "test-repo")

	// Verify key components are in the body
	assert.Contains(t, body, "github.com/gin-gonic/gin")
	assert.Contains(t, body, "v1.9.0")
	assert.Contains(t, body, "v2.0.0")
	assert.Contains(t, body, "Major version upgrade")
	assert.Contains(t, body, "Release notes")
	assert.Contains(t, body, "Changelog")
	assert.Contains(t, body, "Compare versions")
	assert.Contains(t, body, "Breaking Changes")
	assert.Contains(t, body, "Actions Taken by Bot")
	assert.Contains(t, body, "Go Major Upgrade Workflow")
}

func TestSkippedRepo(t *testing.T) {
	skip := SkippedRepo{
		Repository: "my-repo",
		Reason:     "no dependabot config found",
	}
	assert.Equal(t, "my-repo", skip.Repository)
	assert.Equal(t, "no dependabot config found", skip.Reason)
}