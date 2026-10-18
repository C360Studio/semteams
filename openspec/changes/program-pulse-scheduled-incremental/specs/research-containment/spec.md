# research-containment Specification Delta

## MODIFIED Requirements

### Requirement: Scheduled cron inflight, cooldown, and Pattern-B kill-switch containment

Scheduled cron rules — including the wired `configs/rules/research/10-scheduled-program-pulse.json` (`id: "scheduled_program_pulse"`, `schedule: "0 9 * * *"`, `cooldown: "20h"`, shipped with `enabled: false` by default in `configs/flow-bootstrap.json` and `configs/e2e-flow-bootstrap.json`) — SHALL prevent stacked dispatches via the scheduler's inflight and cooldown gates, and operators SHALL be able to enable or disable the live cron rule via the Pattern-B `rule.ConfigManager` without restarting the process.

#### Scenario: Inflight and cooldown gates prevent stacked cron dispatches

- **GIVEN** an enabled cron rule whose action execution exceeds a tick interval or falls within its configured `cooldown`
- **WHEN** subsequent scheduler ticks arrive while the prior fire is in flight or within cooldown
- **THEN** the scheduler skips the overlapping ticks without dispatching duplicate actions

#### Scenario: Pattern-B rule disable stops future cron firings without restart

- **GIVEN** a running `rule.Processor` and `rule.ConfigManager` with an enabled cron rule that has fired
- **WHEN** the rule is updated to `enabled: false` through `rule.ConfigManager.SaveRule` without restarting the process
- **THEN** hot-reload reconciliation deregisters the scheduled rule and no further ticks fire across a bounded post-disable observation window
- **AND** disabling the schedule does not retroactively cancel work already dispatched by the prior fire

#### Scenario: Shipped scheduled Program Pulse rule is wired disabled by default and routes via coordinator into research

- **GIVEN** `configs/rules/research/10-scheduled-program-pulse.json` wired in `configs/flow-bootstrap.json` and `configs/e2e-flow-bootstrap.json`
- **WHEN** the rule definition is loaded at boot or enabled at runtime via `rule.ConfigManager.SaveRule`
- **THEN** its default on-disk state SHALL be `enabled: false` with `type: "cron"`, positive `cooldown` (`20h`), and `loop_max_iterations: "8"` on its `publish_agent` coordinator action
- **AND** when enabled, its `publish_agent` action SHALL dispatch an `@pulse` prompt to `agent.task.coordinator` (`run_scope: "new"`) with `action_allowlist: ["research", "respond_direct"]`
