package golang

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/log"
)

// DetectMajorUpgradesInputs contains the parameters for detecting major upgrades
type DetectMajorUpgradesInputs struct {
	RepoPath  string
	Directory string // Subdirectory containing go.mod (e.g., "/", "/services/api")
}

// MajorUpgrade represents a single major version upgrade opportunity
type MajorUpgrade struct {
	FromModule   string
	ToModule     string
	FromVersion  Version
	ToVersion    Version
	ReleasesURL  string
	ChangelogURL string
	CompareURL   string
}

// DetectMajorUpgradesOutputs contains the list of detected major upgrades
type DetectMajorUpgradesOutputs struct {
	Upgrades     []MajorUpgrade
	ModuleErrors []string
}

// goListModule represents the structure returned by 'go list -m -u -json all'
//
//nolint:tagliatelle
type goListModule struct {
	Update *struct {
		Path    string  `json:"Path"`
		Version Version `json:"Version"`
	} `json:"Update"`
	Error *struct {
		Err string `json:"Err"`
	} `json:"Error"`
	Path      string  `json:"Path"`
	Query     string  `json:"Query"`
	Time      string  `json:"Time"`
	GoMod     string  `json:"GoMod"`
	GoVersion string  `json:"GoVersion"`
	Version   Version `json:"Version"`
	Indirect  bool    `json:"Indirect"`
}

// DetectMajorUpgrades detects major version upgrades for Go modules in the repository
func DetectMajorUpgrades(ctx context.Context, i DetectMajorUpgradesInputs) (DetectMajorUpgradesOutputs, error) {
	logger := activity.GetLogger(ctx)

	// Determine working directory
	workDir := i.RepoPath
	if i.Directory != "" && i.Directory != "/" {
		workDir = filepath.Join(i.RepoPath, strings.TrimPrefix(i.Directory, "/"))
	}

	logger.Info("Detecting major upgrades", "repoPath", i.RepoPath, "directory", i.Directory, "workDir", workDir)

	// Execute: go list -e -m -u -json all (-e flag allows command to continue on errors)
	cmd := exec.CommandContext(ctx, "go", "list", "-e", "-m", "-json", "all")
	cmd.Dir = workDir

	// Set environment variables for private module access
	env := os.Environ()
	if goPrivate := os.Getenv("GOPRIVATE"); goPrivate != "" {
		// GOPRIVATE is already in env, just make sure it's used
	} else {
		// Set GOPRIVATE from IsPrivateModule pattern detection if available
		env = append(env, "GOPRIVATE=")
	}

	// Set GIT_CONFIG_PARAMETERS for Git authentication if GITHUB_TOKEN is available
	if githubToken := os.Getenv("GITHUB_TOKEN"); githubToken != "" {
		organization := os.Getenv("GITHUB_ORGANIZATION")
		if organization == "" {
			organization = "containifyci"
		}
		gitConfigParam := fmt.Sprintf("'url.https://oauth:%s@github.com/%s/.insteadOf=https://github.com/%s/'", githubToken, organization, organization)
		env = append(env, "GIT_CONFIG_PARAMETERS="+gitConfigParam)

		// Also set GOPRIVATE for the organization
		goPrivate := os.Getenv("GOPRIVATE")
		if goPrivate == "" {
			env = append(env, "GOPRIVATE=github.com/"+organization+"*")
		}
	}

	cmd.Env = env

	output, err := cmd.CombinedOutput()
	if err != nil {
		if len(env) > 0 {
			logger.Error("Detecting major upgrades", "env", env)
		}
		return DetectMajorUpgradesOutputs{}, fmt.Errorf("failed to execute 'go list -e -m -u -json all': %w - %s", err, output)
	}

	// Parse JSON output (go list outputs multiple JSON objects, not an array)
	var upgrades []MajorUpgrade
	var moduleErrors []string
	decoder := json.NewDecoder(strings.NewReader(string(output)))

	for decoder.More() {
		var module goListModule
		if err := decoder.Decode(&module); err != nil {
			logger.Warn("Failed to decode module JSON", "error", err)
			continue
		}

		// Check if module has an error
		if module.Error != nil {
			errorMsg := fmt.Sprintf("%s: %s", module.Path, module.Error.Err)
			logger.Warn("Module has error, skipping", "module", module.Path, "error", module.Error.Err)
			moduleErrors = append(moduleErrors, errorMsg)
			continue
		}

		// Check for upgrade
		upgrade := detectModuleUpgrade(ctx, module, logger)
		if upgrade != nil {
			upgrades = append(upgrades, *upgrade)
		}
	}

	logger.Info("Major upgrades detected", "count", len(upgrades), "moduleErrors", len(moduleErrors))

	return DetectMajorUpgradesOutputs{
		Upgrades:     upgrades,
		ModuleErrors: moduleErrors,
	}, nil
}

// detectModuleUpgrade checks if a single module has a major version upgrade available
func detectModuleUpgrade(ctx context.Context, module goListModule, logger log.Logger) *MajorUpgrade {
	// Skip indirect dependencies
	if module.Indirect {
		return nil
	}

	// Parse base and current major version
	modulePath := SplitBaseMajorKind(module.Path)
	logger.Debug("Checking for upgrades", "module", module.Path, "base", modulePath.Base, "currentMajor", modulePath.Major)

	// Probe for highest available major version
	highestMajor := findHighestMajor(ctx, modulePath)

	// Skip if already at highest version
	if highestMajor == modulePath.Major {
		logger.Info("Already at highest major", "module", module.Path, "current", modulePath.Major)
		return nil
	}

	// Construct target module path with new major version
	toModulePath := modulePath.JoinWithMajor(highestMajor)

	// Get version info for the new major
	toModule, err := goList(ctx, toModulePath)
	if err != nil {
		logger.Warn("Failed to get version for new major", "module", toModulePath, "error", err)
		return nil
	}

	// Verify we got valid version info
	if toModule == nil || toModule.Version == "" {
		logger.Warn("No version info available", "module", toModulePath)
		return nil
	}

	// Skip pseudo-versions (versions bound to git commits, not released tags)
	if IsPseudoVersion(toModule.Version) {
		logger.Warn("Skipping pseudo-version (not a released tag)",
			"module", toModulePath,
			"version", toModule.Version)
		return nil
	}

	logger.Info("Found major upgrade", "from", module.Version, "to", toModule.Version, "fromMajor", modulePath.Major, "toMajor", highestMajor)

	return &MajorUpgrade{
		FromModule:   module.Path,
		ToModule:     toModulePath,
		FromVersion:  module.Version,
		ToVersion:    toModule.Version,
		ReleasesURL:  GenerateReleasesURL(modulePath.Base),
		ChangelogURL: GenerateChangelogURL(modulePath.Base),
		CompareURL:   GenerateCompareURL(modulePath.Base, module.Version, toModule.Version),
	}
}

// findHighestMajor probes base/vN@latest until misses >= 3
func findHighestMajor(ctx context.Context, modulePath ModulePath) int {
	logger := activity.GetLogger(ctx)
	current := modulePath.Major
	maxMajor := modulePath.Major

	for n := current + 1; n <= current+50; n++ {
		cand := modulePath.JoinWithMajorAndLatest(n)
		logger.Debug("Probing for major version", "module", cand)
		mod, err := goList(ctx, cand)
		switch {
		case err == nil && mod != nil && (mod.Error == nil || mod.Error.Err == ""):
			maxMajor = n
			continue
		case err != nil:
			logger.Debug("Major version probe failed", "module", cand, "error", err)
		case mod != nil && mod.Error != nil:
			logger.Debug("Major version probe returned error", "module", cand, "error", mod.Error.Err)
		}
		break
	}

	return maxMajor
}

// goList runs "go list -m -json -e <module>"
func goList(ctx context.Context, module string) (*goListModule, error) {
	cmd := exec.CommandContext(ctx, "go", "list", "-m", "-e", "-json", module)
	var out bytes.Buffer

	env := os.Environ()

	// For public modules we still hit proxy.golang.org/cached-only
	// For private modules we do NOT override GOPROXY so that:
	//   - GOPRIVATE is honored
	//   - go can talk directly to GitHub using git credentials.
	if IsPrivateModule(module) == "" {
		env = append(env, "GOPROXY=https://proxy.golang.org/cached-only")
	}

	cmd.Env = env
	cmd.Stdout = &out
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to run go list: %w", err)
	}

	var m goListModule
	if err := json.Unmarshal(out.Bytes(), &m); err != nil {
		return nil, fmt.Errorf("failed to unmarshal go list output: %w", err)
	}

	return &m, nil
}
