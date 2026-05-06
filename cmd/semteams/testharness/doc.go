// Package testharness implements the SemTeams test-harness catalog — the
// operator-curated registry of available test harnesses for the
// dev-via-spec arc (R3.7, ADR-033, ADR-036).
//
// Test harnesses are platform-shape DevOps assets, not chain output:
// operators add a `harnesses.json` entry (and, for external-sidecar
// flows, a matching compose profile); the chain consumes the catalog
// (researcher consults it; architect scopes checks to a named
// test_harness; builder runs the project-native test command against
// the named sidecar — Testcontainers-managed in-process for
// process-local-testcontainer flows, operator-managed for external-
// sidecar flows). The chain does NOT synthesise test harnesses in this
// slice; that is R3.7.4 (`harness-via-spec`).
//
// # Why product-local
//
// Framework-alignment review (ADR-033 §addendum): semstreams beta.39
// has no test-harness/catalog primitive. The pattern mirrors
// `flowtemplate.Manager` — KV-backed Pattern-B with file-loader on
// boot — but the test-harness concept is SemTeams-specific (no semspec /
// semdragon use case yet). Migration target: extract upstream when
// a 2nd product needs it.
//
// # Wire shape
//
//	{
//	  "harnesses": [
//	    {
//	      "name": "meshtasticd-3.x",
//	      "image": "meshtastic/meshtasticd:3.5.0",
//	      "exposes": {
//	        "tcp": [{"port": 4403, "protocol": "meshtastic-protobuf"}]
//	      },
//	      "smoke_contract_schema": "meshtasticd.smoke_contract.v1",
//	      "real_dependencies": [
//	        {"groupId": "com.geeksville.mesh",
//	         "artifactId": "meshtastic-protobufs",
//	         "version_range": "[3.0,4.0)"}
//	      ],
//	      "domain_description": "Real Meshtastic protocol over TCP."
//	    }
//	  ]
//	}
//
// `compose_profile` is OPTIONAL as of ADR-034 (§"What R3.7.2 work is
// preserved"); chains using process-local-testcontainer runtime
// manage the sidecar lifecycle in-process via Testcontainers and
// don't reference a profile. External-sidecar runtime (operator
// pre-provisioned) still uses it.
//
// The first entry (`meshtasticd-3.x`) ships with R3.7.2.e′; smoke-
// contract execution arrives with R3.7.2.h′ + smoke #7.
package testharness
