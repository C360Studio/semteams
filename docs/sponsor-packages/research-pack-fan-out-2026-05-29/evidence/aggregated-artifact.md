# Aggregated research artifact — run 6

The actual artifact the chain produced on smoke #fan-out-validation-6, captured from the synthesize loop's `emit_research_artifact` tool call. Renders the full data flow from the three parallel investigators → aggregated artifact → reviewer verdict → user-facing response.

The original markdown rendering at `/artifacts/research/<slug>.md` inside the backend container was lost on teardown, but the structured payload is preserved in the synthesize loop's trajectory (`/tmp/fan-out-validation-6/trajectory-10fca886-*.json`).

## What the user saw

The coordinator wake-up loop composed this user-facing reply from the reviewer-approved artifact and emitted it as the chain terminal. This is the answer that would be sent on the user-response bus:

> The research team has completed the latency comparison between MQTT and NATS for IoT Edge environments running on ARM architecture. The findings indicate that NATS generally offers significantly lower latency and higher throughput, making it ideal for high-performance edge computing. However, MQTT remains the better choice if your edge devices face intermittent connectivity or require standard lightweight protocol integrations. Let us know if you would like to drill down into the specific benchmark numbers or deployment topologies.

## Reviewer verdict

**Action:** `approved`

**Reason:**

> Research artifact (MQTT vs. NATS latency comparison for IoT Edge on ARM, revision 1) meets all checklist items; all elements are well-formed and consistent with the artifact's purpose.

## Aggregated artifact

**Title:** MQTT vs. NATS Latency Comparison for IoT Edge on ARM

**Revision:** 1

### Actors

| Name | Role |
|---|---|
| MQTT | Messaging protocol for IoT |
| NATS | High-performance messaging system |
| IoT Edge Deployments | Distributed computing environment |
| Constrained ARM Devices | Hardware with limited resources (e.g., Raspberry Pi, microcontrollers) |
| Broker | Server handling message routing for MQTT |
| Client Devices | Endpoints that send/receive messages |
| TLS 1.3 | Transport Layer Security protocol for secure communication |
| PQC (Post-Quantum Cryptography) | Cryptographic methods resistant to quantum attacks |
| Eclipse Mosquitto | Popular open-source MQTT broker |
| HiveMQ | Commercial MQTT broker |
| ActiveMQ Artemis | Apache messaging broker supporting MQTT |
| NATS Server | Core component of the NATS messaging system |
| NATS JetStream | NATS's built-in persistence and streaming layer |
| Edge Servers | More powerful computing devices at the edge |
| Industrial Gateways | Devices connecting industrial equipment to networks |

### Integration points

| From | To | Data | Direction |
|---|---|---|---|
| Client Devices | Broker | IoT Messages | write |
| Broker | Client Devices | IoT Messages | write |
| Client Devices | NATS Server | IoT Messages | write |
| NATS Server | Client Devices | IoT Messages | write |

### Tasks

- Describe the key features, QoS levels, and latency implications of the MQTT protocol, detailing its performance characteristics on constrained ARM devices.
- Describe the key features, messaging patterns (publish/subscribe, request/reply), and latency implications of the NATS protocol, including its performance on constrained ARM devices.
- Compare MQTT and NATS by analyzing their architectural designs, implementation considerations, and protocol-specific factors that influence latency in IoT edge deployments.

### Open gaps

- needs_test_harness: not applicable — informational research on protocol comparison, no concrete external verification surface.
- More comprehensive and specific latency benchmarks across a broader spectrum of constrained ARM devices and various real-world IoT scenarios for MQTT would provide a deeper understanding of its performance.

## Per-investigator findings (3 parallel gatherers)

Each gatherer was scoped to one subtopic from the planner's decomposition. The synthesize loop read each via `read_loop_result` and produced the aggregate above.

### Investigator 1 — MQTT protocol analysis
*Loop ID: `bd8f6d27`*

Subtopic: Characterize MQTT protocol, its latency implications, and performance on constrained ARM devices.

MQTT Protocol Characteristics:
- Lightweight: MQTT is designed to be lightweight with minimal message headers, with control messages potentially as small as two bytes, making it suitable for resource-constrained devices [4].
- Publish-Subscribe Model: It utilizes a publish-subscribe messaging pattern, which is well-suited for many IoT communication scenarios [2].
- QoS Levels: MQTT offers three Quality of Service (QoS) levels to guarantee message delivery:
    - QoS 0 (At Most Once): Messages are sent without acknowledgment, offering the lowest latency but no guarantee of delivery.
    - QoS 1 (At Least Once): Messages are guaranteed to be delivered at least once, with potential for duplicates. This involves a handshake mechanism, introducing some latency.
    - QoS 2 (Exactly Once): Messages are guaranteed to be delivered exactly once. This level involves a four-packet handshake, resulting in the highest latency (e.g., on a 150ms cellular link, this can add a minimum of 600ms before another message can use the same packet ID) [1, 3, 5].

Latency Implications:
- QoS Level: Higher QoS levels (1 and 2) directly increase network overhead and latency due to the additional handshake mechanisms required to ensure delivery guarantees [1]. QoS 2, in particular, can double the round-trip time compared to QoS 1 [3].
- Security Protocols: The use of Transport Layer Security (TLS) for securing MQTT communication can introduce latency, especially in resource-constrained environments. However, MQTT over TLS 1.3 offers improved security and reduced latency compared to TLS 1.2 [1, 5]. Post-Quantum Cryptography (PQC)-based solutions without TLS are being explored as a means to achieve lower latency [1].
- Network Overhead: Generally, higher QoS levels and persistent sessions can lead to increased network traffic, higher latency, and greater memory usage on both the client device and the broker [4].

Performance on Constrained ARM Devices:
- MQTT's lightweight design makes it inherently suitable for resource-constrained devices, including those based on ARM architecture [2, 4].
- Studies indicate that PQC schemes implemented on Cortex-M/A series ARM devices demonstrate performance comparable to Elliptic-Curve Cryptography (ECC)-based schemes [1].
- While specific, wide-ranging benchmarks across various ARM devices are not extensively detailed in the search results, there are indications of good performance. For example, the Azure IoT Operations MQTT Broker reports sub-10 ms latencies for time-sensitive operations even with constrained payload sizes [1].
- Research is actively focused on optimizing MQTT for highly constrained environments, such as 8-bit AVR sensor nodes, which further underscores its potential for efficient operation on constrained hardware [1].

Open Gaps: While the lightweight nature and some performance indicators are present, more comprehensive and specific latency benchmarks across a broader spectrum of constrained ARM devices and various real-world IoT scenarios would provide a deeper understanding of MQTT's performance in such environments.

---

### Investigator 2 — NATS protocol analysis
*Loop ID: `1fa96c8a`*

Subtopic: Characterize NATS protocol, its latency implications, and performance on constrained ARM devices.

NATS Protocol Characteristics:
NATS is a high-performance, open-source messaging system designed for modern, cloud-native, and edge-first applications. It supports multiple messaging patterns, including publish/subscribe (Pub/Sub), request/reply, and streaming with optional persistence via JetStream. The NATS server is implemented in Go, utilizes a lightweight, single-threaded I/O loop, and employs zero-copy message dispatch with subject-based routing to achieve high throughput and low latency.

Latency Implications:
NATS is explicitly optimized for low latency and scalability, consistently claiming sub-millisecond latencies. Its design emphasizes minimal protocol overhead, which directly contributes to its low latency characteristics. Benchmarks have shown average request-reply latencies around 50.87 microseconds. The NATS Command Line Interface (CLI) includes tools for comprehensive performance testing, benchmarking, and latency measurement in NATS deployments.

Performance on Constrained ARM Devices:
NATS is well-suited for constrained environments, including ARM-based IoT edge devices. The NATS server is distributed as a small static binary that can be deployed on resource-constrained hardware such as a Raspberry Pi. It supports an Adaptive Edge architecture, enabling flexible deployments across cloud, on-premise, edge, and IoT environments. A benchmark comparing NATS with MQTT on Raspberry Pi 4 devices (2GB RAM) with 100 leaf nodes demonstrated NATS achieving 12ms sync latency, significantly outperforming MQTT bridges (Eclipse Mosquitto) which exhibited 96ms sync latency and required four times more memory in the same scenario. This indicates NATS's strong performance and efficiency on constrained ARM devices, making it a viable choice for IoT edge deployments where both resource utilization and latency are critical factors.

---

### Investigator 3 — Comparison factors
*Loop ID: `c0d0cf9e`*

Subtopic: Compare MQTT and NATS latency factors in IoT edge deployments, including architectural and implementation considerations.

MQTT Latency Factors:
- QoS levels (0, 1, 2) directly impact latency, with QoS 0 offering the lowest latency (fire-and-forget) and QoS 2 (exactly once) having the highest overhead due to a four-step handshake.
- Broker-based architecture, generally suitable for distributed devices requiring minimal coordination.
- Implementations like Mosquitto (C-based) are optimized for resource-constrained hardware but may saturate earlier under high client loads due to its single-threaded nature.
- Java-based MQTT brokers (e.g., HiveMQ, ActiveMQ Artemis) consume significantly more memory, which is a critical consideration for memory-constrained ARM edge devices.

NATS Latency Factors:
- Designed for ultra-low latency and high throughput, making it suitable for real-time analytics and high-frequency industrial workloads.
- Offers sub-millisecond p99 latency in production environments.
- Employs a distributed mesh architecture of servers, providing no single point of failure and enabling efficient scaling.
- NATS JetStream is better suited for more powerful hardware like edge servers or industrial gateways with sufficient CPU and SSDs.
- Implemented in Go, prioritizing simplicity and performance with a 'fire-and-forget' architecture that minimizes overhead.
- Benefits significantly from multi-core processors, scaling efficiently under high connection loads.

Common Architectural and Implementation Considerations for IoT Edge Deployments:
- Protocol design: The inherent design choices of each protocol (e.g., MQTT's QoS vs. NATS's 'fire-and-forget') directly influence latency and reliability tradeoffs.
- Broker architecture: Single-threaded vs. multi-threaded designs and centralized vs. distributed models impact scalability and resource utilization.
- Implementation language/runtime: Native languages (C, Go, Rust) generally offer lower memory footprints and latency compared to managed runtimes (Java, Erlang), which is crucial for constrained ARM devices.
- Resource utilization: CPU and memory consumption are critical. Native implementations tend to be more resource-efficient.
- Payload size: Larger message payloads inherently increase transmission latency for both protocols.
- Client load: The number of concurrent clients and message rates heavily influence latency and broker performance, especially on resource-limited edge devices.
- Network conditions: Instability and unreliability in edge network environments can significantly affect latency and the ability to maintain QoS guarantees for protocols like MQTT.

---

## Trace and cost

- **Chain shape:** 8 loops (coordinator dispatch → plan → 3 gathers in parallel → synthesize → reviewer → coordinator wake-up)
- **Wallclock:** ~2 minutes 8 seconds
- **Models:** Gemini 3.1 Pro for coordinator (1 call); Gemini 2.5 Flash for plan / 3 gathers / synthesize / reviewer (12 calls)
- **Spend:** ~$0.30
- **Parallelism saving:** ~30s span across the three investigators vs ~90s sequential

Full trajectories preserved at `/tmp/fan-out-validation-6/trajectory-*.json`.
