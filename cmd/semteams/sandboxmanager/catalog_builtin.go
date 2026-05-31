package sandboxmanager

import (
	"path/filepath"
	"time"
)

// BuiltinCatalog returns the three canonical hand-authored profiles
// shipped with semteams per ADR-043 §Canonical-profile catalog.
// repoRoot is the absolute path the manager resolves devcontainer
// paths against (typically the running process's working directory
// or a configured repo root).
//
// The catalog is intentionally small. New profiles get added by
// reviewer-approved PRs — extending the catalog is more friction
// than emitting LLM-rendered devcontainer JSON at request time, and
// that friction is the point (auditability + image-layer cache
// reuse + per-profile admission policy). v3 considers LLM-driven
// extension when the catalog stops covering new task classes.
func BuiltinCatalog(repoRoot string) (*Catalog, error) {
	profiles := []Profile{
		{
			Name:             "go-backend",
			Version:          "v1",
			DevcontainerPath: filepath.Join(repoRoot, ".devcontainer", "go-backend", "devcontainer.json"),
			Languages:        []string{"go"},
			Tools:            []string{"task", "gh", "jq", "git"},
			AllowedNetwork:   []NetworkPolicy{NetworkRestricted, NetworkNone},
			// No privileges — pure go-test / go-build workloads.
			TTL: 24 * time.Hour,
		},
		{
			Name:             "svelte-ui",
			Version:          "v1",
			DevcontainerPath: filepath.Join(repoRoot, ".devcontainer", "svelte-ui", "devcontainer.json"),
			Languages:        []string{"node"},
			Tools:            []string{"npm", "playwright", "git"},
			// Public network allowed because playwright browser
			// installs / playwright-test e2e against staging endpoints
			// require outbound HTTP.
			AllowedNetwork: []NetworkPolicy{NetworkRestricted, NetworkPublic, NetworkNone},
			TTL:            24 * time.Hour,
		},
		{
			Name:             "full-stack-e2e",
			Version:          "v1",
			DevcontainerPath: filepath.Join(repoRoot, ".devcontainer", "full-stack-e2e", "devcontainer.json"),
			Languages:        []string{"go", "node"},
			Tools:            []string{"task", "docker", "npm", "playwright", "git", "gh"},
			Services:         []string{"nats"},
			AllowedNetwork:   []NetworkPolicy{NetworkRestricted, NetworkPublic, NetworkNone},
			// docker-socket is THE reason this profile exists; the
			// agentic-e2e stack itself spawns sibling containers
			// (testcontainers + docker-compose-driven services).
			AllowedPrivileges: []Privilege{PrivilegeDockerSocket},
			// Shorter TTL than backend / UI: e2e stacks pull more
			// images, are more sensitive to upstream drift.
			TTL: 12 * time.Hour,
		},
	}
	return NewCatalog(profiles)
}
