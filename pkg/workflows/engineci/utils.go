package engineci

import (
	"path/filepath"
	"strings"
)

// SanitizeRepoName extracts and sanitizes the repository name from a URL
// Example: https://github.com/containifyci/temporal-worker -> temporal-worker
func SanitizeRepoName(repoURL string) string {
	// Extract the last component of the URL path
	parts := strings.Split(strings.TrimRight(repoURL, "/"), "/")
	repoName := parts[len(parts)-1]

	// Remove .git suffix if present
	repoName = strings.TrimSuffix(repoName, ".git")

	// Replace special characters with hyphens
	repoName = strings.ReplaceAll(repoName, "/", "-")
	repoName = strings.ReplaceAll(repoName, "\\", "-")
	repoName = strings.ReplaceAll(repoName, ".", "-")
	repoName = strings.ReplaceAll(repoName, " ", "-")

	// Convert to lowercase
	repoName = strings.ToLower(repoName)

	return repoName
}

// GetCloneDirectory returns the full path for the clone directory
func GetCloneDirectory(repoURL string) string {
	repoName := SanitizeRepoName(repoURL)
	return filepath.Join("/tmp", "ci-"+repoName)
}
