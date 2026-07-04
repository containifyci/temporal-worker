package github

import (
	"context"
	"net/http"

	github "github.com/google/go-github/v85/github"
	"golang.org/x/oauth2"
)

// NewGitHubClient creates an authenticated GitHub client using a personal access token
func NewGitHubClient(ctx context.Context, token string) *github.Client {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)
	return github.NewClient(tc)
}

// NewGitHubClientWithHTTP creates an authenticated GitHub client with a custom HTTP client
func NewGitHubClientWithHTTP(ctx context.Context, token string, httpClient *http.Client) *github.Client {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	// Create a transport that wraps the provided HTTP client's transport with OAuth2
	transport := &oauth2.Transport{
		Source: ts,
		Base:   httpClient.Transport,
	}
	oauthClient := &http.Client{
		Transport: transport,
		// Copy relevant fields from the provided client
		CheckRedirect: httpClient.CheckRedirect,
		Timeout:       httpClient.Timeout,
		Jar:           httpClient.Jar,
	}
	return github.NewClient(oauthClient)
}