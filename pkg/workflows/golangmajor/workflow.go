package golangmajor

import (
	"errors"
	"fmt"
	"strings"
	"time"

	gitactivity "github.com/containifyci/temporal-worker/pkg/activities/git"
	golangactivity "github.com/containifyci/temporal-worker/pkg/activities/golang"
	"go.temporal.io/sdk/log"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// GoMajorSweepWorkflow orchestrates major upgrades across all Go repositories
func GoMajorSweepWorkflow(ctx workflow.Context, inputs GoMajorSweepWorkflowInputs) (GoMajorSweepWorkflowOutputs, error) {
	inputs.Defaults()

	logger, sessionCtx, err := newSession(&ctx)
	if err != nil {
		return GoMajorSweepWorkflowOutputs{}, err
	}
	defer workflow.CompleteSession(sessionCtx)

	logger.Info("Starting Go Major Sweep Workflow", "organization", inputs.Organization, "maxConcurrency", inputs.MaxConcurrency, "dryRun", inputs.DryRun)

	// Search for Go repositories
	logger.Info("Searching for Go repositories")
	var searchResult golangactivity.SearchGoRepositoriesOutputs
	err = workflow.ExecuteActivity(ctx, golangactivity.SearchGoRepositories, golangactivity.SearchGoRepositoriesInputs{
		Organization: inputs.Organization,
		Language:     inputs.Language,
	}).Get(ctx, &searchResult)
	if err != nil {
		return GoMajorSweepWorkflowOutputs{}, fmt.Errorf("failed to search Go repositories: %w", err)
	}

	logger.Info("Found Go repositories", "count", len(searchResult.Repositories), "totalCount", searchResult.TotalCount)

	if inputs.DryRun {
		logger.Info("Dry run mode - skipping repository processing")
		return GoMajorSweepWorkflowOutputs{
			TotalReposProcessed: len(searchResult.Repositories),
		}, nil
	}

	// Process repositories in batches with concurrency control
	var skippedRepos []SkippedRepo
	var allResults []GoMajorUpgradeRepoWorkflowOutputs

	// Prepare list of child workflows to execute (with their full config)
	type workflowToExecute struct {
		repo                string
		dir                 string
		commitMessagePrefix string
	}
	var pendingWorkflows []workflowToExecute

	// First pass: fetch configs and prepare child workflow list
	for _, repo := range searchResult.Repositories {
		// Fetch dependabot config from GitHub
		var dependabotConfig golangactivity.FetchDependabotConfigFromGitHubOutputs
		err = workflow.ExecuteActivity(ctx, golangactivity.FetchDependabotConfigFromGitHub, golangactivity.FetchDependabotConfigFromGitHubInputs{
			Organization: inputs.Organization,
			Repository:   repo,
		}).Get(ctx, &dependabotConfig)

		if err != nil {
			// Check if error is "no dependabot config"
			if errors.Is(err, golangactivity.ErrNoDependabotConfig) {
				logger.Info("Skipping repository: no dependabot config", "repo", repo)
				skippedRepos = append(skippedRepos, SkippedRepo{
					Repository: repo,
					Reason:     "no dependabot config found",
				})
				continue
			}
			logger.Warn("Failed to fetch dependabot config", "repo", repo, "error", err)
			skippedRepos = append(skippedRepos, SkippedRepo{
				Repository: repo,
				Reason:     fmt.Sprintf("failed to fetch dependabot config: %v", err),
			})
			continue
		}

		// Check if any gomod configurations found
		if len(dependabotConfig.GoModConfigs) == 0 {
			logger.Info("Skipping repository: no gomod configurations", "repo", repo)
			skippedRepos = append(skippedRepos, SkippedRepo{
				Repository: repo,
				Reason:     "no gomod configurations in dependabot config",
			})
			continue
		}

		logger.Info("Processing repository with gomod configs", "repo", repo, "configCount", len(dependabotConfig.GoModConfigs))

		// Queue child workflow for each gomod configuration
		for _, goModConfig := range dependabotConfig.GoModConfigs {
			pendingWorkflows = append(pendingWorkflows, workflowToExecute{
				repo:                repo,
				dir:                 goModConfig.Directory,
				commitMessagePrefix: goModConfig.CommitMessagePrefix,
			})
		}
	}

	// Second pass: execute child workflows in batches
	logger.Info("Executing child workflows in batches", "totalWorkflows", len(pendingWorkflows), "maxConcurrency", inputs.MaxConcurrency)

	for i := 0; i < len(pendingWorkflows); i += inputs.MaxConcurrency {
		// Calculate batch size
		end := i + inputs.MaxConcurrency
		if end > len(pendingWorkflows) {
			end = len(pendingWorkflows)
		}
		batch := pendingWorkflows[i:end]

		logger.Info("Starting batch", "batchNumber", i/inputs.MaxConcurrency+1, "batchSize", len(batch))

		// Start child workflows in this batch
		var batchFutures []struct {
			future workflow.Future
			repo   string
			dir    string
		}

		for _, wf := range batch {
			// Create unique workflow ID including directory
			workflowID := fmt.Sprintf("GoMajorUpgrade-%s-%s-%s", wf.repo, strings.ReplaceAll(wf.dir, "/", "-"), workflow.Now(ctx).Format("20060102-150405"))

			childCtx := workflow.WithChildOptions(ctx, workflow.ChildWorkflowOptions{
				WorkflowID: workflowID,
			})

			future := workflow.ExecuteChildWorkflow(childCtx, GoMajorUpgradeRepoWorkflow, GoMajorUpgradeRepoWorkflowInputs{
				Repository:            wf.repo,
				Organization:          inputs.Organization,
				Directory:             wf.dir,
				CommitMessagePrefix:   wf.commitMessagePrefix,
				OpenPullRequestsLimit: 5,
			})

			batchFutures = append(batchFutures, struct {
				future workflow.Future
				repo   string
				dir    string
			}{
				future: future,
				repo:   wf.repo,
				dir:    wf.dir,
			})
		}

		// Wait for all workflows in this batch to complete
		for _, bf := range batchFutures {
			var result GoMajorUpgradeRepoWorkflowOutputs
			err := bf.future.Get(ctx, &result)
			if err != nil {
				logger.Warn("Child workflow failed", "repo", bf.repo, "dir", bf.dir, "error", err)
				result = GoMajorUpgradeRepoWorkflowOutputs{
					Repository: bf.repo,
					Error:      err.Error(),
				}
			}
			allResults = append(allResults, result)
		}

		logger.Info("Batch completed", "batchNumber", i/inputs.MaxConcurrency+1, "resultsCollected", len(batchFutures))
	}

	// Aggregate results
	totalPRs := 0
	var failedRepos []string
	var errorMessages []string

	for _, result := range allResults {
		if len(result.PRsCreated) > 0 {
			totalPRs += len(result.PRsCreated)
		}
		if result.Error != "" {
			failedRepos = append(failedRepos, result.Repository)
			errorMessages = append(errorMessages, fmt.Sprintf("%s: %s", result.Repository, result.Error))
		}
	}

	logger.Info("Go Major Sweep Workflow completed",
		"totalRepos", len(searchResult.Repositories),
		"processedRepos", len(allResults),
		"skippedRepos", len(skippedRepos),
		"totalPRs", totalPRs,
		"failedRepos", len(failedRepos))

	return GoMajorSweepWorkflowOutputs{
		TotalReposProcessed: len(allResults),
		TotalPRsCreated:     totalPRs,
		FailedRepos:         failedRepos,
		SkippedRepos:        skippedRepos,
		Errors:              errorMessages,
	}, nil
}

// GoMajorUpgradeRepoWorkflow processes a single repository for major upgrades
func GoMajorUpgradeRepoWorkflow(ctx workflow.Context, inputs GoMajorUpgradeRepoWorkflowInputs) (GoMajorUpgradeRepoWorkflowOutputs, error) {
	inputs.Defaults()

	logger, sessionCtx, err := newSession(&ctx)
	if err != nil {
		return GoMajorUpgradeRepoWorkflowOutputs{}, err
	}
	defer workflow.CompleteSession(sessionCtx)
	logger.Info("Starting Go Major Upgrade Repo Workflow", "repository", inputs.Repository)

	// Clone repository (clone default branch, feature branches are created per-upgrade later)
	logger.Info("Cloning repository")

	var repoPath string
	err = workflow.ExecuteActivity(sessionCtx, gitactivity.CloneRepoForUpgrade, inputs.Organization, inputs.Repository, "").Get(sessionCtx, &repoPath)
	if err != nil {
		return GoMajorUpgradeRepoWorkflowOutputs{
			Repository: inputs.Repository,
			Error:      fmt.Sprintf("failed to clone repository: %v", err),
		}, err
	}

	// Detect major upgrades
	logger.Info("Detecting major upgrades")
	var detectResult golangactivity.DetectMajorUpgradesOutputs
	err = workflow.ExecuteActivity(sessionCtx, golangactivity.DetectMajorUpgrades, golangactivity.DetectMajorUpgradesInputs{
		RepoPath:  repoPath,
		Directory: inputs.Directory,
	}).Get(sessionCtx, &detectResult)
	if err != nil {
		return GoMajorUpgradeRepoWorkflowOutputs{
			Repository: inputs.Repository,
			Error:      fmt.Sprintf("failed to detect major upgrades: %v", err),
		}, err
	}

	if len(detectResult.Upgrades) == 0 {
		logger.Info("No major upgrades found for repository")
		return GoMajorUpgradeRepoWorkflowOutputs{
			Repository:   inputs.Repository,
			UpgradeCount: 0,
		}, nil
	}

	logger.Info("Found major upgrades", "count", len(detectResult.Upgrades))

	// Count existing open major upgrade PRs
	logger.Info("Counting open major upgrade PRs")
	var openPRCount golangactivity.CountOpenMajorUpgradePRsOutputs
	err = workflow.ExecuteActivity(sessionCtx, golangactivity.CountOpenMajorUpgradePRs, golangactivity.CountOpenMajorUpgradePRsInputs{
		Organization: inputs.Organization,
		Repository:   inputs.Repository,
	}).Get(sessionCtx, &openPRCount)
	if err != nil {
		logger.Error("Failed to count open PRs", "error", err)
		return GoMajorUpgradeRepoWorkflowOutputs{
			Repository: inputs.Repository,
			Error:      fmt.Sprintf("failed to count open PRs: %v", err),
		}, err
	}

	currentOpenPRs := openPRCount.Count
	logger.Info("Current open major upgrade PRs", "count", currentOpenPRs, "limit", inputs.OpenPullRequestsLimit)

	// Process each major upgrade (create 1 PR per dependency)
	var prsCreated []string
	for _, upgrade := range detectResult.Upgrades {
		// Check if limit reached
		if currentOpenPRs >= inputs.OpenPullRequestsLimit {
			logger.Info("Open PR limit reached, stopping", "limit", inputs.OpenPullRequestsLimit, "current", currentOpenPRs)
			break
		}

		// If a specific dependency module is specified, only process that one
		if inputs.DependencyModule != "" && upgrade.FromModule != inputs.DependencyModule {
			logger.Info("Skipping upgrade (not matching specified module)", "fromModule", upgrade.FromModule, "specifiedModule", inputs.DependencyModule)
			continue
		}

		logger.Info("Processing major upgrade", "fromModule", upgrade.FromModule, "toModule", upgrade.ToModule, "from", upgrade.FromVersion, "to", upgrade.ToVersion)

		// Create branch name for this specific upgrade
		branchName := generateBranchName(upgrade)
		logger.Info("Creating branch for upgrade", "branch", branchName)

		// Check if PR already exists for this branch
		var prExists golangactivity.CheckPRExistsForBranchOutputs
		err = workflow.ExecuteActivity(sessionCtx, golangactivity.CheckPRExistsForBranch, golangactivity.CheckPRExistsForBranchInputs{
			Organization: inputs.Organization,
			Repository:   inputs.Repository,
			BranchName:   branchName,
		}).Get(sessionCtx, &prExists)
		if err != nil {
			logger.Error("Failed to check PR existence", "error", err)
			continue
		}

		if prExists.Exists {
			logger.Info("PR already exists for branch, skipping", "branch", branchName, "prNumber", prExists.PRNumber, "prURL", prExists.PRURL)
			continue
		}

		// Checkout new branch (from main/default branch)
		err = workflow.ExecuteActivity(sessionCtx, gitactivity.GitCheckoutBranch, gitactivity.GitCheckoutBranchInputs{
			RepoPath:   repoPath,
			BranchName: branchName,
			CreateNew:  true,
		}).Get(sessionCtx, nil)
		if err != nil {
			logger.Error("Failed to checkout branch", "branch", branchName, "error", err)
			continue
		}

		var upgradeResult golangactivity.UpgradeDependencyOutputs
		// Upgrade dependency
		err = workflow.ExecuteActivity(sessionCtx, golangactivity.UpgradeDependency, golangactivity.UpgradeDependencyInputs{
			RepoPath:    repoPath,
			FromModule:  upgrade.FromModule,
			ToModule:    upgrade.ToModule,
			ToVersion:   upgrade.ToVersion,
			FromVersion: upgrade.FromVersion,
			Directory:   inputs.Directory,
		}).Get(sessionCtx, &upgradeResult)
		if err != nil {
			logger.Error("UpgradeDependency activity failed", "module", upgrade.FromModule, "error", err)
			// Continue to next upgrade
			continue
		}

		// Skip if no changes at all (neither go get nor mod replace made changes)
		if !upgradeResult.HasChanges {
			logger.Info("No changes detected, skipping PR", "module", upgrade.FromModule)
			// Continue to next upgrade
			continue
		}

		// Proceed with commit and PR creation even if mod command failed
		if upgradeResult.ModError != "" {
			logger.Warn("mod replace failed but changes detected, proceeding with PR", "module", upgrade.FromModule, "error", upgradeResult.ModError)
		}

		// Commit and push changes
		commitMsg := fmt.Sprintf("%s(deps): bump %s from %s to %s", inputs.CommitMessagePrefix, upgrade.ToModule, upgrade.FromVersion, upgrade.ToVersion)
		err = workflow.ExecuteActivity(sessionCtx, gitactivity.CommitAndPush, gitactivity.CommitAndPushInputs{
			RepoPath:   repoPath,
			CommitMsg:  commitMsg,
			BranchName: branchName,
		}).Get(sessionCtx, nil)
		if err != nil {
			logger.Error("Failed to commit and push", "error", err)
			continue
		}

		// Create PR with Dependabot-style body
		prTitle := fmt.Sprintf("%s(deps): bump %s from %s to %s", inputs.CommitMessagePrefix, upgrade.ToModule, upgrade.FromVersion, upgrade.ToVersion)
		prBody := generateDependabotStylePRBody(upgrade, inputs.Repository)

		var prOut golangactivity.PRCreateOutputs
		err = workflow.ExecuteActivity(sessionCtx, golangactivity.PRCreate, golangactivity.PRCreateInputs{
			Org:         inputs.Organization,
			RepoName:    inputs.Repository,
			Branch:      branchName,
			Title:       prTitle,
			Description: prBody,
		}).Get(sessionCtx, &prOut)
		if err != nil {
			logger.Error("Failed to create PR", "error", err)
			continue
		}

		logger.Info("PR created successfully", "prID", prOut.ID, "title", prOut.Title)

		// Add DependaBot-style labels to PR
		logger.Info("Adding labels to PR", "prID", prOut.ID)
		err = workflow.ExecuteActivity(sessionCtx, golangactivity.PRAddLabels, golangactivity.PRAddLabelsInputs{
			Organization: inputs.Organization,
			Repository:   inputs.Repository,
			PRNumber:     prOut.ID,
			Labels:       []string{"dependencies", "go", "major"},
		}).Get(sessionCtx, nil)
		if err != nil {
			logger.Warn("Failed to add labels to PR", "prID", prOut.ID, "error", err)
			// Not critical - PR is still created
		}

		// Increment counter for open PRs
		currentOpenPRs++

		// Add comment about mod command failure if it occurred
		if upgradeResult.ModError != "" {
			logger.Info("Adding mod command error comment to PR", "prID", prOut.ID)

			errorMsg := fmt.Sprintf(`⚠️ **Module Upgrade Warning**

The `+"`mod replace`"+` command failed during the upgrade process. However, `+"`go get`"+` succeeded and modified go.mod/go.sum.

**Error Details:**
`+"```"+`
%s
`+"```"+`

**Action Required:**
1. Review the error message above
2. Manually verify the import path changes in the code
3. Run tests to ensure everything works
4. You may need to manually run `+"`mod replace`"+` or fix import paths

The PR was created to preserve the `+"`go get`"+` changes. You can either:
- Fix the issues and update this PR
- Close this PR if the upgrade is not viable
`, upgradeResult.ModError)

			err = workflow.ExecuteActivity(sessionCtx, golangactivity.PRComment, golangactivity.PRCommentInputs{
				ID:       prOut.ID,
				Message:  errorMsg,
				Org:      inputs.Organization,
				RepoName: inputs.Repository,
			}).Get(sessionCtx, nil)
			if err != nil {
				logger.Warn("Failed to add mod error comment to PR", "prID", prOut.ID, "error", err)
				// Not critical - PR is still created and approved
			}
		}

		// Add comment about module errors if any were detected
		if len(detectResult.ModuleErrors) > 0 {
			logger.Info("Adding module error comment to PR", "prID", prOut.ID, "errorCount", len(detectResult.ModuleErrors))

			// Format error message
			var errorMsg strings.Builder
			errorMsg.WriteString("⚠️ **Module Detection Warnings**\n\n")
			errorMsg.WriteString("The following modules had errors during detection and were skipped:\n\n")
			for _, modErr := range detectResult.ModuleErrors {
				fmt.Fprintf(&errorMsg, "- `%s`\n", modErr)
			}
			errorMsg.WriteString("\nThese modules may have issues that need manual investigation.")

			err = workflow.ExecuteActivity(sessionCtx, golangactivity.PRComment, golangactivity.PRCommentInputs{
				ID:       prOut.ID,
				Message:  errorMsg.String(),
				Org:      inputs.Organization,
				RepoName: inputs.Repository,
			}).Get(sessionCtx, nil)
			if err != nil {
				logger.Warn("Failed to add module error comment to PR", "prID", prOut.ID, "error", err)
				// Not critical - PR is still created
			}
		}

		prURL := fmt.Sprintf("https://github.com/%s/%s/pull/%d", inputs.Organization, inputs.Repository, prOut.ID)
		prsCreated = append(prsCreated, prURL)

		// Reset to main branch for next upgrade
		err = workflow.ExecuteActivity(sessionCtx, gitactivity.GitResetToMain, gitactivity.GitResetToMainInputs{
			RepoPath: repoPath,
		}).Get(sessionCtx, nil)
		if err != nil {
			logger.Error("Failed to reset to main branch", "error", err)
		}
	}

	logger.Info("Repository processing completed", "prsCreated", len(prsCreated))

	return GoMajorUpgradeRepoWorkflowOutputs{
		Repository:   inputs.Repository,
		PRsCreated:   prsCreated,
		UpgradeCount: len(detectResult.Upgrades),
	}, nil
}

// newSession creates a Temporal session context for activities that need to run on the same host
func newSession(ctx *workflow.Context) (log.Logger, workflow.Context, error) {
	logger := workflow.GetLogger(*ctx)

	options := workflowActivityOptions{
		RetryPolicyOptions: temporal.RetryPolicy{
			InitialInterval:    10 * time.Second,
			BackoffCoefficient: 1.5,
			MaximumInterval:    5 * time.Minute,
			MaximumAttempts:    5,
		},
		StartToCloseTimeout: 5 * time.Minute,
	}

	wo := workflow.ActivityOptions{
		StartToCloseTimeout: options.StartToCloseTimeout,
		RetryPolicy:         &options.RetryPolicyOptions,
	}

	*ctx = workflow.WithActivityOptions(*ctx, wo)

	// Setup session options.
	// Since we are operating on files, we need activities to always be scheduled on the same host
	so := &workflow.SessionOptions{
		CreationTimeout:  5 * time.Minute,
		ExecutionTimeout: 60 * time.Minute, // keep session open as long as possible
	}

	sessionCtx, err := workflow.CreateSession(*ctx, so)
	if err != nil {
		return logger, nil, err
	}

	return logger, sessionCtx, nil
}

// workflowActivityOptions holds configuration for workflow activity options
type workflowActivityOptions struct {
	RetryPolicyOptions  temporal.RetryPolicy
	StartToCloseTimeout time.Duration
}
