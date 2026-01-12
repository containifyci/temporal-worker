package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
)

func setupTestEnv(t *testing.T) *testsuite.TestActivityEnvironment {
	env := (&testsuite.WorkflowTestSuite{}).NewTestActivityEnvironment()
	env.RegisterActivity(CloneRepo)
	return env
}

func tempDir(t *testing.T) string {
	return filepath.Join(os.TempDir(), strings.ReplaceAll(t.Name(), "/", "-"))
}

func TestCloneRepo_Success(t *testing.T) {
	env := setupTestEnv(t)
	targetDir := tempDir(t)
	t.Cleanup(func() { _ = os.RemoveAll(targetDir) })

	val, err := env.ExecuteActivity(CloneRepo, "https://github.com/fr12k/go-file", "main", targetDir)
	require.NoError(t, err)

	var workDir string
	require.NoError(t, val.Get(&workDir))
	assert.Equal(t, targetDir, workDir)

	// Verify .git directory exists
	_, err = os.Stat(filepath.Join(targetDir, ".git"))
	require.NoError(t, err)
}

func TestCloneRepo_WithExistingDirectory(t *testing.T) {
	env := setupTestEnv(t)
	targetDir := tempDir(t)
	t.Cleanup(func() { _ = os.RemoveAll(targetDir) })

	// Create existing directory with a file
	require.NoError(t, os.MkdirAll(targetDir, 0755))
	testFile := filepath.Join(targetDir, "testfile.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("test"), 0644))

	val, err := env.ExecuteActivity(CloneRepo, "https://github.com/fr12k/go-file", "main", targetDir)
	require.NoError(t, err)

	var workDir string
	require.NoError(t, val.Get(&workDir))

	// Verify old file is gone
	_, err = os.Stat(testFile)
	assert.True(t, os.IsNotExist(err))
}

func TestCloneRepo_Errors(t *testing.T) {
	tests := []struct {
		name     string
		repo     string
		ref      string
		errorMsg string
	}{
		{"NonExistentRepo", "https://github.com/fr12k/invalidrepo-67890", "main", "git clone failed"},
		{"InvalidBranch", "https://github.com/fr12k/go-file", "nonexistent-branch-xyz", "git clone failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := setupTestEnv(t)
			targetDir := tempDir(t)
			t.Cleanup(func() { _ = os.RemoveAll(targetDir) })

			_, err := env.ExecuteActivity(CloneRepo, tt.repo, tt.ref, targetDir)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.errorMsg)
		})
	}
}
