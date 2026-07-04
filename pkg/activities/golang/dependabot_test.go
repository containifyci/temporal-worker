package golang

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
)

func TestParseDependabotConfig_Success(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()
	githubDir := filepath.Join(tmpDir, ".github")
	require.NoError(t, os.MkdirAll(githubDir, 0o755))

	// Create sample dependabot.yaml with multiple gomod entries
	dependabotYAML := `---
version: 2
updates:
  - package-ecosystem: "github-actions"
    directory: "/"
    schedule:
      interval: "weekly"
      day: "sunday"
    commit-message:
      prefix: "[PF-267]"
      include: "scope"
  - package-ecosystem: "gomod"
    directory: "."
    schedule:
      interval: "weekly"
      day: "sunday"
    commit-message:
      prefix: "[PF-291]"
      include: "scope"
  - package-ecosystem: "gomod"
    directory: "/services/api"
    schedule:
      interval: "weekly"
    commit-message:
      prefix: "[PF-123]"
      include: "scope"
  - package-ecosystem: "docker"
    directory: "/"
    schedule:
      interval: "weekly"
    commit-message:
      prefix: "[PF-282]"
`

	configPath := filepath.Join(githubDir, "dependabot.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(dependabotYAML), 0o644))

	// Parse using the local file function
	result, err := ParseDependabotConfig(tmpDir)

	require.NoError(t, err)
	assert.Len(t, result, 2)

	// First gomod entry (directory: "." normalized to "/")
	assert.Equal(t, "/", result[0].Directory)
	assert.Equal(t, "[PF-291]", result[0].CommitMessagePrefix)

	// Second gomod entry
	assert.Equal(t, "/services/api", result[1].Directory)
	assert.Equal(t, "[PF-123]", result[1].CommitMessagePrefix)
}

func TestParseDependabotConfig_YMLExtension(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()
	githubDir := filepath.Join(tmpDir, ".github")
	require.NoError(t, os.MkdirAll(githubDir, 0o755))

	// Create dependabot.yml (not .yaml)
	dependabotYML := `---
version: 2
updates:
  - package-ecosystem: "gomod"
    directory: "/"
    schedule:
      interval: "weekly"
    commit-message:
      prefix: "[PF-2]"
      include: "scope"
`

	configPath := filepath.Join(githubDir, "dependabot.yml")
	require.NoError(t, os.WriteFile(configPath, []byte(dependabotYML), 0o644))

	// Parse using the local file function
	result, err := ParseDependabotConfig(tmpDir)

	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "/", result[0].Directory)
	assert.Equal(t, "[PF-2]", result[0].CommitMessagePrefix)
}

func TestParseDependabotConfig_NoFile(t *testing.T) {
	// Create temporary directory without dependabot config
	tmpDir := t.TempDir()

	_, err := ParseDependabotConfig(tmpDir)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNoDependabotConfig)
}

func TestParseDependabotConfig_NoGoModEntries(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()
	githubDir := filepath.Join(tmpDir, ".github")
	require.NoError(t, os.MkdirAll(githubDir, 0o755))

	// Create dependabot.yaml without gomod entries
	dependabotYAML := `---
version: 2
updates:
  - package-ecosystem: "github-actions"
    directory: "/"
    schedule:
      interval: "weekly"
    commit-message:
      prefix: "[PF-267]"
      include: "scope"
  - package-ecosystem: "docker"
    directory: "/"
    schedule:
      interval: "weekly"
    commit-message:
      prefix: "[PF-282]"
`

	configPath := filepath.Join(githubDir, "dependabot.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(dependabotYAML), 0o644))

	// Parse - success but empty configs
	result, err := ParseDependabotConfig(tmpDir)

	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestParseDependabotConfig_DirectoryNormalization(t *testing.T) {
	tests := []struct {
		name              string
		inputDirectory    string
		expectedDirectory string
	}{
		{"Root dot", ".", "/"},
		{"Root slash", "/", "/"},
		{"Subdirectory with leading slash", "/services/api", "/services/api"},
		{"Subdirectory without leading slash", "services/api", "/services/api"},
		{"Subdirectory with trailing slash", "/services/api/", "/services/api"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory
			tmpDir := t.TempDir()
			githubDir := filepath.Join(tmpDir, ".github")
			require.NoError(t, os.MkdirAll(githubDir, 0o755))

			// Create dependabot.yaml with specific directory
			dependabotYAML := `version: 2
updates:
  - package-ecosystem: "gomod"
    directory: "` + tt.inputDirectory + `"
    schedule:
      interval: "weekly"
    commit-message:
      prefix: "[TEST]"
`

			configPath := filepath.Join(githubDir, "dependabot.yaml")
			require.NoError(t, os.WriteFile(configPath, []byte(dependabotYAML), 0o644))

			// Parse
			result, err := ParseDependabotConfig(tmpDir)
			require.NoError(t, err)
			require.Len(t, result, 1)
			assert.Equal(t, tt.expectedDirectory, result[0].Directory)
		})
	}
}

func TestFetchDependabotConfigFromGitHub_NoToken(t *testing.T) {
	// This test verifies that the activity fails without a GITHUB_TOKEN
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestActivityEnvironment()

	env.RegisterActivity(FetchDependabotConfigFromGitHub)

	_, err := env.ExecuteActivity(FetchDependabotConfigFromGitHub, FetchDependabotConfigFromGitHubInputs{
		Organization: "containifyci",
		Repository:   "temporal-worker",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GITHUB_TOKEN")
}