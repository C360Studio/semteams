package executors

import (
	"os"

	agentictools "github.com/c360studio/semstreams/processor/agentic-tools"
)

func init() {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		// Skip registration when no token is configured so the binary starts
		// cleanly in environments where GitHub integration is not needed.
		return
	}

	client := NewGitHubHTTPClient(token)

	if err := agentictools.RegisterTool("github_read", NewGitHubReadExecutor(client)); err != nil {
		panic("failed to register github_read tools: " + err.Error())
	}
	if err := agentictools.RegisterTool("github_write", NewGitHubWriteExecutor(client)); err != nil {
		panic("failed to register github_write tools: " + err.Error())
	}
}
