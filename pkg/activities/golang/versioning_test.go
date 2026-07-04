package golang

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitBaseMajorKind(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		input      string
		wantBase   string
		wantMajor  int
		wantKind   VersionPathKind
	}{
		{
			name:      "slash style v63",
			input:     "github.com/google/go-github/v63",
			wantBase:  "github.com/google/go-github",
			wantMajor: 63,
			wantKind:  KindSlash,
		},
		{
			name:      "dot style gopkg.in",
			input:     "gopkg.in/DataDog/dd-trace-go.v23",
			wantBase:  "gopkg.in/DataDog/dd-trace-go",
			wantMajor: 23,
			wantKind:  KindDot,
		},
		{
			name:      "no suffix defaults to v1",
			input:     "github.com/sirupsen/logrus",
			wantBase:  "github.com/sirupsen/logrus",
			wantMajor: 1,
			wantKind:  KindNone,
		},
		{
			name:      "v2 suffix",
			input:     "github.com/go-chi/chi/v2",
			wantBase:  "github.com/go-chi/chi",
			wantMajor: 2,
			wantKind:  KindSlash,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := SplitBaseMajorKind(tt.input)
			assert.Equal(t, tt.wantBase, result.Base)
			assert.Equal(t, tt.wantMajor, result.Major)
			assert.Equal(t, tt.wantKind, result.Kind)
		})
	}
}

func TestModulePath_JoinWithMajor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		module    ModulePath
		newMajor  int
		want      string
	}{
		{
			name:     "slash style v2+",
			module:   ModulePath{Base: "github.com/gin-gonic/gin", Major: 1, Kind: KindSlash},
			newMajor: 2,
			want:     "github.com/gin-gonic/gin/v2",
		},
		{
			name:     "slash style v1 omits suffix",
			module:   ModulePath{Base: "github.com/gin-gonic/gin", Major: 2, Kind: KindSlash},
			newMajor: 1,
			want:     "github.com/gin-gonic/gin",
		},
		{
			name:     "dot style always includes",
			module:   ModulePath{Base: "gopkg.in/DataDog/dd-trace-go", Major: 22, Kind: KindDot},
			newMajor: 23,
			want:     "gopkg.in/DataDog/dd-trace-go.v23",
		},
		{
			name:     "dot style v1",
			module:   ModulePath{Base: "gopkg.in/yaml", Major: 2, Kind: KindDot},
			newMajor: 1,
			want:     "gopkg.in/yaml.v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.module.JoinWithMajor(tt.newMajor))
		})
	}
}

func TestIsPseudoVersion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		version  Version
		expected bool
	}{
		{
			name:     "valid pseudo-version",
			version:  "v2.0.0-20251030143703-54fe93c1e561",
			expected: true,
		},
		{
			name:     "valid pseudo-version v1",
			version:  "v1.5.0-20230101120000-abcdef123456",
			expected: true,
		},
		{
			name:     "valid pseudo-version v3",
			version:  "v3.10.2-20241225093045-1234567890ab",
			expected: true,
		},
		{
			name:     "real released version",
			version:  "v2.0.0",
			expected: false,
		},
		{
			name:     "real released version with patch",
			version:  "v1.2.3",
			expected: false,
		},
		{
			name:     "pre-release rc",
			version:  "v2.0.0-rc4",
			expected: true,
		},
		{
			name:     "pre-release alpha",
			version:  "v2.0.0-alpha3",
			expected: true,
		},
		{
			name:     "invalid pseudo-version - wrong timestamp length",
			version:  "v2.0.0-2025103014-54fe93c1e561",
			expected: false,
		},
		{
			name:     "version without v prefix",
			version:  "2.0.0-20251030143703-54fe93c1e561",
			expected: false,
		},
		{
			name:     "empty version",
			version:  "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := IsPseudoVersion(tt.version)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsMajorUpgrade(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		from     string
		to       string
		expected bool
	}{
		{
			name:     "v1 to v2 is major",
			from:     "v1.0.0",
			to:       "v2.0.0",
			expected: true,
		},
		{
			name:     "v1 to v1 is not major",
			from:     "v1.0.0",
			to:       "v1.1.0",
			expected: false,
		},
		{
			name:     "v2 to v3 is major",
			from:     "v2.5.0",
			to:       "v3.0.0",
			expected: true,
		},
		{
			name:     "same version is not major",
			from:     "v1.0.0",
			to:       "v1.0.0",
			expected: false,
		},
		{
			name:     "patch upgrade is not major",
			from:     "v1.0.0",
			to:       "v1.0.1",
			expected: false,
		},
		{
			name:     "downgrade is not major",
			from:     "v2.0.0",
			to:       "v1.0.0",
			expected: false,
		},
		{
			name:     "without v prefix",
			from:     "1.0.0",
			to:       "2.0.0",
			expected: true,
		},
		{
			name:     "invalid semver",
			from:     "invalid",
			to:       "v2.0.0",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := IsMajorUpgrade(tt.from, tt.to)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateModulePath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		input     string
		wantError bool
	}{
		{
			name:      "valid module path",
			input:     "github.com/gin-gonic/gin/v2",
			wantError: false,
		},
		{
			name:      "module path with shell metacharacters",
			input:     "github.com/gin-gonic/gin;rm -rf /",
			wantError: true,
		},
		{
			name:      "module path with dollar sign",
			input:     "github.com/$HOME/gin",
			wantError: true,
		},
		{
			name:      "empty module path",
			input:     "",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateModulePath(tt.input)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGenerateReleasesURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		modulePath string
		expected   string
	}{
		{
			name:       "GitHub module",
			modulePath: "github.com/gin-gonic/gin",
			expected:   "https://github.com/gin-gonic/gin/releases",
		},
		{
			name:       "GitHub module with /v2",
			modulePath: "github.com/go-chi/chi/v2",
			expected:   "https://github.com/go-chi/chi/releases",
		},
		{
			name:       "Non-GitHub module",
			modulePath: "golang.org/x/mod",
			expected:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := GenerateReleasesURL(tt.modulePath)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateChangelogURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		modulePath string
		expected   string
	}{
		{
			name:       "GitHub module",
			modulePath: "github.com/gin-gonic/gin",
			expected:   "https://github.com/gin-gonic/gin/blob/main/CHANGELOG.md",
		},
		{
			name:       "GitHub module with /v2",
			modulePath: "github.com/go-chi/chi/v2",
			expected:   "https://github.com/go-chi/chi/blob/main/CHANGELOG.md",
		},
		{
			name:       "Non-GitHub module",
			modulePath: "golang.org/x/mod",
			expected:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := GenerateChangelogURL(tt.modulePath)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateCompareURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		modulePath string
		from       Version
		to         Version
		expected   string
	}{
		{
			name:       "GitHub module",
			modulePath: "github.com/gin-gonic/gin",
			from:       "v1.9.0",
			to:         "v2.0.0",
			expected:   "https://github.com/gin-gonic/gin/compare/v1.9.0...v2.0.0",
		},
		{
			name:       "GitHub module with /v2",
			modulePath: "github.com/go-chi/chi/v2",
			from:       "v2.0.0",
			to:         "v3.0.0",
			expected:   "https://github.com/go-chi/chi/compare/v2.0.0...v3.0.0",
		},
		{
			name:       "Non-GitHub module",
			modulePath: "golang.org/x/mod",
			from:       "v0.1.0",
			to:         "v1.0.0",
			expected:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := GenerateCompareURL(tt.modulePath, tt.from, tt.to)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeDirectory(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Root dot", ".", "/"},
		{"Root slash", "/", "/"},
		{"Subdirectory with leading slash", "/services/api", "/services/api"},
		{"Subdirectory without leading slash", "services/api", "/services/api"},
		{"Subdirectory with trailing slash", "/services/api/", "/services/api"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := normalizeDirectory(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUpgradeDependencyOutputs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		outputs        UpgradeDependencyOutputs
		wantUpgraded   bool
		wantGoGetError string
		wantModError   string
		wantHasChanges bool
	}{
		{
			name: "successful upgrade",
			outputs: UpgradeDependencyOutputs{
				Upgraded:   true,
				GoGetError: "",
				ModError:   "",
				HasChanges: true,
			},
			wantUpgraded:   true,
			wantGoGetError: "",
			wantModError:   "",
			wantHasChanges: true,
		},
		{
			name: "mod replace failed but has changes",
			outputs: UpgradeDependencyOutputs{
				Upgraded:   false,
				GoGetError: "",
				ModError:   "mod replace failed: exit status 1",
				HasChanges: true,
			},
			wantUpgraded:   false,
			wantGoGetError: "",
			wantModError:   "mod replace failed: exit status 1",
			wantHasChanges: true,
		},
		{
			name: "no changes detected",
			outputs: UpgradeDependencyOutputs{
				Upgraded:   false,
				GoGetError: "",
				ModError:   "",
				HasChanges: false,
			},
			wantUpgraded:   false,
			wantGoGetError: "",
			wantModError:   "",
			wantHasChanges: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.wantUpgraded, tt.outputs.Upgraded)
			assert.Equal(t, tt.wantGoGetError, tt.outputs.GoGetError)
			assert.Equal(t, tt.wantModError, tt.outputs.ModError)
			assert.Equal(t, tt.wantHasChanges, tt.outputs.HasChanges)
		})
	}
}

func TestSearchGoRepositoriesInputsDefaults(t *testing.T) {
	t.Parallel()
	inputs := SearchGoRepositoriesInputs{}
	inputs.Defaults()

	// When no env var is set, defaults to "containifyci"
	assert.Equal(t, "containifyci", inputs.Organization)
	assert.Equal(t, "Go", inputs.Language)
	assert.Equal(t, 100, inputs.PerPage)
}

func TestSearchGoRepositoriesInputsDefaults_WithValues(t *testing.T) {
	t.Parallel()
	inputs := SearchGoRepositoriesInputs{
		Organization: "custom-org",
		Language:     "Python",
		PerPage:      50,
	}
	inputs.Defaults()

	assert.Equal(t, "custom-org", inputs.Organization)
	assert.Equal(t, "Python", inputs.Language)
	assert.Equal(t, 50, inputs.PerPage)
}