package engineci

import (
	"testing"
)

func TestSanitizeRepoName(t *testing.T) {
	tests := []struct {
		name     string
		repoURL  string
		expected string
	}{
		{
			name:     "GitHub HTTPS URL",
			repoURL:  "https://github.com/containifyci/temporal-worker",
			expected: "temporal-worker",
		},
		{
			name:     "GitHub HTTPS URL with .git suffix",
			repoURL:  "https://github.com/containifyci/temporal-worker.git",
			expected: "temporal-worker",
		},
		{
			name:     "GitHub SSH URL",
			repoURL:  "git@github.com:containifyci/temporal-worker.git",
			expected: "temporal-worker",
		},
		{
			name:     "URL with trailing slash",
			repoURL:  "https://github.com/containifyci/temporal-worker/",
			expected: "temporal-worker",
		},
		{
			name:     "Repo name with dots",
			repoURL:  "https://github.com/containifyci/my.repo.name",
			expected: "my-repo-name",
		},
		{
			name:     "Repo name with special characters",
			repoURL:  "https://github.com/containifyci/My Repo Name",
			expected: "my-repo-name",
		},
		{
			name:     "Simple repo name",
			repoURL:  "https://github.com/user/repo",
			expected: "repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeRepoName(tt.repoURL)
			if result != tt.expected {
				t.Errorf("SanitizeRepoName(%q) = %q, want %q", tt.repoURL, result, tt.expected)
			}
		})
	}
}

func TestGetCloneDirectory(t *testing.T) {
	tests := []struct {
		name     string
		repoURL  string
		expected string
	}{
		{
			name:     "Simple GitHub repo",
			repoURL:  "https://github.com/containifyci/temporal-worker",
			expected: "/tmp/ci-temporal-worker",
		},
		{
			name:     "Repo with .git suffix",
			repoURL:  "https://github.com/containifyci/temporal-worker.git",
			expected: "/tmp/ci-temporal-worker",
		},
		{
			name:     "Repo with special characters",
			repoURL:  "https://github.com/containifyci/my.repo.name",
			expected: "/tmp/ci-my-repo-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetCloneDirectory(tt.repoURL)
			if result != tt.expected {
				t.Errorf("GetCloneDirectory(%q) = %q, want %q", tt.repoURL, result, tt.expected)
			}
		})
	}
}
