package golang

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"go.temporal.io/sdk/activity"
)

// SearchGoRepositoriesInputs contains the parameters for searching Go repositories
type SearchGoRepositoriesInputs struct {
	Organization string
	Language     string
	PerPage      int
}

// SearchGoRepositoriesOutputs contains the results of the repository search
type SearchGoRepositoriesOutputs struct {
	Repositories []string
	TotalCount   int
}

// gitHubSearchResponse represents the GitHub Search API response
type gitHubSearchResponse struct {
	Items []struct {
		Name string `json:"name"`
	} `json:"items"`
	TotalCount int `json:"total_count"` //nolint:tagliatelle
}

// Defaults sets default values for SearchGoRepositoriesInputs
func (i *SearchGoRepositoriesInputs) Defaults() {
	if i.Organization == "" {
		i.Organization = os.Getenv("GITHUB_ORGANIZATION")
		if i.Organization == "" {
			i.Organization = "containifyci"
		}
	}
	if i.Language == "" {
		i.Language = "Go"
	}
	if i.PerPage == 0 {
		i.PerPage = 100
	}
}

// SearchGoRepositories searches for Go repositories in the organization using GitHub Search API
func SearchGoRepositories(ctx context.Context, i SearchGoRepositoriesInputs) (SearchGoRepositoriesOutputs, error) {
	i.Defaults()
	logger := activity.GetLogger(ctx)

	logger.Info("Searching for Go repositories", "organization", i.Organization, "language", i.Language)

	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		return SearchGoRepositoriesOutputs{}, errors.New("GITHUB_TOKEN environment variable is required")
	}

	var allRepos []string
	page := 1
	totalCount := 0
	query := fmt.Sprintf("org:%s+language:%s", i.Organization, i.Language)

	for {
		searchResp, err := fetchRepositoryPage(ctx, query, page, i.PerPage, githubToken, http.DefaultClient)
		if err != nil {
			return SearchGoRepositoriesOutputs{}, err
		}

		// Set total count on first page
		if page == 1 {
			totalCount = searchResp.TotalCount
			logger.Info("Found Go repositories", "totalCount", totalCount)
		}

		// Extract repository names
		for _, item := range searchResp.Items {
			allRepos = append(allRepos, item.Name)
		}

		// Check if we have more pages
		if len(searchResp.Items) < i.PerPage {
			break
		}

		page++

		// GitHub Search API limits to 1000 results (10 pages of 100)
		if page > 10 {
			logger.Warn("Reached GitHub Search API limit of 1000 results")
			break
		}
	}

	logger.Info("Repository search completed", "reposFound", len(allRepos))
	return SearchGoRepositoriesOutputs{
		Repositories: allRepos,
		TotalCount:   totalCount,
	}, nil
}

// fetchRepositoryPage fetches a single page of repositories from GitHub Search API
func fetchRepositoryPage(ctx context.Context, query string, page, perPage int, token string, client *http.Client) (*gitHubSearchResponse, error) {
	searchURL := fmt.Sprintf("https://api.github.com/search/repositories?q=%s&per_page=%d&page=%d",
		url.QueryEscape(query), perPage, page)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute search request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Check for rate limiting
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("GitHub API rate limit exceeded (status: %d)", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var searchResp gitHubSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("failed to decode search response: %w", err)
	}

	return &searchResp, nil
}