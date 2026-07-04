package golangmajor

import (
	"crypto/sha256"
	"fmt"
	"os"

	golangactivity "github.com/containifyci/temporal-worker/pkg/activities/golang"
)

// generateBranchName creates a DependaBot-style branch name for the upgrade
// Format: dependabot/go_modules/major-<hash>
// Hash is deterministic based on module name and version change
func generateBranchName(upgrade golangactivity.MajorUpgrade) string {
	// Create deterministic hash from module name and versions
	input := fmt.Sprintf("%s:%s:%s", upgrade.ToModule, upgrade.FromVersion, upgrade.ToVersion)
	hash := sha256.Sum256([]byte(input))
	hashStr := fmt.Sprintf("%x", hash)[:10] // First 10 chars of hex

	return fmt.Sprintf("dependabot/go_modules/major-%s", hashStr)
}

// generateDependabotStylePRBody generates a Dependabot-style PR description
func generateDependabotStylePRBody(upgrade golangactivity.MajorUpgrade, repoName string) string {
	temporalUIHost := os.Getenv("TEMPORAL_WEB_URL")
	if temporalUIHost == "" {
		temporalUIHost = "http://localhost:8080"
	}
	temporalUILink := temporalUIHost

	template := fmt.Sprintf(`## What's Changed
Bumps %s from %s to %s. **This is a major version upgrade.**

This PR was automatically created by the Go major upgrade automation.

### 📦 Package Changes
- **Module**: %s
- **From**: %s
- **To**: %s
- **Type**: Major version upgrade

### 🔗 Useful Links
- [Release notes](%s)
- [Changelog](%s)
- [Compare versions](%s)

### ⚠️ Breaking Changes
Major version upgrades may contain breaking changes. Please review:
1. Release notes and changelog above
2. Import path changes (already updated by bot)
3. API changes in your code
4. Run full test suite before merging

### ✅ Actions Taken by Bot
- ✅ Updated go.mod and go.sum
- ✅ Rewrote import paths for new major version (/vN)
- ✅ Ran `+"`go mod tidy`"+`
- ⚠️ **CI tests not run by bot** - please verify tests pass

---

**Created by**: Go Major Upgrade Workflow
**Temporal UI**: [View workflow execution](%s)
`,
		upgrade.FromModule,
		upgrade.FromVersion,
		upgrade.ToVersion,
		upgrade.ToModule,
		upgrade.FromVersion,
		upgrade.ToVersion,
		upgrade.ReleasesURL,
		upgrade.ChangelogURL,
		upgrade.CompareURL,
		temporalUILink,
	)

	return template
}
