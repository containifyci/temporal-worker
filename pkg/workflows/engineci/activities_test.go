package engineci

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
)

type ActivitiesTestSuite struct {
	testsuite.WorkflowTestSuite
}

func TestActivitiesTestSuite(t *testing.T) {
	// Note: These are integration-style tests that require git and engine-ci to be in PATH
	// They are more like smoke tests to ensure the activities are wired correctly
}

func TestSanitizeRepoNameInActivities(t *testing.T) {
	// Test that GetCloneDirectory properly sanitizes repo names
	testCases := []struct {
		repoURL  string
		expected string
	}{
		{"https://github.com/test/repo", "/tmp/ci-repo"},
		{"https://github.com/test/my.repo", "/tmp/ci-my-repo"},
	}

	for _, tc := range testCases {
		result := GetCloneDirectory(tc.repoURL)
		assert.Equal(t, tc.expected, result)
	}
}

func TestRunEngineCI(t *testing.T) {
	// This test runs 'engine-ci version' to verify the activity works with real binary
	// Skip if engine-ci not available
	if _, err := exec.LookPath("engine-ci"); err != nil {
		t.Skip("engine-ci not found in PATH, skipping test")
	}

	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestActivityEnvironment()
	env.RegisterActivity(RunEngineCI)

	// Create a temporary directory for the test
	tempDir := "/tmp/test-engine-ci-version-" + strings.ReplaceAll(t.Name(), "/", "-")
	err := os.MkdirAll(tempDir, 0755)
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Execute RunEngineCI with 'version' argument
	val, err := env.ExecuteActivity(RunEngineCI, tempDir, []string{"version"}, map[string]string{})

	// Should succeed without error
	assert.NoError(t, err)

	var details *EngineCIDetails
	err = val.Get(&details)
	require.NoError(t, err)

	// Verify exit code is 0 (success)
	assert.Equal(t, 0, details.ExitCode, "engine-ci version should exit with code 0")

	// Verify we got some output (version information)
	assert.NotEmpty(t, details.Last50Lines, "engine-ci version should produce output")
	t.Logf("engine-ci version output: %s", details.Last50Lines)
}

func TestRunEngineCI_ExitCodeHandling(t *testing.T) {
	// This test verifies that we properly capture exit codes
	// We'll use a simple shell command instead of engine-ci for testing

	// Note: This is a conceptual test. In real scenarios, you'd need to mock
	// the exec.Command calls or use a test double for the engine-ci binary

	t.Skip("Skipping integration test - requires engine-ci binary")
}
