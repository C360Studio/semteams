// Package harness implements the SemTeams harness catalog — the
// operator-curated registry of available test harnesses for the
// dev-via-spec arc (R3.7, ADR-033).
//
// Harnesses are platform-shape DevOps assets, not chain output:
// operators add a compose profile + a `harnesses.json` entry; the
// chain consumes the catalog (researcher consults it; architect
// scopes smoke contracts to a named harness; builder runs
// `mvn verify -P<profile>` against the named sidecar). The chain
// does NOT synthesise harnesses in this slice; that is R3.7.4
// (`harness-via-spec`).
//
// # Why product-local
//
// Framework-alignment review (ADR-033 §addendum): semstreams beta.39
// has no harness/test-target/catalog primitive. The pattern mirrors
// `flowtemplate.Manager` — KV-backed Pattern-B with file-loader on
// boot — but the harness concept is SemTeams-specific (no semspec /
// semdragon use case yet). Migration target: extract upstream when
// a 2nd product needs it.
//
// # Wire shape
//
//	{
//	  "harnesses": [
//	    {
//	      "name": "meshtasticd-3.x",
//	      "compose_profile": "harness-meshtasticd",
//	      "image": "meshtastic/meshtasticd:3.5.0",
//	      "exposes": {
//	        "tcp": [{"port": 4403, "protocol": "meshtastic-protobuf"}]
//	      },
//	      "smoke_contract_schema": "meshtastic.smoke_contract.v1",
//	      "real_dependencies": [
//	        {"groupId": "com.geeksville.mesh",
//	         "artifactId": "meshtastic-protobufs",
//	         "version_range": "[2.x,3.x)"}
//	      ],
//	      "domain_description": "Real Meshtastic protocol over TCP."
//	    }
//	  ]
//	}
//
// The catalog file is empty in this slice (R3.7.1). The first entry
// (`meshtasticd-3.x`) lands with R3.7.2 when smoke-contract execution
// arrives.
package harness
