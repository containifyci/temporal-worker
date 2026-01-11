package filesystem

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
)

func setupTestEnv(_ *testing.T) *testsuite.TestActivityEnvironment {
	env := (&testsuite.WorkflowTestSuite{}).NewTestActivityEnvironment()
	env.RegisterActivity(CleanupDirectory)
	return env
}

func tempDir(t *testing.T) string {
	return filepath.Join(os.TempDir(), strings.ReplaceAll(t.Name(), "/", "-"))
}

func TestCleanupDirectory_Success(t *testing.T) {
	env := setupTestEnv(t)
	dir := tempDir(t)
	require.NoError(t, os.MkdirAll(dir, 0755))

	// Create files in directory
	for i := 0; i < 3; i++ {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("test"), 0644))
	}

	_, err := env.ExecuteActivity(CleanupDirectory, dir)
	require.NoError(t, err)

	_, err = os.Stat(dir)
	assert.True(t, os.IsNotExist(err))
}

func TestCleanupDirectory_NonExistent(t *testing.T) {
	env := setupTestEnv(t)
	_, err := env.ExecuteActivity(CleanupDirectory, tempDir(t))
	require.NoError(t, err)
}

func TestCleanupDirectory_ErrorHandling(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping permission test on Windows")
	}

	env := setupTestEnv(t)
	parentDir := tempDir(t)
	childDir := filepath.Join(parentDir, "child")

	require.NoError(t, os.MkdirAll(childDir, 0755))
	defer func() {
		os.Chmod(parentDir, 0755)
		os.RemoveAll(parentDir)
	}()

	require.NoError(t, os.Chmod(parentDir, 0555))

	_, err := env.ExecuteActivity(CleanupDirectory, childDir)
	assert.Error(t, err)
}
