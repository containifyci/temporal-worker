package golang

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.temporal.io/sdk/activity"
	"gopkg.in/yaml.v3"

	githubactivity "github.com/containifyci/temporal-worker/pkg/activities/github"
)

// ErrNoDependabotConfig is returned when no dependabot configuration file is found
var ErrNoDependabotConfig = errors.New("no dependabot config found")

// DependabotConfig represents the structure of .github/dependabot.yaml
type DependabotConfig struct {
	Updates []DependabotUpdate `yaml:"updates"`
	Version int                `yaml:"version"`
}

// DependabotUpdate represents a single package ecosystem update configuration
type DependabotUpdate struct {
	PackageEcosystem string              `yaml:"package-ecosystem"` //nolint:tagliatelle
	Directory        string              `yaml:"directory"`
	Schedule         DependabotSchedule  `yaml:"schedule"`
	CommitMessage    DependabotCommitMsg `yaml:"commit-message"` //nolint:tagliatelle
}

// DependabotSchedule represents the schedule configuration
type DependabotSchedule struct {
	Interval string `yaml:"interval"`
	Day      string `yaml:"day"`
}

// DependabotCommitMsg represents commit message configuration
type DependabotCommitMsg struct {
	Prefix  string `yaml:"prefix"`
	Include string `yaml:"include"`
}

// GoModConfig contains the parsed configuration for a single gomod entry
type GoModConfig struct {
	Directory           string
	CommitMessagePrefix string
}

// FetchDependabotConfigFromGitHubInputs contains parameters for fetching dependabot config from GitHub
type FetchDependabotConfigFromGitHubInputs struct {
	Organization string
	Repository   string
}

// FetchDependabotConfigFromGitHubOutputs contains the parsed gomod configurations
type FetchDependabotConfigFromGitHubOutputs struct {
	GoModConfigs []GoModConfig
}

// normalizeDirectory normalizes a dependabot directory path to a canonical form.
// Converts "." to "/", ensures leading slash, removes trailing slash (except for root).
func normalizeDirectory(directory string) string {
	// Convert "." to "/"
	if directory == "." {
		directory = "/"
	}

	// Ensure leading slash
	if !strings.HasPrefix(directory, "/") {
		directory = "/" + directory
	}

	// Remove trailing slash for consistency (except for root)
	directory = strings.TrimSuffix(directory, "/")
	if directory == "" {
		directory = "/"
	}

	return directory
}

// FetchDependabotConfigFromGitHub fetches and parses the dependabot config from GitHub without cloning
func FetchDependabotConfigFromGitHub(ctx context.Context, i FetchDependabotConfigFromGitHubInputs) (FetchDependabotConfigFromGitHubOutputs, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Fetching dependabot configuration from GitHub", "org", i.Organization, "repo", i.Repository)

	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		return FetchDependabotConfigFromGitHubOutputs{}, errors.New("GITHUB_TOKEN environment variable is required")
	}

	// Create GitHub client using the shared helper
	client := githubactivity.NewGitHubClient(githubToken)

	// Try both .yaml and .yml extensions
	filePaths := []string{".github/dependabot.yaml", ".github/dependabot.yml"}

	var configPath string
	var fileContent string

	for _, path := range filePaths {
		content, _, _, err := client.Repositories.GetContents(ctx, i.Organization, i.Repository, path, nil)
		if err == nil && content != nil {
			// Get the file content
			decoded, decodeErr := content.GetContent()
			if decodeErr != nil {
				continue
			}
			configPath = path
			fileContent = decoded
			break
		}
	}

	// If neither file exists, return special error
	if configPath == "" {
		logger.Info("No dependabot configuration file found in repository")
		return FetchDependabotConfigFromGitHubOutputs{}, ErrNoDependabotConfig
	}

	logger.Info("Found dependabot configuration", "path", configPath)

	// Parse YAML
	var config DependabotConfig
	if err := yaml.Unmarshal([]byte(fileContent), &config); err != nil {
		return FetchDependabotConfigFromGitHubOutputs{}, fmt.Errorf("failed to parse dependabot config: %w", err)
	}

	// Extract gomod configurations
	goModConfigs := make([]GoModConfig, 0, len(config.Updates))
	for _, update := range config.Updates {
		if update.PackageEcosystem != "gomod" {
			continue
		}
		// Normalize directory
		directory := normalizeDirectory(update.Directory)

		goModConfig := GoModConfig{
			Directory:           directory,
			CommitMessagePrefix: update.CommitMessage.Prefix,
		}
		goModConfigs = append(goModConfigs, goModConfig)
		logger.Info("Found gomod configuration", "directory", directory, "prefix", update.CommitMessage.Prefix)
	}

	if len(goModConfigs) == 0 {
		logger.Info("No gomod configurations found in dependabot config")
	}

	logger.Info("Parsed dependabot configuration successfully", "goModCount", len(goModConfigs))

	return FetchDependabotConfigFromGitHubOutputs{
		GoModConfigs: goModConfigs,
	}, nil
}

// ParseDependabotConfig parses the .github/dependabot.yaml file and extracts gomod configurations
func ParseDependabotConfig(repoPath string) ([]GoModConfig, error) {
	// Try both .yaml and .yml extensions
	configPaths := []string{
		filepath.Join(repoPath, ".github", "dependabot.yaml"),
		filepath.Join(repoPath, ".github", "dependabot.yml"),
	}

	var configPath string
	var configData []byte
	var err error

	for _, path := range configPaths {
		configData, err = os.ReadFile(path)
		if err == nil {
			configPath = path
			break
		}
	}

	// If neither file exists, return special error
	if configPath == "" {
		return nil, ErrNoDependabotConfig
	}

	// Parse YAML
	var config DependabotConfig
	if err := yaml.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("failed to parse dependabot config: %w", err)
	}

	// Extract gomod configurations
	goModConfigs := make([]GoModConfig, 0, len(config.Updates))
	for _, update := range config.Updates {
		if update.PackageEcosystem != "gomod" {
			continue
		}
		directory := normalizeDirectory(update.Directory)
		goModConfig := GoModConfig{
			Directory:           directory,
			CommitMessagePrefix: update.CommitMessage.Prefix,
		}
		goModConfigs = append(goModConfigs, goModConfig)
	}

	return goModConfigs, nil
}