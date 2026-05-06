# Test-harness manifest contract

> Added ADR-036 Phase 1. Read this after `15-commitment-driven-authoring.md`
> and before starting any test code for a `process-local-testcontainer` check.

## The manifest file

When the architect named one or more `test_harness:` references in checks,
your workspace contains `.test-harness/manifest.json`. Read it:

```bash
bash cat .test-harness/manifest.json
```

The file is a JSON object keyed by catalog ID. Each value is the resolved
manifest for that harness:

```json
{
  "meshtasticd-3.x": {
    "id": "meshtasticd-3.x",
    "image": "meshtastic/meshtasticd:3.5.0",
    "ports": {"meshtastic-protobuf": 4403}
  }
}
```

If `.test-harness/manifest.json` is absent, the architect emitted no
`test_harness:` references — your checks are unit-only or static-analysis.
Skip this fragment entirely.

## Testcontainers Java idiom

For each `process-local-testcontainer` check, read `image` and `ports` from
the manifest. Instantiate a `GenericContainer<?>` with those exact values:

```java
@Testcontainers
class MeshtasticdIntegrationIT {

    @Container
    static GenericContainer<?> meshtasticd =
        new GenericContainer<>("meshtastic/meshtasticd:3.5.0")  // from manifest.image
            .withExposedPorts(4403)                             // from manifest.ports value
            .waitingFor(Wait.forListeningPort());

    @Test
    void positionAppPacketProducesObservation() {
        int port = meshtasticd.getMappedPort(4403);
        // connect to localhost:port and drive the harness
    }
}
```

Read the image string and port number from the manifest. Do NOT substitute
a different image tag or guess a port — the operator curated the catalog
and the architect transcribed it; the rendered values are the contract.

## Testcontainers lifecycle

Testcontainers starts the container when the JVM starts the test class and
stops it on JVM exit. No `docker run`, no `docker stop`, no pre-test hook.
The `@Container` annotation (or its Go/Python equivalent) handles the full
lifecycle. Your test process and the harness container share the test JVM's
lifecycle — no coordination with SemTeams tooling required.

The sandbox runs in DooD mode (Docker-out-of-Docker). Testcontainers' host
Docker socket access works as normal: `docker run` inside the sandbox spawns
sibling containers on the host daemon. No special configuration needed.

## When the manifest is present but image is missing

If the C's `**Test harness**` line is present in SPEC.md but `**Image**`
is absent, the architect's catalog lookup failed at emit time. Do NOT
fabricate an image tag from training data. Terminate with:

```
builder_decide(action="needs_clarification",
               reason="test_harness <name> resolved in SPEC.md but Image
                       is missing — catalog entry may have been removed or
                       renamed since architect emit. Operator must update
                       the catalog or fix the check's test_harness reference.")
```

## No SemTeams harness CLI

There is no `test_harness up` or `test_harness down` command. The test
framework owns the lifecycle. If you find yourself writing shell commands
to start a harness container manually, stop — that's the Testcontainers
annotation's job.
