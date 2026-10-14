// Package portfolio loads and validates the operator-authored program, project,
// and repository configuration defined by configs/portfolio/portfolio.schema.json.
//
// Shared by the github_read product tool (#273) and the program-report pulse
// walking skeleton (#267). Enforces the same structural and uniqueness rules as
// ui/src/lib/server/work/portfolio.ts:
//   - unknown JSON properties are rejected;
//   - program IDs are unique;
//   - project IDs are unique across the entire portfolio document;
//   - a repository is listed at most once per project (case-insensitive owner/name);
//   - a repository appearing under multiple projects carries the same project_board
//     everywhere.
package portfolio

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// Portfolio is the top-level operator-authored portfolio document.
type Portfolio struct {
	Programs []Program `json:"programs"`
}

// Program groups related projects under an operator-defined portfolio boundary.
type Program struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Projects []Project `json:"projects"`
}

// Project is a bounded effort or product within a program, spanning one or more repositories.
type Project struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Repositories []Repository `json:"repositories"`
}

// Repository identifies a GitHub repository (owner/name) and optional Projects v2 board metadata.
type Repository struct {
	Owner        string        `json:"owner"`
	Name         string        `json:"name"`
	ProjectBoard *ProjectBoard `json:"project_board,omitempty"`
}

// Slug returns "Owner/Name" preserving configured casing.
func (r Repository) Slug() string {
	return r.Owner + "/" + r.Name
}

// ProjectBoard identifies an optional GitHub Project (v2) board associated with a repository.
type ProjectBoard struct {
	Owner         string  `json:"owner"`
	Number        int     `json:"number"`
	StatusField   string  `json:"status_field"`
	PriorityField *string `json:"priority_field,omitempty"`
}

// RepositoryKey returns the canonical case-insensitive key "owner/name".
func RepositoryKey(owner, name string) string {
	return strings.ToLower(strings.TrimSpace(owner) + "/" + strings.TrimSpace(name))
}

// LoadFile reads, parses, and validates a portfolio JSON document from disk.
func LoadFile(path string) (*Portfolio, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("portfolio path is empty")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read portfolio %s: %w", path, err)
	}
	p, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("invalid portfolio %s: %w", path, err)
	}
	return p, nil
}

// Parse decodes and validates a portfolio JSON document from raw bytes.
func Parse(data []byte) (*Portfolio, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var p Portfolio
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("decode portfolio JSON: %w", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("decode portfolio JSON: unexpected trailing data")
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

// Validate enforces the schema and uniqueness invariants of configs/portfolio/portfolio.schema.json.
func (p *Portfolio) Validate() error {
	if p == nil {
		return fmt.Errorf("portfolio is nil")
	}
	if p.Programs == nil {
		return fmt.Errorf("$.programs: required, must be an array")
	}

	seenPrograms := make(map[string]struct{}, len(p.Programs))
	seenProjects := make(map[string]struct{})
	boardByRepo := make(map[string]string)

	for i, prog := range p.Programs {
		progPath := fmt.Sprintf("$.programs[%d]", i)
		if !nonEmpty(prog.ID) {
			return fmt.Errorf("%s.id: required, must be a non-empty string", progPath)
		}
		if !nonEmpty(prog.Name) {
			return fmt.Errorf("%s.name: required, must be a non-empty string", progPath)
		}
		if _, dup := seenPrograms[prog.ID]; dup {
			return fmt.Errorf("%s.id: duplicate program id %q", progPath, prog.ID)
		}
		seenPrograms[prog.ID] = struct{}{}

		if prog.Projects == nil {
			return fmt.Errorf("%s.projects: required, must be an array", progPath)
		}

		for j, proj := range prog.Projects {
			projPath := fmt.Sprintf("%s.projects[%d]", progPath, j)
			if !nonEmpty(proj.ID) {
				return fmt.Errorf("%s.id: required, must be a non-empty string", projPath)
			}
			if !nonEmpty(proj.Name) {
				return fmt.Errorf("%s.name: required, must be a non-empty string", projPath)
			}
			if _, dup := seenProjects[proj.ID]; dup {
				return fmt.Errorf("%s.id: duplicate project id %q", projPath, proj.ID)
			}
			seenProjects[proj.ID] = struct{}{}

			if proj.Repositories == nil {
				return fmt.Errorf("%s.repositories: required, must be an array", projPath)
			}

			seenReposInProject := make(map[string]struct{}, len(proj.Repositories))
			for k, repo := range proj.Repositories {
				repoPath := fmt.Sprintf("%s.repositories[%d]", projPath, k)
				if err := validateRepository(repo, repoPath); err != nil {
					return err
				}
				key := RepositoryKey(repo.Owner, repo.Name)
				if _, dup := seenReposInProject[key]; dup {
					return fmt.Errorf("%s: repository %s/%s listed twice in this project", repoPath, repo.Owner, repo.Name)
				}
				seenReposInProject[key] = struct{}{}

				boardSig := boardSignature(repo.ProjectBoard)
				if prevSig, seen := boardByRepo[key]; seen && prevSig != boardSig {
					return fmt.Errorf("$.programs[*].projects[%s].repositories: repository %s/%s appears with conflicting project_board settings", proj.ID, repo.Owner, repo.Name)
				}
				boardByRepo[key] = boardSig
			}
		}
	}
	return nil
}

func validateRepository(repo Repository, path string) error {
	if !nonEmpty(repo.Owner) {
		return fmt.Errorf("%s.owner: required, must be a non-empty string", path)
	}
	if !nonEmpty(repo.Name) {
		return fmt.Errorf("%s.name: required, must be a non-empty string", path)
	}
	if repo.ProjectBoard != nil {
		b := repo.ProjectBoard
		bPath := path + ".project_board"
		if !nonEmpty(b.Owner) {
			return fmt.Errorf("%s.owner: required, must be a non-empty string", bPath)
		}
		if b.Number < 1 {
			return fmt.Errorf("%s.number: required, must be a positive integer", bPath)
		}
		if !nonEmpty(b.StatusField) {
			return fmt.Errorf("%s.status_field: required, must be a non-empty string", bPath)
		}
		if b.PriorityField != nil && !nonEmpty(*b.PriorityField) {
			return fmt.Errorf("%s.priority_field: must be a non-empty string when present", bPath)
		}
	}
	return nil
}

func boardSignature(b *ProjectBoard) string {
	if b == nil {
		return "null"
	}
	prio := ""
	if b.PriorityField != nil {
		prio = *b.PriorityField
	}
	return fmt.Sprintf("%s#%d#%s#%s", b.Owner, b.Number, b.StatusField, prio)
}

func nonEmpty(s string) bool {
	return strings.TrimSpace(s) != ""
}

// FindRepository looks up a configured repository by owner and name (case-insensitive).
func (p *Portfolio) FindRepository(owner, name string) (Repository, bool) {
	if p == nil {
		return Repository{}, false
	}
	wanted := RepositoryKey(owner, name)
	for _, prog := range p.Programs {
		for _, proj := range prog.Projects {
			for _, repo := range proj.Repositories {
				if RepositoryKey(repo.Owner, repo.Name) == wanted {
					return repo, true
				}
			}
		}
	}
	return Repository{}, false
}

// ResolveRepositories returns the deduplicated list of configured repositories
// matching the optional programID, projectID, and repository ("owner/name")
// filters. Any filter naming an unconfigured program, project, or repository
// fails closed with a descriptive error.
func (p *Portfolio) ResolveRepositories(programID, projectID, repositoryRef string) ([]Repository, error) {
	if p == nil {
		return nil, fmt.Errorf("portfolio is not configured")
	}
	programID = strings.TrimSpace(programID)
	projectID = strings.TrimSpace(projectID)
	repositoryRef = strings.TrimSpace(repositoryRef)

	var wantedRepoKey string
	if repositoryRef != "" {
		owner, name, ok := strings.Cut(repositoryRef, "/")
		if !ok || !nonEmpty(owner) || !nonEmpty(name) || strings.Contains(name, "/") {
			return nil, fmt.Errorf("repository %q must be in \"owner/name\" format", repositoryRef)
		}
		if _, found := p.FindRepository(owner, name); !found {
			return nil, fmt.Errorf("repository %s/%s is not in the configured portfolio", owner, name)
		}
		wantedRepoKey = RepositoryKey(owner, name)
	}

	var matchedProgram bool
	var matchedProject bool
	seen := make(map[string]struct{})
	out := make([]Repository, 0)

	for _, prog := range p.Programs {
		if programID != "" && prog.ID != programID {
			continue
		}
		matchedProgram = true
		for _, proj := range prog.Projects {
			if projectID != "" && proj.ID != projectID {
				continue
			}
			matchedProject = true
			for _, repo := range proj.Repositories {
				key := RepositoryKey(repo.Owner, repo.Name)
				if wantedRepoKey != "" && key != wantedRepoKey {
					continue
				}
				if _, exists := seen[key]; exists {
					continue
				}
				seen[key] = struct{}{}
				out = append(out, repo)
			}
		}
	}

	if programID != "" && !matchedProgram {
		return nil, fmt.Errorf("program %q is not in the configured portfolio", programID)
	}
	if projectID != "" && !matchedProject {
		if programID != "" {
			return nil, fmt.Errorf("project %q is not in program %q of the configured portfolio", projectID, programID)
		}
		return nil, fmt.Errorf("project %q is not in the configured portfolio", projectID)
	}
	if wantedRepoKey != "" && len(out) == 0 {
		return nil, fmt.Errorf("repository %q is not in the requested portfolio scope", repositoryRef)
	}
	return out, nil
}
