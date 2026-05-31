package sandboxmanager

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParseRequirements decodes an LLM-supplied tool-args map into a
// SandboxRequirements. Shared between request_sandbox and
// query_sandbox_attestation — both tools accept the same shape and
// must accept identical inputs for the (profile, requirements_hash)
// signature to round-trip across the two tools without drift. Lives
// in this package (not the tools) so the type and its parser stay
// co-located.
//
// Unrecognized fields are ignored (forward-compat). Validate is NOT
// run here — callers (Manager.Request) own that.
func ParseRequirements(raw map[string]any) (SandboxRequirements, error) {
	out := SandboxRequirements{}
	if v, ok := raw["languages"].([]any); ok {
		s, err := parseStringSlice("languages", v)
		if err != nil {
			return out, err
		}
		out.Languages = s
	}
	if v, ok := raw["tools"].([]any); ok {
		s, err := parseStringSlice("tools", v)
		if err != nil {
			return out, err
		}
		out.Tools = s
	}
	if v, ok := raw["services"].([]any); ok {
		s, err := parseStringSlice("services", v)
		if err != nil {
			return out, err
		}
		out.Services = s
	}
	if v, ok := raw["network"].(string); ok {
		out.Network = NetworkPolicy(v)
	}
	if v, ok := raw["secrets"].([]any); ok {
		s, err := parseStringSlice("secrets", v)
		if err != nil {
			return out, err
		}
		out.Secrets = s
	}
	if v, ok := raw["mounts"].([]any); ok {
		mounts, err := parseMountSlice(v)
		if err != nil {
			return out, err
		}
		out.Mounts = mounts
	}
	if v, ok := raw["privileges"].([]any); ok {
		privs, err := parsePrivilegeSlice(v)
		if err != nil {
			return out, err
		}
		out.Privileges = privs
	}
	if v, ok := raw["verification"].([]any); ok {
		probes, err := parseProbeSlice(v)
		if err != nil {
			return out, err
		}
		out.Verification = probes
	}
	return out, nil
}

func parseStringSlice(field string, in []any) ([]string, error) {
	out := make([]string, 0, len(in))
	for i, v := range in {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("%s[%d]: expected string, got %T", field, i, v)
		}
		out = append(out, strings.TrimSpace(s))
	}
	return out, nil
}

func parseMountSlice(in []any) ([]MountClass, error) {
	s, err := parseStringSlice("mounts", in)
	if err != nil {
		return nil, err
	}
	mounts := make([]MountClass, len(s))
	for i, m := range s {
		mounts[i] = MountClass(m)
	}
	return mounts, nil
}

func parsePrivilegeSlice(in []any) ([]Privilege, error) {
	s, err := parseStringSlice("privileges", in)
	if err != nil {
		return nil, err
	}
	privs := make([]Privilege, len(s))
	for i, p := range s {
		privs[i] = Privilege(p)
	}
	return privs, nil
}

func parseProbeSlice(in []any) ([]VerifyProbe, error) {
	probes := make([]VerifyProbe, 0, len(in))
	for i, p := range in {
		pm, ok := p.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("verification[%d]: expected object, got %T", i, p)
		}
		probe := VerifyProbe{}
		if s, ok := pm["name"].(string); ok {
			probe.Name = s
		}
		if s, ok := pm["command"].(string); ok {
			probe.Command = s
		}
		if iv, ok := pm["expect_exit"]; ok {
			n, err := parseExpectExit(i, iv)
			if err != nil {
				return nil, err
			}
			probe.ExpectExit = n
		}
		probes = append(probes, probe)
	}
	return probes, nil
}

func parseExpectExit(idx int, v any) (int, error) {
	switch x := v.(type) {
	case float64:
		return int(x), nil
	case int:
		return x, nil
	case json.Number:
		n, _ := x.Int64()
		return int(n), nil
	default:
		return 0, fmt.Errorf("verification[%d].expect_exit: expected integer, got %T", idx, v)
	}
}
