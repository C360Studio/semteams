package emitchange

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/c360studio/semteams/cmd/semteams/openspec"
	"github.com/c360studio/semteams/cmd/semteams/slug"
)

// The parsed+validated emit_change payload. The shape mirrors an OpenSpec change
// (proposal + optional design + per-capability deltas + tasks) plus the
// chain-level acceptance_command. Tasks are the §D6 superset (OpenSpec-visible
// text/section/number + the execution-rich goal/target_files/test_command/…).

type changePayload struct {
	Slug              string           `json:"slug"`
	Proposal          *proposalPayload `json:"proposal"`
	Design            *designPayload   `json:"design"`
	Deltas            []deltaPayload   `json:"deltas"`
	Tasks             []taskPayload    `json:"tasks"`
	AcceptanceCommand string           `json:"acceptance_command"`
}

type proposalPayload struct {
	Intent   string   `json:"intent"`
	ScopeIn  []string `json:"scope_in"`
	ScopeOut []string `json:"scope_out"`
	Approach string   `json:"approach"`
}

type designPayload struct {
	TechnicalApproach string              `json:"technical_approach"`
	Decisions         []decisionPayload   `json:"decisions"`
	DataFlow          string              `json:"data_flow"`
	FileChanges       []fileChangePayload `json:"file_changes"`
}

type decisionPayload struct {
	Name string `json:"name"`
	Body string `json:"body"`
}

type fileChangePayload struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

type deltaPayload struct {
	Capability string               `json:"capability"`
	Added      []requirementPayload `json:"added"`
	Modified   []modifiedReqPayload `json:"modified"`
	Removed    []removedReqPayload  `json:"removed"`
}

type requirementPayload struct {
	Name      string            `json:"name"`
	Statement string            `json:"statement"`
	Scenarios []scenarioPayload `json:"scenarios"`
}

type modifiedReqPayload struct {
	requirementPayload
	Previously string `json:"previously"`
}

type removedReqPayload struct {
	Name      string `json:"name"`
	Rationale string `json:"rationale"`
}

type scenarioPayload struct {
	Name  string        `json:"name"`
	Steps []stepPayload `json:"steps"`
}

type stepPayload struct {
	Keyword string `json:"kw"`
	Text    string `json:"text"`
}

type taskPayload struct {
	Section         string   `json:"section"`
	Number          string   `json:"number"`
	Text            string   `json:"text"`
	Done            bool     `json:"done"`
	Goal            string   `json:"goal"`
	TargetFiles     []string `json:"target_files"`
	TestCommand     string   `json:"test_command"`
	Assumptions     []string `json:"assumptions"`
	NonGoals        []string `json:"non_goals"`
	ExpectedOutcome string   `json:"expected_outcome"`
	RequirementRef  string   `json:"requirement_ref"`
}

func parseArgs(raw map[string]any) (*changePayload, error) {
	if raw == nil {
		return nil, fmt.Errorf("arguments are required")
	}
	buf, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal arguments: %w", err)
	}
	var p changePayload
	dec := json.NewDecoder(strings.NewReader(string(buf)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("unmarshal arguments: %w", err)
	}
	if err := p.validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

// validate enforces the ADR-057 discipline structurally
// ([[encode-principles-structurally]]): a SHALL-class statement + a WHEN/THEN
// scenario per requirement (§D3), and the execution-rich superset per task (§D6).
func (p *changePayload) validate() error {
	if !slugPattern.MatchString(p.Slug) {
		return fmt.Errorf("slug %q must be lower-kebab-case (1-64 chars, [a-z0-9-], leading alphanumeric)", p.Slug)
	}
	if err := p.validateProposal(); err != nil {
		return err
	}
	if err := p.validateDeltas(); err != nil {
		return err
	}
	if err := p.validateTasks(); err != nil {
		return err
	}
	if err := p.validateAndNormalizeRefs(); err != nil {
		return err
	}
	if trim(p.AcceptanceCommand) == "" {
		return fmt.Errorf("acceptance_command is required (the chain-level full-suite gate CBG re-runs — ADR-057 §D1/§D6)")
	}
	return nil
}

// validateAndNormalizeRefs enforces requirement-key sanity across the deltas +
// task links (the §D6 task→requirement linkage). Two jobs:
//
//   - Per-capability <rid> uniqueness. <rid> = slug.Slugify(name); if two
//     requirements in one capability slugify equal, openspec.Change.Facts would
//     silently disambiguate the second with a "-2" suffix the author can't
//     predict — so reject it here. With uniqueness enforced, every rid is exactly
//     Slugify(name), so a normalized ref is stable.
//   - requirement_ref resolution + normalization. The author writes
//     "<capability>/<requirement name>"; this checks the capability has a delta
//     and the name matches an ADDED/MODIFIED requirement there (a task implements
//     a present requirement, not a removed one), then rewrites the stored ref to
//     "<capability>/<rid>" so it directly indexes delta.<cap>.<rid>. A dangling
//     ref is rejected at emit time rather than stamped as a silent no-op.
func (p *changePayload) validateAndNormalizeRefs() error {
	// allRIDs: every rid in a capability (all ops) — uniqueness + collision msg.
	// implementable: rids a task may reference (added + modified only).
	allRIDs := map[string]map[string]string{}
	implementable := map[string]map[string]bool{}
	rid := func(name string) string {
		s := slug.Slugify(name)
		if s == "" {
			s = "requirement" // mirror openspec's empty-slug fallback
		}
		return s
	}
	for di, d := range p.Deltas {
		allRIDs[d.Capability] = map[string]string{}
		implementable[d.Capability] = map[string]bool{}
		claim := func(name string, impl bool) error {
			r := rid(name)
			if prev, dup := allRIDs[d.Capability][r]; dup {
				return fmt.Errorf("deltas[%d=%s]: requirements %q and %q both reduce to rid %q; requirement names must be distinct within a capability", di, d.Capability, prev, name, r)
			}
			allRIDs[d.Capability][r] = name
			if impl {
				implementable[d.Capability][r] = true
			}
			return nil
		}
		for _, r := range d.Added {
			if err := claim(r.Name, true); err != nil {
				return err
			}
		}
		for _, m := range d.Modified {
			if err := claim(m.Name, true); err != nil {
				return err
			}
		}
		for _, rm := range d.Removed {
			if err := claim(rm.Name, false); err != nil {
				return err
			}
		}
	}

	for i := range p.Tasks {
		ref := trim(p.Tasks[i].RequirementRef)
		if ref == "" {
			continue
		}
		capName, name, ok := strings.Cut(ref, "/")
		if !ok || capName == "" || trim(name) == "" {
			return fmt.Errorf("tasks[%d].requirement_ref %q must be '<capability>/<requirement-name>'", i, ref)
		}
		if _, known := implementable[capName]; !known {
			return fmt.Errorf("tasks[%d].requirement_ref %q: no delta for capability %q in this change", i, ref, capName)
		}
		r := rid(name)
		if !implementable[capName][r] {
			return fmt.Errorf("tasks[%d].requirement_ref %q: capability %q has no added/modified requirement %q (a task implements a present requirement)", i, ref, capName, name)
		}
		p.Tasks[i].RequirementRef = capName + "/" + r // normalize to the rid key
	}
	return nil
}

func (p *changePayload) validateProposal() error {
	if p.Proposal == nil {
		return fmt.Errorf("proposal is required")
	}
	if trim(p.Proposal.Intent) == "" {
		return fmt.Errorf("proposal.intent is required (why the change matters)")
	}
	if trim(p.Proposal.Approach) == "" {
		return fmt.Errorf("proposal.approach is required (high-level direction)")
	}
	if p.Proposal.ScopeIn == nil {
		return fmt.Errorf("proposal.scope_in is required (emit [] explicitly if nothing in scope)")
	}
	if p.Proposal.ScopeOut == nil {
		return fmt.Errorf("proposal.scope_out is required (emit [] explicitly if nothing excluded)")
	}
	return nil
}

func (p *changePayload) validateDeltas() error {
	if len(p.Deltas) == 0 {
		return fmt.Errorf("deltas must contain at least one capability delta")
	}
	requirementCount := 0
	seenCap := map[string]bool{}
	for di, d := range p.Deltas {
		if err := validateCapabilityName(d.Capability); err != nil {
			return fmt.Errorf("deltas[%d].capability: %w", di, err)
		}
		if seenCap[d.Capability] {
			return fmt.Errorf("deltas[%d].capability %q is duplicated; one delta per capability", di, d.Capability)
		}
		seenCap[d.Capability] = true

		for ai, r := range d.Added {
			if err := validateRequirement(r); err != nil {
				return fmt.Errorf("deltas[%d=%s].added[%d]: %w", di, d.Capability, ai, err)
			}
		}
		for mi, m := range d.Modified {
			if err := validateRequirement(m.requirementPayload); err != nil {
				return fmt.Errorf("deltas[%d=%s].modified[%d]: %w", di, d.Capability, mi, err)
			}
			if trim(m.Previously) == "" {
				return fmt.Errorf("deltas[%d=%s].modified[%d].previously is required (the prior statement — OpenSpec '(Previously: …)')", di, d.Capability, mi)
			}
		}
		for ri, rm := range d.Removed {
			if trim(rm.Name) == "" {
				return fmt.Errorf("deltas[%d=%s].removed[%d].name is required", di, d.Capability, ri)
			}
			if trim(rm.Rationale) == "" {
				return fmt.Errorf("deltas[%d=%s].removed[%d].rationale is required (why deprecated — OpenSpec '(Rationale: …)')", di, d.Capability, ri)
			}
		}
		requirementCount += len(d.Added) + len(d.Modified) + len(d.Removed)
	}
	if requirementCount == 0 {
		return fmt.Errorf("a change must add, modify, or remove at least one requirement across its deltas")
	}
	return nil
}

// validateRequirement enforces §D3: an RFC-2119 SHALL-class statement and ≥1
// scenario carrying at least a WHEN and a THEN.
func validateRequirement(r requirementPayload) error {
	if trim(r.Name) == "" {
		return fmt.Errorf("requirement name is required")
	}
	if !shallPattern.MatchString(r.Statement) {
		return fmt.Errorf("requirement %q statement must be an RFC-2119 statement containing SHALL/MUST/SHOULD/MAY (got %q)", r.Name, r.Statement)
	}
	if len(r.Scenarios) == 0 {
		return fmt.Errorf("requirement %q must carry at least one Given/When/Then scenario (ADR-057 §D3)", r.Name)
	}
	for si, s := range r.Scenarios {
		var hasWhen, hasThen bool
		for _, step := range s.Steps {
			switch strings.ToUpper(trim(step.Keyword)) {
			case "WHEN":
				hasWhen = true
			case "THEN":
				hasThen = true
			}
		}
		if !hasWhen || !hasThen {
			return fmt.Errorf("requirement %q scenario[%d] %q must have at least a WHEN and a THEN step (ADR-057 §D3)", r.Name, si, s.Name)
		}
	}
	return nil
}

// validateTasks enforces the §D6 execution-rich superset and section
// contiguity (a section name, once left, may not reappear). Contiguity keeps the
// grouping faithful to a clean tasks.md for graph round-trip reconstruction; it
// is NOT what aligns the writer-half task index with the content half (that
// follows from groupTaskSections being order-preserving — see toChange).
func (p *changePayload) validateTasks() error {
	if len(p.Tasks) == 0 {
		return fmt.Errorf("tasks must contain at least one task")
	}
	closedSections := map[string]bool{}
	prevSection := ""
	for i, t := range p.Tasks {
		if i == 0 || t.Section != prevSection {
			if closedSections[t.Section] {
				return fmt.Errorf("tasks[%d].section %q is non-contiguous; group all tasks of a section together", i, t.Section)
			}
			if i > 0 {
				closedSections[prevSection] = true
			}
		}
		prevSection = t.Section

		if trim(t.Text) == "" {
			return fmt.Errorf("tasks[%d].text is required (the OpenSpec checkbox prose)", i)
		}
		if trim(t.Goal) == "" {
			return fmt.Errorf("tasks[%d].goal is required (§D6 — the execution goal Ralph drives to)", i)
		}
		if len(t.TargetFiles) == 0 {
			return fmt.Errorf("tasks[%d].target_files must contain at least one path (§D6 — surgical scope)", i)
		}
		if trim(t.TestCommand) == "" {
			return fmt.Errorf("tasks[%d].test_command is required (§D6 — the executable acceptance command)", i)
		}
		if t.Assumptions == nil {
			return fmt.Errorf("tasks[%d].assumptions is required (emit [] explicitly)", i)
		}
		if t.NonGoals == nil {
			return fmt.Errorf("tasks[%d].non_goals is required (emit [] explicitly)", i)
		}
	}
	return nil
}

// validateCapabilityName rejects a capability that is not a safe single path
// segment (it becomes both a predicate fragment and a specs/<cap>/ folder name).
func validateCapabilityName(capability string) error {
	if capability == "" {
		return fmt.Errorf("capability is required")
	}
	if strings.ContainsAny(capability, `/\`) || strings.Contains(capability, "..") || capability == "." {
		return fmt.Errorf("capability %q must be a single safe path segment", capability)
	}
	return nil
}

// --- payload -> pure openspec model projections (the content half) ---

func (d *designPayload) toModel() *openspec.Design {
	out := &openspec.Design{
		TechnicalApproach: d.TechnicalApproach,
		DataFlow:          d.DataFlow,
	}
	for _, dec := range d.Decisions {
		out.Decisions = append(out.Decisions, openspec.DesignDecision{Name: dec.Name, Body: dec.Body})
	}
	for _, fc := range d.FileChanges {
		out.FileChanges = append(out.FileChanges, openspec.FileChange{Path: fc.Path, Kind: fc.Kind})
	}
	return out
}

func (d deltaPayload) toModel() openspec.Delta {
	out := openspec.Delta{Capability: d.Capability}
	for _, r := range d.Added {
		out.Added = append(out.Added, r.toModel())
	}
	for _, m := range d.Modified {
		out.Modified = append(out.Modified, openspec.ModifiedRequirement{
			Requirement: m.requirementPayload.toModel(),
			Previously:  m.Previously,
		})
	}
	for _, rm := range d.Removed {
		out.Removed = append(out.Removed, openspec.RemovedRequirement{Name: rm.Name, Rationale: rm.Rationale})
	}
	return out
}

func (r requirementPayload) toModel() openspec.Requirement {
	out := openspec.Requirement{Name: r.Name, Statement: r.Statement}
	for _, s := range r.Scenarios {
		sc := openspec.Scenario{Name: s.Name}
		for _, step := range s.Steps {
			sc.Steps = append(sc.Steps, openspec.Step{Keyword: strings.ToUpper(trim(step.Keyword)), Text: step.Text})
		}
		out.Scenarios = append(out.Scenarios, sc)
	}
	return out
}
