package sandboxmanager

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// AdmissionPolicy is the operator-level policy that gates a
// SandboxRequirements against a matched Profile. It encodes "what
// the operator pre-approved this deployment to allow," distinct
// from the profile's per-profile allowlist (what the *profile*
// supports). Both have to admit a capability for it to pass.
//
// The policy is operator-config-loaded at boot; per-request changes
// require a re-load (per ADR-043: "Each secret must be pre-
// provisioned in operator config — boot-time policy, no per-
// request"). This is intentional friction — admission is a security
// surface where silent loosening is the failure mode.
type AdmissionPolicy struct {
	// AllowPublicNetwork unlocks NetworkPublic requirements globally.
	// Default false: profiles that advertise AllowedNetwork=public
	// still pend on admission until the operator flips this OR the
	// chain clears admission-reviewer approval. Combines OR-style
	// with reviewer approval — the reviewer can sign off case-by-case
	// even when the operator hasn't flipped the global flag.
	AllowPublicNetwork bool

	// AllowPrivilegedContainers is the "anything goes" escape hatch
	// for full --privileged mode. Default false. Forbidden unless
	// the operator explicitly opts in; reviewer approval is not
	// sufficient (ADR-043 §Decision: "Forbidden by default" for
	// privileged; "Operator-level config flag + chain approval").
	AllowPrivilegedContainers bool

	// AvailableSecrets is the set of secret identifiers the operator
	// has pre-provisioned (env vars, sealed files, mounted at
	// devcontainer up). Requirements declaring a secret not in this
	// set fail terminally — Coordinator can't recover by asking
	// the user to enter a credential.
	AvailableSecrets []string
}

// AdmissionOutcome enumerates the three terminal admission classes:
//
//   - Admitted: every requirement clears profile + policy gates.
//     Manager proceeds to devcontainer up.
//   - AdmissionPending: at least one requirement needs human/policy
//     approval (e.g. docker-socket on a profile that allows it but
//     the chain has not been pre-approved). Manager returns; rule
//     pack spawns admission-reviewer.
//   - Denied: at least one requirement is structurally inadmissible
//     (privileged when AllowPrivilegedContainers is false; secret
//     not in AvailableSecrets; mount class the profile rejects).
//     Terminal — Coordinator surfaces failure to user.
type AdmissionOutcome string

// AdmissionOutcome enum values. Wire-form is the lowercased string;
// stamped on attestation.outcome triples (sandbox.attestation.outcome).
const (
	AdmissionAdmitted AdmissionOutcome = "admitted"
	AdmissionPending  AdmissionOutcome = "admission_pending"
	AdmissionDenied   AdmissionOutcome = "denied"
)

// AdmissionResult is the structured outcome of AdmissionCheck.
// Reasons is a human-readable list, ordered by the gate that
// surfaced them; PendingReasons are subset reasons that an
// admission-reviewer signoff would clear; DeniedReasons are the
// terminal subset.
//
// When Outcome=Admitted, both reason slices are empty. When
// Outcome=AdmissionPending, PendingReasons is non-empty and
// DeniedReasons is empty. When Outcome=Denied, DeniedReasons is
// non-empty (PendingReasons may also be non-empty — they go down
// with the ship).
type AdmissionResult struct {
	Outcome        AdmissionOutcome
	PendingReasons []string
	DeniedReasons  []string
}

// Reasons returns the union of Pending and Denied reasons in stable
// order. Used by Attest to stamp the human-readable list on the
// attestation triples.
func (r AdmissionResult) Reasons() []string {
	out := make([]string, 0, len(r.PendingReasons)+len(r.DeniedReasons))
	out = append(out, r.DeniedReasons...)
	out = append(out, r.PendingReasons...)
	return out
}

// AdmissionCheck applies the gates from ADR-043 §Decision against a
// matched profile and operator policy. Returns an AdmissionResult.
// Error is returned only for malformed input (typically a Validate
// failure callers should have caught first); admission denials are
// represented in AdmissionResult.Outcome, not as errors.
//
// Gates applied (one row per ADR §Decision table):
//
//  1. Privilege classes — profile allowlist + policy gate. Privileged
//     requires AllowPrivilegedContainers; docker-socket requires
//     profile-allowlisted; both pending if profile allows but
//     operator hasn't, denied if profile rejects.
//
//  2. Network class — profile allowlist + (for public) policy gate.
//     Public on a profile that allows it pends without
//     AllowPublicNetwork; restricted/none on a profile that
//     allows it admits.
//
//  3. Mount class — profile-policy currently uniform (workspace-write
//     + workspace-read both admit on every profile). Future:
//     workspace-read may need a per-profile flag. Today: pass-through.
//
//  4. Secrets — every requested secret must be in AvailableSecrets.
//     Missing secret is terminal (no reviewer path can provision
//     one out of thin air).
//
//  5. Verification probes — validate only (must have been Validate'd
//     by caller; if any probe Name slipped past Validate, treat as
//     denied to fail-fast rather than silently materialize).
//
// Order matters only insofar as DeniedReasons + PendingReasons are
// surfaced in audit. Outcome is the worst-case across all rows
// (Denied > Pending > Admitted).
func AdmissionCheck(req SandboxRequirements, profile Profile, policy AdmissionPolicy) AdmissionResult {
	var pending, denied []string

	for _, p := range req.Privileges {
		switch p {
		case PrivilegePrivileged:
			if !policy.AllowPrivilegedContainers {
				denied = append(denied, "privilege:privileged requires AllowPrivilegedContainers operator policy")
				continue
			}
			if !ProfileAllowsPrivilege(profile, PrivilegePrivileged) {
				denied = append(denied, fmt.Sprintf("privilege:privileged not allowed on profile %s", profile.ID()))
				continue
			}
			pending = append(pending, "privilege:privileged requires chain admission-reviewer approval")
		case PrivilegeDockerSocket:
			if !ProfileAllowsPrivilege(profile, PrivilegeDockerSocket) {
				denied = append(denied, fmt.Sprintf("privilege:docker-socket not allowed on profile %s", profile.ID()))
				continue
			}
			pending = append(pending, "privilege:docker-socket requires chain admission-reviewer approval")
		default:
			denied = append(denied, fmt.Sprintf("privilege:%s unknown to admission (Validate should have caught)", p))
		}
	}

	switch req.Network {
	case "", NetworkRestricted, NetworkNone:
		net := req.Network
		if net == "" {
			net = NetworkRestricted
		}
		if !ProfileAllowsNetwork(profile, net) {
			denied = append(denied, fmt.Sprintf("network:%s not allowed on profile %s", net, profile.ID()))
		}
	case NetworkPublic:
		if !ProfileAllowsNetwork(profile, NetworkPublic) {
			denied = append(denied, fmt.Sprintf("network:public not allowed on profile %s", profile.ID()))
			break
		}
		if !policy.AllowPublicNetwork {
			pending = append(pending, "network:public requires AllowPublicNetwork operator policy OR admission-reviewer approval")
		}
	default:
		denied = append(denied, fmt.Sprintf("network:%s unknown to admission (Validate should have caught)", req.Network))
	}

	// Secret-availability lookup is CASE-SENSITIVE against the
	// operator config. Validate already pins requirement-side secret
	// names to [A-Z_][A-Z0-9_]*, so a lowercase policy entry
	// ("openai_api_key") deliberately does NOT match a canonical
	// requirement ("OPENAI_API_KEY") — operator config errors should
	// fail-shut, not silently widen admission. Per PR 4.1 go-reviewer
	// finding C4.
	availSet := make(map[string]struct{}, len(policy.AvailableSecrets))
	for _, s := range policy.AvailableSecrets {
		availSet[strings.TrimSpace(s)] = struct{}{}
	}
	for _, s := range req.Secrets {
		if _, ok := availSet[strings.TrimSpace(s)]; !ok {
			denied = append(denied, fmt.Sprintf("secret:%s not pre-provisioned by operator", s))
		}
	}

	for _, m := range req.Mounts {
		switch m {
		case MountWorkspaceWrite, MountWorkspaceRead:
		default:
			denied = append(denied, fmt.Sprintf("mount:%s unknown to admission (Validate should have caught)", m))
		}
	}

	for _, p := range req.Verification {
		if strings.TrimSpace(p.Name) == "" || strings.TrimSpace(p.Command) == "" {
			denied = append(denied, fmt.Sprintf("verification probe %q malformed (Validate should have caught)", p.Name))
		}
	}

	sort.Strings(pending)
	sort.Strings(denied)
	pending = dedupSorted(pending)
	denied = dedupSorted(denied)

	switch {
	case len(denied) > 0:
		return AdmissionResult{Outcome: AdmissionDenied, DeniedReasons: denied, PendingReasons: pending}
	case len(pending) > 0:
		return AdmissionResult{Outcome: AdmissionPending, PendingReasons: pending}
	default:
		return AdmissionResult{Outcome: AdmissionAdmitted}
	}
}

func dedupSorted(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := in[:0]
	for i, s := range in {
		if i > 0 && s == in[i-1] {
			continue
		}
		out = append(out, s)
	}
	// Reslice to a fresh backing so callers' caller can't accidentally
	// resize over our cap.
	return slices.Clone(out)
}
