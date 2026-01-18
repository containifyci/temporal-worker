# Engine-CI Temporal Workflows

This package implements a production-ready Temporal workflow for running Engine-CI jobs with per-repository singleton execution, signal-driven queuing, and automatic cleanup.

## Features

- **Per-Repo Singleton Pattern**: Only one workflow runs per repository at a time (workflow ID: `engine-ci-<repo-name>`)
- **Signal-Driven Queuing**: Multiple CI requests queue up and process sequentially
- **Scaled Concurrency**: Supports 2 workflows and 4 activities running in parallel
- **Sticky Execution**: Workflow state kept in memory for improved performance
- **Smart Cleanup**: Clone directories removed on success, preserved on failure for debugging
- **Robust Error Handling**: Individual job failures don't block the queue
- **Idle Timeout**: Workflows exit after 1 minute of inactivity

## Architecture

### Workflow: `EngineCIRepoWorkflow`

The main workflow processes Engine-CI jobs sequentially for a single repository.

**Lifecycle**:
1. Receives jobs via signals (`engine-ci-signal`)
2. Queues jobs in FIFO order
3. For each job:
   - Clones the git repository
   - Runs engine-ci with provided arguments
   - Cleans up clone directory (only on success)
4. Exits after 1 minute of no activity

**Configuration**:
- Idle timeout: 1 minute
- Global activity timeout: 45 minutes
- Per-job activity timeout: 15 minutes
- Retry policy: 30s initial, 10min max, exponential backoff

### Activities

#### 1. `CloneRepo`
Clones a git repository to a temporary directory.

**Parameters**:
- `repoURL`: Git repository URL
- `ref`: Git reference (branch/tag)

**Returns**: Working directory path (`/tmp/ci-<sanitized-repo-name>`)

**Error Handling**: Returns detailed git clone errors

#### 2. `RunEngineCI`
Executes the engine-ci binary in the cloned repository.

**Parameters**:
- `workDir`: Working directory path
- `args`: Command-line arguments for engine-ci
- `env`: Environment variables (key-value map)

**Returns**: `EngineCIDetails` with exit code and last 50 lines of output

**Exit Code Handling**: Non-zero exit codes are captured but don't fail the activity

#### 3. `CleanupRepo`
Removes the clone directory.

**Parameters**:
- `workDir`: Directory path to remove

**Returns**: Error if cleanup fails (non-critical)

**Conditional Execution**: Only called when engine-ci succeeds (exit code 0)

## Usage

### Starting a Workflow

Use the client with `--engine-ci` mode:

```bash
./temporal-worker-client --engine-ci \
  --repo https://github.com/containifyci/temporal-worker \
  --ref main \
  --args "run,-t,all"
```

With environment variables:

```bash
./temporal-worker-client --engine-ci \
  --repo https://github.com/containifyci/temporal-worker \
  --ref main \
  --args "run,-t,all" \
  --env "CI=true" \
  --env "DEBUG=1"
```

### Queuing Multiple Jobs

Send another signal to the same repo - it will queue up:

```bash
# First job starts immediately
./temporal-worker-client --engine-ci --repo https://github.com/user/repo --ref main --args "run,-t,test"

# Second job queues and runs after first completes
./temporal-worker-client --engine-ci --repo https://github.com/user/repo --ref feature --args "run,-t,lint"
```

### Running Multiple Repos in Parallel

Different repositories get separate workflows:

```bash
# These run in parallel (up to 2 workflows)
./temporal-worker-client --engine-ci --repo https://github.com/user/repo1 --ref main --args "run,-t,all"
./temporal-worker-client --engine-ci --repo https://github.com/user/repo2 --ref main --args "run,-t,all"
```

## Worker Configuration

The worker is configured with:

```go
worker.Options{
    MaxConcurrentWorkflowTaskExecutionSize: 2,   // Max 2 workflows
    MaxConcurrentActivityExecutionSize:     4,   // Max 4 activities
    StickyScheduleToStartTimeout:           10 * time.Minute,
}
```

**Pre-Flight Checks**: Worker validates `git` and `engine-ci` binaries on startup and prints their versions.

## Data Structures

### `EngineCIWorkflowInput`
```go
type EngineCIWorkflowInput struct {
    GitRepoURL string            // Git repository URL
    GitRef     string            // Git reference (branch/tag)
    RepoName   string            // Sanitized repository name
    EngineArgs []string          // Engine-CI arguments
    Env        map[string]string // Environment variables
}
```

### `EngineCIDetails`
```go
type EngineCIDetails struct {
    ExitCode    int    // Exit code from engine-ci execution
    Last50Lines string // Last 50 lines of output
}
```

## Testing

Run all tests:

```bash
go test ./pkg/workflows/engineci/... -v
```

Test coverage includes:
- ✅ Utils tests (repo name sanitization)
- ✅ Workflow tests (signal handling, queuing, idle timeout)
- ✅ Activity tests (cleanup, error handling)

## Workflow Behavior Examples

### Example 1: Successful Job

```
1. Signal received → Job queued
2. Clone repo → Success
3. Run engine-ci → Exit code 0
4. Cleanup repo → Directory removed
5. Wait for next signal (1 minute timeout)
```

### Example 2: Failed Job

```
1. Signal received → Job queued
2. Clone repo → Success
3. Run engine-ci → Exit code 1
4. Cleanup skipped → Directory preserved for debugging
5. Wait for next signal (1 minute timeout)
```

### Example 3: Multiple Jobs

```
1. Signal 1 received → Job 1 queued
2. Signal 2 received → Job 2 queued
3. Process Job 1 → Complete
4. Process Job 2 → Complete
5. Wait for next signal (1 minute timeout)
```

### Example 4: Idle Timeout

```
1. No signals received
2. Wait 1 minute
3. Workflow exits gracefully
```

## Monitoring

Use the Temporal UI to monitor workflows:

- Workflow ID format: `engine-ci-<repo-name>`
- Task Queue: `hello-world`
- View signal history, activity execution, and logs

## Troubleshooting

### Clone Directory Not Cleaned Up

**Cause**: Engine-CI job failed (non-zero exit code)

**Solution**: Directory preserved for debugging at `/tmp/ci-<repo-name>`. Check logs for details.

### Workflow Exits Too Quickly

**Cause**: Idle timeout (no signals for 1 minute)

**Solution**: This is expected behavior. Send a new signal to start a new workflow.

### Activities Taking Too Long

**Cause**: Long-running git clone or engine-ci execution

**Solution**: Activity timeout is 15 minutes per job. Increase if needed in `workflow.go`.

### Pre-Flight Check Fails

**Cause**: `git` or `engine-ci` not in PATH

**Solution**: Install missing tools or update PATH. Worker prints version information on startup.

## Upgrading

This implementation is backward compatible with existing GitHub PR workflows. Both can run simultaneously on the same worker.

## Performance

- **Memory**: ~1-5MB per workflow (sticky execution)
- **CPU**: Max 4 concurrent git/engine-ci processes
- **Disk**: Clone directories in `/tmp` (auto-cleanup on success)
- **Network**: Bounded by git clone speed

## Future Enhancements

- Global rate limiting across all repos
- Configurable idle timeout per repo
- Webhook integration for automatic triggering
- Job status query handlers
- Metrics and monitoring
- Private repository support (SSH keys, tokens)
