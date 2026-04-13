package executors

import (
	"os"

	teamtools "github.com/c360studio/semteams/processor/teams-tools"
)

func init() {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		// Skip registration when no token is configured so the binary starts
		// cleanly in environments where GitHub integration is not needed.
		return
	}

	client := NewGitHubHTTPClient(token)

	if err := teamtools.RegisterTool("github_read", NewGitHubReadExecutor(client)); err != nil {
		panic("failed to register github_read tools: " + err.Error())
	}
	if err := teamtools.RegisterTool("github_write", NewGitHubWriteExecutor(client)); err != nil {
		panic("failed to register github_write tools: " + err.Error())
	}
}
