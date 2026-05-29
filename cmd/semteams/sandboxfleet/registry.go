package sandboxfleet

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/c360studio/semstreams/message"
	agentictools "github.com/c360studio/semstreams/processor/agentic-tools"
)

// State enumerates the tenant lifecycle states stored on the registry
// entity. Per ADR-042 §addendum §A:
//
//   - Provisioning  — bootstrap arc is creating the tenant (clone +
//     install in flight). Concurrent bootstrap arcs on the same
//     signature detect this state and decide(needs_clarification).
//   - ReadyRunning  — tenant container is up; docker exec works.
//   - ReadyStopped  — tenant container is stopped (workspace volume
//     persists); start-on-next-use is fast.
//   - Stale         — exists on disk but failed re-verification
//     (dep version drift, repo upstream moved). Needs re-provision.
//   - Evicting      — slated for cleanup; query_sandbox_tenant
//     returns a hit but the plan persona treats as miss.
type State string

// Tenant lifecycle state constants. The string values are the
// canonical wire form stamped onto sandbox.tenant.state triples;
// rule packs and personas reference them by exact string value.
const (
	StateProvisioning State = "provisioning"
	StateReadyRunning State = "ready_running"
	StateReadyStopped State = "ready_stopped"
	StateStale        State = "stale"
	StateEvicting     State = "evicting"
)

// Predicate constants stamped on the tenant registry entity. Keep in
// sync with the sandbox-bootstrap rule pack's $entity.triple.<name>
// references — the rules + the persona prompts read these.
const (
	PredicateState         = "sandbox.tenant.state"
	PredicateSignature     = "sandbox.tenant.signature"
	PredicateContainerName = "sandbox.tenant.container_name"
	PredicateImage         = "sandbox.tenant.image"
	PredicateWorkspace     = "sandbox.tenant.workspace"
	PredicateReadyAt       = "sandbox.tenant.ready_at"
	PredicateLastUsed      = "sandbox.tenant.last_used"
	PredicatePlanHash      = "sandbox.tenant.plan_hash"
)

// tripleSource tags the triples this package publishes. Distinct from
// the emit-tool sources so an operator grepping the graph can see what
// stamped a tenant predicate (registry mutation vs LLM-driven emit).
const tripleSource = "sandbox-fleet-registry"

// Record is the deserialized tenant state. Returned by Lookup;
// callers compare State + ReadyAt + PlanHash to decide skip vs
// re-provision per the plan persona's freshness contract.
type Record struct {
	// Signature is the canonical Hash. Pinned in the record so a
	// caller can reach the EntityID without recomputing.
	Signature string

	// State is the current lifecycle state. Empty when the record
	// does not exist (Lookup returns nil Record + nil error in that
	// case; callers check for nil).
	State State

	// ContainerName is the Docker container name for the tenant
	// ("semteams-tenant-<prefix>"). Populated on every commit path;
	// empty during the brief provisioning window before plan stamps.
	ContainerName string

	// Image is the base image:tag the tenant was provisioned from.
	Image string

	// Workspace is the absolute container-internal workspace path
	// (typically "/workspace").
	Workspace string

	// ReadyAt is the most recent commit timestamp. Used by the plan
	// persona's freshness check (TTL against now).
	ReadyAt time.Time

	// LastUsed is the most recent docker-exec or commit timestamp.
	// Used by future auto-GC.
	LastUsed time.Time

	// PlanHash is the deterministic hash of the plan fields that
	// affect provisioning (install_steps + base_image + clone_command
	// + volume_mounts). Mismatch with the candidate plan_hash
	// triggers a re-provision.
	PlanHash string
}

// TenantRegistry reads and writes tenant lifecycle state on the
// canonical 6-part tenant entity. Backed by the graph substrate
// (TriplePublisher for writes, EntityTripleReader for reads); ADR-042
// §F-5 Option B — entity-namespace storage that reuses existing
// substrate rather than introducing a new KV bucket.
//
// All methods are safe to call from multiple goroutines provided the
// underlying TriplePublisher and EntityTripleReader are safe. Both
// the framework's natsTriplePublisher and chain.NATSEntityReader are
// concurrency-safe.
type TenantRegistry struct {
	publisher agentictools.TriplePublisher
	entities  EntityTripleReader
	org       string
	platform  string
	logger    *slog.Logger
}

// EntityTripleReader matches chain.NATSEntityReader's shape. Declared
// here so the registry package does not import cmd/semteams/chain
// (avoiding a cycle if chain ever wants to read tenant state); the
// production wiring in product_tools.go passes chain.NATSEntityReader
// through this interface.
type EntityTripleReader interface {
	ReadEntity(ctx context.Context, entityID string) (map[string]any, error)
}

// NewTenantRegistry constructs a TenantRegistry bound to the given
// org + platform identity. publisher writes registry-state triples;
// entities reads them.
//
// Both publisher and entities MUST be non-nil — TenantRegistry has
// no graceful-degradation mode for missing graph substrate (the
// foundation PR is part of the boot-required wiring). Callers
// constructing without a live NATS client should not register the
// emit tools that depend on this registry.
func NewTenantRegistry(publisher agentictools.TriplePublisher, entities EntityTripleReader, org, platform string, logger *slog.Logger) *TenantRegistry {
	if publisher == nil {
		panic("sandboxfleet.NewTenantRegistry: publisher must not be nil")
	}
	if entities == nil {
		panic("sandboxfleet.NewTenantRegistry: entities must not be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &TenantRegistry{
		publisher: publisher,
		entities:  entities,
		org:       org,
		platform:  platform,
		logger:    logger,
	}
}

// Lookup returns the current registry record for a signature, or
// (nil, nil) when the entity has no triples (registry miss). Graph
// errors are propagated.
func (r *TenantRegistry) Lookup(ctx context.Context, sig TargetSignature) (*Record, error) {
	entityID, err := sig.TryEntityID(r.org, r.platform)
	if err != nil {
		return nil, fmt.Errorf("tenant entity id: %w", err)
	}
	triples, err := r.entities.ReadEntity(ctx, entityID)
	if err != nil {
		return nil, fmt.Errorf("read tenant entity %q: %w", entityID, err)
	}
	if len(triples) == 0 {
		return nil, nil
	}
	state, _ := triples[PredicateState].(string)
	if state == "" {
		// Entity exists with non-state triples only — treat as a
		// miss so the plan persona doesn't act on an incoherent
		// record. Should not happen in practice (every state
		// transition stamps PredicateState first), but the guard
		// makes the contract explicit.
		return nil, nil
	}
	rec := &Record{
		Signature: sig.Hash(),
		State:     State(state),
	}
	rec.ContainerName, _ = triples[PredicateContainerName].(string)
	rec.Image, _ = triples[PredicateImage].(string)
	rec.Workspace, _ = triples[PredicateWorkspace].(string)
	rec.PlanHash, _ = triples[PredicatePlanHash].(string)
	rec.ReadyAt = parseTimeTriple(triples[PredicateReadyAt])
	rec.LastUsed = parseTimeTriple(triples[PredicateLastUsed])
	return rec, nil
}

// MarkProvisioning transitions the registry to provisioning state
// and stamps the signature + plan_hash + plan-derived container
// name. Idempotent — emit_bootstrap_plan calls this on every
// provision/reprovision plan to land the latest plan_hash before
// the execute phase mutates host state.
//
// Does NOT stamp ContainerName / Image / Workspace — those are the
// outputs of provisioning, not inputs. They land on Commit.
func (r *TenantRegistry) MarkProvisioning(ctx context.Context, sig TargetSignature, planHash string) error {
	entityID, err := sig.TryEntityID(r.org, r.platform)
	if err != nil {
		return fmt.Errorf("tenant entity id: %w", err)
	}
	now := time.Now().UTC()
	triples := []message.Triple{
		baseTriple(entityID, PredicateState, string(StateProvisioning), now),
		baseTriple(entityID, PredicateSignature, sig.Hash(), now),
		baseTriple(entityID, PredicatePlanHash, planHash, now),
	}
	if err := r.publisher.AddTriplesBatch(ctx, triples); err != nil {
		return fmt.Errorf("stamp provisioning state for %s: %w", entityID, err)
	}
	r.logger.Info("tenant marked provisioning",
		slog.String("entity_id", entityID),
		slog.String("signature", sig.Hash()),
		slog.String("plan_hash", planHash))
	return nil
}

// Commit records the cross-run reuse-ready state: state=ready_running,
// ready_at=now, last_used=now, plan_hash, container_name, image,
// workspace. Called by emit_bootstrap_committed under
// reviewer-bootstrap's approval gate. This is the load-bearing commit
// — subsequent bootstrap arcs on the same signature take the skip
// path on the strength of these triples.
func (r *TenantRegistry) Commit(ctx context.Context, sig TargetSignature, containerName, image, workspace, planHash string) error {
	return r.CommitByHash(ctx, sig.Hash(), containerName, image, workspace, planHash)
}

// CommitByHash is the hash-keyed Commit. The reviewer-bootstrap
// persona reads the signature hash from the run entity (it was
// stamped at plan time) and echoes it back; the registry doesn't
// need the full canonical components for the commit because the
// entity ID is derived from the hash alone.
//
// Validates the hash format defensively — entity IDs constructed
// from hand-edited / hallucinated hashes that contain dots would
// panic the upstream IsValidEntityID guard. Returns a typed error
// instead, so the tool executor can surface ToolErrorInvalidArgs.
func (r *TenantRegistry) CommitByHash(ctx context.Context, sigHash, containerName, image, workspace, planHash string) error {
	entityID, err := entityIDFromHash(r.org, r.platform, sigHash)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339Nano)
	triples := []message.Triple{
		baseTriple(entityID, PredicateState, string(StateReadyRunning), now),
		baseTriple(entityID, PredicateSignature, sigHash, now),
		baseTriple(entityID, PredicateContainerName, containerName, now),
		baseTriple(entityID, PredicateImage, image, now),
		baseTriple(entityID, PredicateWorkspace, workspace, now),
		baseTriple(entityID, PredicatePlanHash, planHash, now),
		baseTriple(entityID, PredicateReadyAt, nowStr, now),
		baseTriple(entityID, PredicateLastUsed, nowStr, now),
	}
	if err := r.publisher.AddTriplesBatch(ctx, triples); err != nil {
		return fmt.Errorf("commit tenant %s: %w", entityID, err)
	}
	r.logger.Info("tenant committed ready_running",
		slog.String("entity_id", entityID),
		slog.String("signature", sigHash),
		slog.String("container_name", containerName))
	return nil
}

// entityIDFromHash builds the canonical 6-part tenant entity ID
// from a precomputed hash (the registry's cache key). Used by the
// hash-keyed mutation paths (CommitByHash) where the full
// TargetSignature is not in scope.
func entityIDFromHash(org, platform, sigHash string) (string, error) {
	if org == "" || platform == "" {
		return "", fmt.Errorf("entity id requires non-empty org + platform")
	}
	if sigHash == "" {
		return "", fmt.Errorf("entity id requires non-empty signature hash")
	}
	for _, r := range sigHash {
		if r == '.' {
			return "", fmt.Errorf("signature hash %q contains dots; entity-id separator", sigHash)
		}
	}
	id := fmt.Sprintf("%s.%s.sandbox.tenant.execution.%s", org, platform, sigHash)
	if !message.IsValidEntityID(id) {
		return "", fmt.Errorf("constructed tenant entity id %q failed IsValidEntityID", id)
	}
	return id, nil
}

// MarkState writes a single state-transition triple. Used by ad-hoc
// transitions (ready_running ↔ ready_stopped on docker start/stop,
// → stale on verification failure, → evicting on GC) that don't
// touch the full attribute set.
func (r *TenantRegistry) MarkState(ctx context.Context, sig TargetSignature, state State) error {
	entityID, err := sig.TryEntityID(r.org, r.platform)
	if err != nil {
		return fmt.Errorf("tenant entity id: %w", err)
	}
	now := time.Now().UTC()
	if err := r.publisher.AddTriple(ctx, baseTriple(entityID, PredicateState, string(state), now)); err != nil {
		return fmt.Errorf("mark tenant %s state %s: %w", entityID, state, err)
	}
	r.logger.Info("tenant state transition",
		slog.String("entity_id", entityID),
		slog.String("state", string(state)))
	return nil
}

// TouchLastUsed updates only the last_used timestamp. Called by
// docker-exec-on-warm-tenant paths so auto-GC can rank by recency.
// Cheap; one triple per call.
func (r *TenantRegistry) TouchLastUsed(ctx context.Context, sig TargetSignature) error {
	entityID, err := sig.TryEntityID(r.org, r.platform)
	if err != nil {
		return fmt.Errorf("tenant entity id: %w", err)
	}
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339Nano)
	if err := r.publisher.AddTriple(ctx, baseTriple(entityID, PredicateLastUsed, nowStr, now)); err != nil {
		return fmt.Errorf("touch tenant %s last_used: %w", entityID, err)
	}
	return nil
}

func baseTriple(subject, predicate string, object any, now time.Time) message.Triple {
	return message.Triple{
		Subject:    subject,
		Predicate:  predicate,
		Object:     object,
		Source:     tripleSource,
		Timestamp:  now,
		Confidence: 1.0,
	}
}

// parseTimeTriple is a best-effort RFC3339 parser. Graph triples
// arrive untyped through the JSON wire; the registry stamps
// timestamps as RFC3339Nano strings.
//
// Numeric inputs (Unix epoch as float64 / int / json.Number — a
// reasonable mistake by an operator stamping a freshness override)
// are treated as ABSENT, not as a Unix-epoch fallback. The
// rationale: silently accepting numeric epoch would allow two
// different stamp conventions to coexist in the graph and produce
// timezone-confusion bugs at freshness-check time. The fail-explicit
// outcome (IsFresh reports "ready_at missing" and the plan persona
// re-provisions) is preferable. Operators stamping freshness on a
// tenant must use RFC3339.
//
// On any parse failure returns the zero time so callers can
// distinguish "absent" via IsZero.
func parseTimeTriple(v any) time.Time {
	s, ok := v.(string)
	if !ok || s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return time.Time{}
}

// IsFresh reports whether a Record is fresh enough to skip
// re-provisioning per the plan persona's freshness contract:
// state is ready_running or ready_stopped, ready_at is within ttl,
// and the candidate plan_hash matches.
//
// Returns (fresh, reason). reason is empty when fresh=true and
// human-readable when fresh=false so the plan persona can echo it
// into decide.reason for audit.
func (rec *Record) IsFresh(candidatePlanHash string, ttl time.Duration, now time.Time) (bool, string) {
	if rec == nil {
		return false, "registry miss"
	}
	if rec.State != StateReadyRunning && rec.State != StateReadyStopped {
		return false, fmt.Sprintf("registry state=%s (not ready)", rec.State)
	}
	if rec.PlanHash != candidatePlanHash {
		return false, fmt.Sprintf("plan_hash drift: cached=%s candidate=%s", rec.PlanHash, candidatePlanHash)
	}
	if rec.ReadyAt.IsZero() {
		return false, "ready_at missing"
	}
	if now.Sub(rec.ReadyAt) > ttl {
		return false, fmt.Sprintf("ready_at=%s older than ttl=%s", rec.ReadyAt.Format(time.RFC3339), ttl)
	}
	return true, ""
}

// HashAsContainerSuffix returns the leading 12 chars suitable as a
// container name suffix. Helper for tests + callers that want the
// derived value without rebuilding a TargetSignature.
func HashAsContainerSuffix(hash string) string {
	if len(hash) >= 12 {
		return hash[:12]
	}
	return hash
}
