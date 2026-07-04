package golang

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"go.temporal.io/sdk/activity"
)

// UpgradeDependencyInputs contains the parameters for upgrading a dependency
type UpgradeDependencyInputs struct {
	RepoPath    string
	FromModule  string
	ToModule    string
	ToVersion   Version
	FromVersion Version
	Directory   string // Subdirectory containing go.mod (e.g., "/", "/services/api")
}

// UpgradeDependencyOutputs contains the results of upgrading a dependency
type UpgradeDependencyOutputs struct {
	GoGetError string
	ModError   string
	Upgraded   bool
	HasChanges bool
}

// UpgradeDependency upgrades a Go dependency to a new major version
func UpgradeDependency(ctx context.Context, i UpgradeDependencyInputs) (UpgradeDependencyOutputs, error) {
	logger := activity.GetLogger(ctx)

	// Determine working directory
	workDir := i.RepoPath
	if i.Directory != "" && i.Directory != "/" {
		workDir = filepath.Join(i.RepoPath, strings.TrimPrefix(i.Directory, "/"))
	}

	logger.Info("Upgrading dependency", "module", i.FromModule, "fromVersion", i.FromVersion, "toVersion", i.ToVersion, "directory", i.Directory, "workDir", workDir)

	// Parse module to determine target
	modulePath := SplitBaseMajorKind(i.ToModule)

	logger.Info("Upgrading to new major", "from", i.FromModule, "to", i.ToModule, "toMajor", modulePath.Major)

	// Step 1: Run go get
	goGetErr := ""
	upgraded := true
	output, modulePath, err := goGetWithFallback(ctx, workDir, modulePath)
	if err != nil {
		// Capture error but don't fail the activity
		goGetErr = fmt.Sprintf("go get failed: %v\nOutput: %s", err, output)
		upgraded = false
		logger.Warn("go get failed, but continuing to mod replace", "error", goGetErr)
	}
	logger.Debug("go get completed", "output", output)

	// Step 2: Run mod replace
	// This single command handles both import rewriting AND go.mod updates
	logger.Debug("Running mod replace", "targetModule", i.ToModule)

	// Validate module paths before use in command
	if validateErr := ValidateModulePath(i.FromModule); validateErr != nil {
		return UpgradeDependencyOutputs{}, fmt.Errorf("invalid from module: %w", validateErr)
	}
	if validateErr := ValidateModulePath(modulePath.Join()); validateErr != nil {
		return UpgradeDependencyOutputs{}, fmt.Errorf("invalid to module: %w", validateErr)
	}

	//nolint:gosec // G204: Module paths are validated and come from trusted Temporal workflow inputs
	modCmd := exec.CommandContext(ctx, "mod", "replace", "--mod-old", i.FromModule, "--mod-new", modulePath.Join())
	env := os.Environ()
	if IsPrivateModule(modulePath.Base) == "" {
		env = append(env, "GOPROXY=https://proxy.golang.org/cached-only")
	}
	modCmd.Env = env
	modCmd.Dir = workDir
	modOutput, err := modCmd.CombinedOutput()

	modError := ""
	if err != nil {
		// Capture error but don't fail the activity
		modError = fmt.Sprintf("mod git failed: %v\nOutput: %s", err, string(modOutput))
		upgraded = false
		logger.Warn("mod replace failed, but continuing to check for changes", "error", modError)
	} else {
		logger.Debug("mod replace completed", "output", string(modOutput))
	}

	// Check if there are any git changes (go.mod/go.sum modifications)
	hasChanges := checkGitHasChanges(ctx, workDir)

	logger.Info("Checking for git changes", "hasChanges", hasChanges)

	if upgraded {
		logger.Info("Dependency upgrade completed successfully", "from", i.FromModule, "to", i.ToModule)
	} else {
		logger.Info("Dependency upgrade partially completed (go get or mod replace failed)", "from", i.FromModule, "to", i.ToModule, "hasChanges", hasChanges)
	}

	return UpgradeDependencyOutputs{
		Upgraded:   upgraded,
		GoGetError: goGetErr,
		ModError:   modError,
		HasChanges: hasChanges,
	}, nil
}

var reCanon = regexp.MustCompile(`module declares its path as:\s+(\S+)`)

// parseGoGetError extracts the canonical module path from a go get error message
func parseGoGetError(err error) string {
	msg := err.Error()
	if m := reCanon.FindStringSubmatch(msg); len(m) == 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func goGetWithFallback(ctx context.Context, workDir string, modulePath ModulePath) (string, ModulePath, error) {
	output, err := goGet(ctx, workDir, modulePath)
	if err != nil {
		canonicalPath := parseGoGetError(err)
		if canonicalPath == "" {
			// No canonical path found in the error output, return the original error
			return output, modulePath, err
		}
		modulePath = SplitBaseMajorKind(canonicalPath)
		logger := activity.GetLogger(ctx)
		logger.Warn("go get failed, retrying with declared module form error", "module", modulePath.JoinWithMajorAndLatest(modulePath.Major))
		_output, err := goGet(ctx, workDir, modulePath)
		return _output, modulePath, err
	}
	return output, modulePath, nil
}

func goGet(ctx context.Context, workDir string, modulePath ModulePath) (string, error) {
	logger := activity.GetLogger(ctx)

	// Validate GitHub token for private modules
	privatePattern := IsPrivateModule(modulePath.Base)
	if privatePattern != "" && os.Getenv("GITHUB_TOKEN") == "" {
		return "", fmt.Errorf("GitHub token is required for private module: %s", modulePath.Base)
	}

	targetModule := modulePath.JoinWithMajorAndLatest(modulePath.Major)
	if err := ValidateModulePath(targetModule); err != nil {
		return "", fmt.Errorf("invalid module path: %w", err)
	}

	getCmd := exec.CommandContext(ctx, "go", "get", targetModule)

	env := os.Environ()
	if privatePattern == "" {
		env = append(env, "GOPROXY=https://proxy.golang.org/cached-only")
	} else {
		// For private modules, set GOPRIVATE and GIT_CONFIG_PARAMETERS
		githubToken := os.Getenv("GITHUB_TOKEN")
		organization := os.Getenv("GITHUB_ORGANIZATION")
		if organization == "" {
			organization = "containifyci"
		}
		gitConfigParam := fmt.Sprintf("'url.https://oauth:%s@github.com/%s/.insteadOf=https://github.com/%s/'", githubToken, organization, organization)
		env = append(env,
			"GOPRIVATE=github.com/"+organization+"/*",
			"GIT_CONFIG_PARAMETERS="+gitConfigParam,
		)
	}
	getCmd.Env = env
	getCmd.Dir = workDir
	getOutput, err := getCmd.CombinedOutput()
	if err != nil {
		return string(getOutput), fmt.Errorf("go get failed: %w\nOutput: %s", err, string(getOutput))
	}
	logger.Debug("go get completed", "output", string(getOutput))
	return string(getOutput), nil
}

// checkGitHasChanges checks if there are uncommitted changes in the working directory
func checkGitHasChanges(ctx context.Context, workDir string) bool {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = workDir
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) != ""
}