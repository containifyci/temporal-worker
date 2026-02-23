# go-githubapp v0.42.0 Upgrade Notes

## Issue

The upgrade from `github.com/palantir/go-githubapp` v0.41.0 to v0.42.0 requires updating the dependency `github.com/containifyci/dunebot` to support `go-github` v83 (the new version required by go-githubapp v0.42.0).

## Root Cause

- go-githubapp v0.42.0 uses go-github v83
- go-githubapp v0.41.0 uses go-github v82
- dunebot v0.3.6 (latest released version) uses go-github v82
- The dunebot package has a type alias `Client = github.Client` pointing to v82, which is incompatible with v83

## Solution

### Temporary Fix (Current State)

The `go.mod` file contains a `replace` directive that points to a locally patched version of dunebot:

```go
replace github.com/containifyci/dunebot => /tmp/dunebot
```

This patched version has the following change in `pkg/github/client.go`:

```diff
- "github.com/google/go-github/v82/github"
+ "github.com/google/go-github/v83/github"
```

And updated dependencies in `go.mod`:
- `github.com/google/go-github/v82` → `github.com/google/go-github/v83`
- `github.com/palantir/go-githubapp v0.41.0` → `github.com/palantir/go-githubapp v0.42.0`

### Permanent Solution (Required)

To complete this upgrade, one of the following actions is needed:

**Option 1: Update dunebot repository (Recommended)**
1. Apply the changes to the `containifyci/dunebot` repository
2. Create a new release (e.g., v0.3.7)
3. Update `temporal-worker` to use the new release
4. Remove the `replace` directive from `go.mod`

**Option 2: Use a fork**
1. Create a fork of dunebot with the required changes
2. Update the `replace` directive to point to the fork
3. Release the fork with a version tag
4. Update `temporal-worker` to use the fork

## Required Change for dunebot

The minimal change required in the `containifyci/dunebot` repository:

**File: `pkg/github/client.go`**
```go
// Line 15: Change import from v82 to v83
import (
    // ... other imports ...
    "github.com/google/go-github/v83/github"  // Changed from v82
    // ... other imports ...
)
```

**File: `go.mod`**
```bash
go get github.com/google/go-github/v83@latest
go get github.com/palantir/go-githubapp@v0.42.0
go mod tidy
```

## Testing

After applying the patch locally:
- ✅ Build succeeds: `go build ./...`
- ✅ Tests pass (except unrelated network test)
- ✅ All dunebot-related packages compile correctly

## Next Steps

1. Decide on the permanent solution approach
2. Apply changes to dunebot
3. Remove the `replace` directive from this repository
4. Verify CI/CD pipeline works correctly
