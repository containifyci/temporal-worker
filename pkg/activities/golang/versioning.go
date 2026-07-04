package golang

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/mod/semver"
)

// Version represents a Go module version string (e.g., "v1.2.3")
type Version string

// VersionPathKind tells how the major version is encoded in the import path.
type VersionPathKind int

const (
	// KindNone means no explicit major suffix (v0/v1 typical).
	KindNone VersionPathKind = iota
	// KindSlash means suffix like "/v63".
	KindSlash
	// KindDot means suffix like ".v23" (gopkg.in style).
	KindDot
)

// ModulePath represents a parsed Go module path with its base, major version, and kind.
type ModulePath struct {
	Base  string
	Major int
	Kind  VersionPathKind
}

// SplitBaseMajorKind parses a module path into a ModulePath struct.
// Examples:
//
//	"github.com/google/go-github/v63"  → Base="github.com/google/go-github", Major=63, Kind=KindSlash
//	"gopkg.in/DataDog/dd-trace-go.v23" → Base="gopkg.in/DataDog/dd-trace-go", Major=23, Kind=KindDot
//	"github.com/sirupsen/logrus"       → Base="github.com/sirupsen/logrus",  Major=1,  Kind=KindNone
func SplitBaseMajorKind(dep string) ModulePath {
	// Try slash style: "/vN" at end
	if m := regexp.MustCompile(`/v(\d+)$`).FindStringSubmatch(dep); len(m) == 2 {
		n, _ := strconv.Atoi(m[1])
		base := dep[:len(dep)-len(m[0])]
		return ModulePath{Base: base, Major: n, Kind: KindSlash}
	}

	// Try dot style: ".vN" at end (gopkg.in)
	if m := regexp.MustCompile(`\.v(\d+)$`).FindStringSubmatch(dep); len(m) == 2 {
		n, _ := strconv.Atoi(m[1])
		base := dep[:len(dep)-len(m[0])]
		return ModulePath{Base: base, Major: n, Kind: KindDot}
	}

	// Default: no suffix → assume v1
	return ModulePath{Base: dep, Major: 1, Kind: KindNone}
}

// JoinWithMajor reconstructs the module path with a new major version.
// Rules:
//   - KindSlash: omit suffix for v1; add "/vN" for v2+
//   - KindDot:   always use ".vN"
//   - KindNone:  same as KindSlash
func (m ModulePath) JoinWithMajor(newMajor int) string {
	if newMajor <= 1 {
		switch m.Kind {
		case KindDot:
			return m.Base + ".v1"
		case KindSlash, KindNone:
			return m.Base
		}
	}

	switch m.Kind {
	case KindDot:
		return fmt.Sprintf("%s.v%d", m.Base, newMajor)
	case KindSlash, KindNone:
		return fmt.Sprintf("%s/v%d", m.Base, newMajor)
	}
	return fmt.Sprintf("%s/v%d", m.Base, newMajor)
}

// Join reconstructs the module path with its current major version.
func (m ModulePath) Join() string {
	return m.JoinWithMajor(m.Major)
}

// JoinWithMajorAndLatest reconstructs the module path with a new major version and @latest suffix.
func (m ModulePath) JoinWithMajorAndLatest(newMajor int) string {
	return m.JoinWithMajor(newMajor) + "@latest"
}

// IsPseudoVersion checks if a version string is a pseudo-version (bound to a git commit)
// or a pre-release version.
func IsPseudoVersion(version Version) bool {
	// Matches:
	//   - vX.Y.Z-yyyymmddhhmmss-abcdef123456  (standard pseudo-version)
	//   - vX.Y.Z-rc1, vX.Y.Z-beta2, vX.Y.Z-alpha3, etc.  (pre-release)
	pseudoVersionPattern := regexp.MustCompile(
		`^v\d+\.\d+\.\d+(?:-(?:\d{14}-[a-f0-9]{12}|[A-Za-z]+\d*))$`,
	)
	return pseudoVersionPattern.MatchString(string(version))
}

// IsMajorUpgrade checks if the version upgrade is a major version change
func IsMajorUpgrade(from, to string) bool {
	// Ensure versions have 'v' prefix for semver comparison
	if !strings.HasPrefix(from, "v") {
		from = "v" + from
	}
	if !strings.HasPrefix(to, "v") {
		to = "v" + to
	}

	// Check if both are valid semver
	if !semver.IsValid(from) || !semver.IsValid(to) {
		return false
	}

	// Compare major versions
	fromMajor := semver.Major(from)
	toMajor := semver.Major(to)

	return fromMajor != toMajor && semver.Compare(to, from) > 0
}

// ValidateModulePath validates that a module path is well-formed and safe to use in commands.
// It checks that the path only contains allowed characters and doesn't contain shell metacharacters.
func ValidateModulePath(modulePath string) error {
	// Module paths should only contain alphanumeric, dots, slashes, hyphens, underscores, and @
	validPattern := regexp.MustCompile(`^[a-zA-Z0-9./_@-]+$`)
	if !validPattern.MatchString(modulePath) {
		return fmt.Errorf("invalid module path format: %s", modulePath)
	}

	// Additional check: should not contain shell metacharacters
	dangerousChars := []string{";", "&", "|", "$", "`", "(", ")", "<", ">", "\"", "'", "\\"}
	for _, char := range dangerousChars {
		if strings.Contains(modulePath, char) {
			return fmt.Errorf("module path contains dangerous character: %s", char)
		}
	}

	return nil
}

// GenerateReleasesURL generates a GitHub releases URL for the module
func GenerateReleasesURL(modulePath string) string {
	if strings.HasPrefix(modulePath, "github.com/") {
		repoPath := strings.TrimPrefix(modulePath, "github.com/")
		// Remove /vN suffix if present
		repoPath = regexp.MustCompile(`/v\d+$`).ReplaceAllString(repoPath, "")
		return fmt.Sprintf("https://github.com/%s/releases", repoPath)
	}
	return ""
}

// GenerateChangelogURL attempts to generate a changelog URL for the module
func GenerateChangelogURL(modulePath string) string {
	changelogFiles := []string{"CHANGELOG.md", "CHANGES.md", "HISTORY.md"}

	if strings.HasPrefix(modulePath, "github.com/") {
		repoPath := strings.TrimPrefix(modulePath, "github.com/")
		// Remove /vN suffix if present
		repoPath = regexp.MustCompile(`/v\d+$`).ReplaceAllString(repoPath, "")
		return fmt.Sprintf("https://github.com/%s/blob/main/%s", repoPath, changelogFiles[0])
	}
	return ""
}

// GenerateCompareURL generates a GitHub compare URL between two versions
func GenerateCompareURL(modulePath string, from, to Version) string {
	if strings.HasPrefix(modulePath, "github.com/") {
		repoPath := strings.TrimPrefix(modulePath, "github.com/")
		// Remove /vN suffix if present
		repoPath = regexp.MustCompile(`/v\d+$`).ReplaceAllString(repoPath, "")
		return fmt.Sprintf("https://github.com/%s/compare/%s...%s", repoPath, from, to)
	}
	return ""
}

// IsPrivateModule checks if a module path matches any pattern in the GOPRIVATE
// environment variable. Returns the matching pattern (or empty string if not private).
// The GOPRIVATE variable is a comma-separated list of glob patterns
// (e.g., "github.com/containifyci/*,github.com/myorg/*").
func IsPrivateModule(module string) string {
	goPrivate := os.Getenv("GOPRIVATE")
	if goPrivate == "" {
		return ""
	}
	patterns := strings.Split(goPrivate, ",")
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		// Handle glob patterns like "github.com/containifyci/*"
		globPattern := strings.TrimSuffix(pattern, "/*")
		if strings.HasPrefix(module, globPattern) {
			return pattern
		}
	}
	return ""
}