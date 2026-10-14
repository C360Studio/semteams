package portfolio_test

import (
	"strings"
	"testing"

	"github.com/c360studio/semteams/cmd/semteams/portfolio"
)

func TestLoadFileExamplePortfolio(t *testing.T) {
	t.Parallel()

	p, err := portfolio.LoadFile("../../../configs/portfolio/portfolio.example.json")
	if err != nil {
		t.Fatalf("LoadFile example portfolio: %v", err)
	}
	if len(p.Programs) != 1 {
		t.Fatalf("len(Programs) = %d, want 1", len(p.Programs))
	}
	repo, ok := p.FindRepository("c360studio", "SEMTEAMS")
	if !ok {
		t.Fatalf("FindRepository(c360studio, SEMTEAMS) returned false")
	}
	if repo.Owner != "C360Studio" || repo.Name != "semteams" {
		t.Errorf("FindRepository = %s/%s, want C360Studio/semteams", repo.Owner, repo.Name)
	}
	if repo.ProjectBoard == nil || repo.ProjectBoard.Number != 3 {
		t.Errorf("ProjectBoard = %+v, want Number=3", repo.ProjectBoard)
	}
}

func TestParseValidationInvariants(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		json    string
		wantErr string
	}{
		{
			name:    "unknown top-level property",
			json:    `{"programs":[],"extra":true}`,
			wantErr: "unknown field",
		},
		{
			name:    "missing programs",
			json:    `{}`,
			wantErr: "$.programs: required",
		},
		{
			name:    "duplicate program id",
			json:    `{"programs":[{"id":"p1","name":"P1","projects":[]},{"id":"p1","name":"P2","projects":[]}]}`,
			wantErr: `duplicate program id "p1"`,
		},
		{
			name: "duplicate project id across programs",
			json: `{"programs":[
				{"id":"p1","name":"P1","projects":[{"id":"shared","name":"S1","repositories":[]}]},
				{"id":"p2","name":"P2","projects":[{"id":"shared","name":"S2","repositories":[]}]}
			]}`,
			wantErr: `duplicate project id "shared"`,
		},
		{
			name: "duplicate repo in project (case-insensitive)",
			json: `{"programs":[{"id":"p1","name":"P1","projects":[{"id":"proj1","name":"Proj","repositories":[
				{"owner":"C360Studio","name":"semteams"},
				{"owner":"c360studio","name":"SEMTEAMS"}
			]}]}]}`,
			wantErr: "listed twice in this project",
		},
		{
			name: "conflicting project_board across projects",
			json: `{"programs":[{"id":"p1","name":"P1","projects":[
				{"id":"proj1","name":"Proj1","repositories":[{"owner":"C360Studio","name":"semteams","project_board":{"owner":"C360Studio","number":1,"status_field":"Status"}}]},
				{"id":"proj2","name":"Proj2","repositories":[{"owner":"C360Studio","name":"semteams","project_board":{"owner":"C360Studio","number":2,"status_field":"Status"}}]}
			]}]}`,
			wantErr: "conflicting project_board settings",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := portfolio.Parse([]byte(tc.json))
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want substring %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestResolveRepositories(t *testing.T) {
	t.Parallel()

	p, err := portfolio.LoadFile("../../../configs/portfolio/portfolio.example.json")
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	all, err := p.ResolveRepositories("", "", "")
	if err != nil {
		t.Fatalf("ResolveRepositories all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("len(all) = %d, want 2", len(all))
	}

	one, err := p.ResolveRepositories("", "semteams", "c360studio/semsource")
	if err != nil {
		t.Fatalf("ResolveRepositories single: %v", err)
	}
	if len(one) != 1 || one[0].Name != "semsource" {
		t.Fatalf("ResolveRepositories single = %+v, want [C360Studio/semsource]", one)
	}

	if _, err := p.ResolveRepositories("", "", "other/unconfigured"); err == nil {
		t.Fatal("ResolveRepositories unconfigured repository should fail")
	}
	if _, err := p.ResolveRepositories("", "unknown-proj", ""); err == nil {
		t.Fatal("ResolveRepositories unknown project should fail")
	}
}
