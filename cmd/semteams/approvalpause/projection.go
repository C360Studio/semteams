package approvalpause

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/c360studio/semstreams/pkg/lifecycle"
	"github.com/c360studio/semstreams/pkg/projection"
	semtypes "github.com/c360studio/semstreams/pkg/types"
	"github.com/google/uuid"
)

const (
	// MarkerApprovalOutstanding is derived atomically from pending minus answered.
	MarkerApprovalOutstanding = "agent.run.approval-outstanding"
	// This is the documented canonical mutation operation, not a fallback port.
	// See ADR-029's approval projection addendum for the public-client gap.
	approvalReconcileSubject = "graph.mutation.entity.reconcile"
	maxProjectionAttempts    = 8
	projectionTimeout        = 5 * time.Second
)

type approvalRequester interface {
	RequestClassified(context.Context, string, []byte, time.Duration) ([]byte, error)
}

// NATSProjection records approval facts on an existing local run through the
// canonical graph owner. It owns no store and never writes lifecycle predicates.
type NATSProjection struct {
	reader        graph.ExactEntityReader
	requester     approvalRequester
	org, platform string
}

// NewNATSProjection binds the three-predicate product contract to this deployment.
// The composition must already declare the graph-ingest canonical mutation port.
func NewNATSProjection(client *natsclient.Client, org, platform string) (*NATSProjection, error) {
	if client == nil {
		return nil, errors.New("approval projection requires NATS")
	}
	return newProjection(graph.NewExactEntityReader(client, projectionTimeout), client, org, platform)
}
func newProjection(reader graph.ExactEntityReader, requester approvalRequester, org, platform string) (*NATSProjection, error) {
	contract := projection.Contract{Name: "semteams-run-approval", MessageType: lifecycle.HarnessMessageType(), EntityPattern: org + "." + platform + ".chain.agent.execution.*", Groups: []projection.PredicateGroup{{Name: "approval", Mode: projection.ModeReconcile, Predicates: approvalPredicates()}}}
	if err := contract.Validate(); err != nil {
		return nil, fmt.Errorf("approval projection contract: %w", err)
	}
	return &NATSProjection{reader: reader, requester: requester, org: org, platform: platform}, nil
}
func approvalPredicates() []string {
	return []string{MarkerApprovalPending, MarkerApprovalAnswered, MarkerApprovalOutstanding}
}

// RecordApproval uses one exact read's revision to fence the facts computed from
// that read. Only a definite revision mismatch is retried. An ambiguous write is
// never guessed successful from matching content or automatically replayed.
func (p *NATSProjection) RecordApproval(ctx context.Context, runID, loopID, executionID string, answered bool) (bool, error) {
	if ctx == nil {
		return false, errors.New("approval projection requires context")
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := p.validateRun(runID); err != nil {
		return false, err
	}
	encoded, _ := json.Marshal([2]string{loopID, executionID})
	key := string(encoded)
	if err := validatePair(key); err != nil {
		return false, err
	}
	ctx, cancel := context.WithTimeout(ctx, projectionTimeout)
	defer cancel()
	// One logical event has stable provenance even when a CAS conflict forces a
	// fresh read. Previously stored tuple annotations are copied unchanged.
	now, requestID := time.Now().UTC(), uuid.NewString()
	for attempt := 0; attempt < maxProjectionAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		exact, err := p.reader.ReadExactEntity(ctx, runID)
		if err != nil {
			return false, fmt.Errorf("read approval run: %w", err)
		}
		if exact == nil || exact.KVRevision == 0 {
			return false, errors.New("approval authority has no exact revision")
		}
		if err := p.validateEntity(exact.Entity, runID); err != nil {
			return false, err
		}
		desired, already, err := approvalFacts(exact.Entity, key, answered, now)
		if err != nil {
			return false, err
		}
		request := graph.ReconcilePredicatesRequest{EntityID: runID, ExpectedRevision: exact.KVRevision, Predicates: approvalPredicates(), Desired: desired, RequestID: requestID}
		data, err := json.Marshal(request)
		if err != nil {
			return false, fmt.Errorf("encode approval projection: %w", err)
		}
		if err := ctx.Err(); err != nil {
			return false, refusedProjection(err)
		}
		reply, err := p.requester.RequestClassified(ctx, approvalReconcileSubject, data, projectionTimeout)
		if err != nil {
			if errors.Is(err, errs.ErrRevisionMismatch) {
				continue
			}
			// A classified server refusal or no responder proves no commit. Other
			// transport failures after dispatch cannot prove whether the owner wrote.
			var classified *errs.ClassifiedError
			if errors.As(err, &classified) || natsclient.IsNoResponders(err) {
				return false, refusedProjection(err)
			}
			return false, unknownProjection(err)
		}
		var response graph.ReconcilePredicatesResponse
		if err := json.Unmarshal(reply, &response); err != nil {
			return false, unknownProjection(err)
		}
		validRevision := (response.Outcome == graph.MutationApplied && response.KVRevision > exact.KVRevision) ||
			(response.Outcome == graph.MutationUnchanged && response.KVRevision == exact.KVRevision)
		if !validRevision || response.RequestID != requestID {
			return false, unknownProjection(errors.New("invalid approval mutation receipt"))
		}
		if err := p.validateEntity(response.Entity, runID); err != nil {
			return false, unknownProjection(err)
		}
		if !sameApprovalFacts(desired, response.Entity.Triples) {
			return false, unknownProjection(errors.New("approval mutation receipt differs from desired facts"))
		}
		return already, nil
	}
	return false, fmt.Errorf("approval projection conflict after %d attempts: %w", maxProjectionAttempts, errs.ErrRevisionMismatch)
}
func (p *NATSProjection) validateRun(id string) error {
	parsed, err := semtypes.ParseEntityID(id)
	if err != nil {
		return fmt.Errorf("approval run identity: %w", err)
	}
	if parsed.Org != p.org || parsed.Platform != p.platform || parsed.System != "chain" || parsed.Domain != "agent" || parsed.Type != "execution" {
		return fmt.Errorf("approval run %q is outside the local run contract", id)
	}
	return nil
}
func (p *NATSProjection) validateEntity(entity *graph.EntityState, id string) error {
	if entity == nil || entity.ID != id || entity.MessageType != lifecycle.HarnessMessageType() {
		return errors.New("approval exact entity identity/type mismatch")
	}
	return nil
}
func unknownProjection(err error) error {
	return &projection.MutationError{Operation: projection.MutationOperationReconcile, Kind: projection.MutationCommitUnknown, Commit: projection.CommitUnknown, Err: err}
}

func refusedProjection(err error) error {
	failure := &projection.MutationError{Operation: projection.MutationOperationReconcile, Kind: projection.MutationUnavailable, Commit: projection.CommitNotCommitted, Class: errs.Classify(err), Err: err}
	var classified *errs.ClassifiedError
	if errors.As(err, &classified) {
		failure.Class, failure.Code = classified.Class, classified.Code
		switch classified.Class {
		case errs.ErrorInvalid:
			failure.Kind = projection.MutationInvalid
		case errs.ErrorFatal:
			failure.Kind = projection.MutationInternal
		}
	}
	return failure
}

func validatePair(value string) error {
	var pair []string
	if err := json.Unmarshal([]byte(value), &pair); err != nil || len(pair) != 2 || pair[1] == "" {
		return errors.New("approval fact is not a loop/execution JSON pair")
	}
	id, err := uuid.Parse(pair[0])
	if err != nil || id.String() != pair[0] {
		return errors.New("approval pair has a noncanonical loop token")
	}
	canonical, _ := json.Marshal([2]string{pair[0], pair[1]})
	if string(canonical) != value {
		return errors.New("approval pair has a noncanonical JSON encoding")
	}
	return nil
}

func approvalFacts(entity *graph.EntityState, key string, answer bool, now time.Time) ([]message.Triple, bool, error) {
	pending, answered := map[string]message.Triple{}, map[string]message.Triple{}
	var priorCount *message.Triple
	for _, tr := range entity.Triples {
		if tr.Predicate == MarkerApprovalOutstanding {
			if priorCount != nil || tr.Subject != entity.ID {
				return nil, false, errors.New("invalid approval outstanding projection")
			}
			copy := tr
			priorCount = &copy
			continue
		}
		var target map[string]message.Triple
		switch tr.Predicate {
		case MarkerApprovalPending:
			target = pending
		case MarkerApprovalAnswered:
			target = answered
		default:
			continue
		}
		value, ok := tr.Object.(string)
		if !ok || tr.Subject != entity.ID {
			return nil, false, errors.New("invalid approval tuple")
		}
		if err := validatePair(value); err != nil {
			return nil, false, err
		}
		if _, duplicate := target[value]; duplicate {
			return nil, false, errors.New("duplicate stored approval tuple")
		}
		target[value] = tr
	}
	for value := range answered {
		if _, ok := pending[value]; !ok {
			return nil, false, errors.New("answered approval absent from pending set")
		}
	}
	previous := len(pending) - len(answered)
	if priorCount != nil && !countEquals(priorCount.Object, previous) {
		return nil, false, errors.New("approval outstanding projection contradicts receipt sets")
	}
	if priorCount == nil && (len(pending) > 0 || len(answered) > 0) {
		return nil, false, errors.New("approval history has no outstanding projection; fresh storage required")
	}
	_, already := answered[key]
	makeFact := func(predicate string, object any) message.Triple {
		return message.Triple{Subject: entity.ID, Predicate: predicate, Object: object, Source: pauserSource, Timestamp: now, Confidence: 1}
	}
	if _, ok := pending[key]; !ok {
		pending[key] = makeFact(MarkerApprovalPending, key)
	}
	if answer && !already {
		answered[key] = makeFact(MarkerApprovalAnswered, key)
	}
	desired := make([]message.Triple, 0, len(pending)+len(answered)+1)
	for _, set := range []map[string]message.Triple{pending, answered} {
		keys := make([]string, 0, len(set))
		for value := range set {
			keys = append(keys, value)
		}
		sort.Strings(keys)
		for _, value := range keys {
			desired = append(desired, set[value])
		}
	}
	count := len(pending) - len(answered)
	if priorCount != nil && previous == count {
		desired = append(desired, *priorCount)
	} else {
		desired = append(desired, makeFact(MarkerApprovalOutstanding, count))
	}
	return desired, already, nil
}
func countEquals(value any, count int) bool {
	switch v := value.(type) {
	case int:
		return v == count
	case float64:
		return v == float64(count)
	default:
		return false
	}
}
func sameApprovalFacts(desired, actual []message.Triple) bool {
	selected := []message.Triple{}
	for _, tr := range actual {
		if tr.Predicate == MarkerApprovalPending || tr.Predicate == MarkerApprovalAnswered || tr.Predicate == MarkerApprovalOutstanding {
			selected = append(selected, tr)
		}
	}
	key := func(t message.Triple) string { data, _ := json.Marshal(t); return string(data) }
	a, b := make([]string, 0, len(desired)), make([]string, 0, len(selected))
	for _, tr := range desired {
		a = append(a, key(tr))
	}
	for _, tr := range selected {
		b = append(b, key(tr))
	}
	sort.Strings(a)
	sort.Strings(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
