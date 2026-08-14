<script lang="ts">
	import { agentApi } from '$lib/services/agentApi';
	import type { LoopTrajectory, TrajectoryFact } from '$lib/types/agent';

	interface Props {
		loopId: string;
	}

	let { loopId }: Props = $props();

	let trajectory = $state<LoopTrajectory | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);

	// Request-race guard (same pattern as TaskStory/RunEvidencePanel): a
	// stale in-flight response for a previous loopId must not overwrite
	// the currently requested loop's data under out-of-order resolution.
	let requestSeq = 0;

	$effect(() => {
		loadTrajectory(loopId);
	});

	async function loadTrajectory(id: string) {
		const requestId = ++requestSeq;
		loading = true;
		error = null;
		try {
			const next = await agentApi.getTrajectory(id);
			if (requestId !== requestSeq || id !== loopId) return;
			trajectory = next;
		} catch (e) {
			if (requestId !== requestSeq || id !== loopId) return;
			error = e instanceof Error ? e.message : 'Failed to load trajectory';
		} finally {
			if (requestId === requestSeq) {
				loading = false;
			}
		}
	}

	// The loop's role isn't a top-level Trajectory field on the beta.160
	// fact-log wire shape — it rides along on every completed fact's
	// capability_preview. loop.started is recorded before any iteration,
	// so it's the most reliable single source.
	function deriveRole(facts: TrajectoryFact[]): string {
		const started = facts.find((f) => f.kind === 'loop.started');
		if (started?.capability_preview) return started.capability_preview;
		const anyCompleted = facts.find((f) => f.capability_preview);
		return anyCompleted?.capability_preview ?? '';
	}

	function terminalFact(facts: TrajectoryFact[]): TrajectoryFact | undefined {
		return facts.find((f) => f.kind === 'loop.terminal');
	}

	const role = $derived(trajectory ? deriveRole(trajectory.facts) : '');
	const terminal = $derived(trajectory ? terminalFact(trajectory.facts) : undefined);
	const outcome = $derived(terminal?.status ?? '');
	const durationMs = $derived(terminal?.elapsed_ms ?? 0);
	const iterations = $derived(terminal?.causal_iteration ?? 0);
	// Tool call arguments and results are not part of the trajectory fact
	// log (privacy-bounded metadata only) — only the tool name preview,
	// timing, and status survive. See ui/.claude regression notes on the
	// beta.160 trajectory migration for what this component used to show.
	const toolFacts = $derived(
		trajectory ? trajectory.facts.filter((f) => f.kind === 'tool.completed') : [],
	);
</script>

<div data-testid="trajectory-viewer" class="trajectory-viewer">
	{#if loading}
		<div class="loading" data-testid="trajectory-loading">Loading trajectory...</div>
	{:else if error}
		<div class="error" data-testid="trajectory-error">{error}</div>
	{:else if trajectory}
		<div class="trajectory-header">
			<h3>{role || 'agent'}</h3>
			<div class="trajectory-meta">
				<span class="outcome" data-testid="trajectory-outcome">{outcome || 'in progress'}</span>
				<span class="duration">{durationMs}ms</span>
				<span class="iterations">{iterations} iterations</span>
			</div>
		</div>

		{#if trajectory.observed_totals.tokens_in > 0 || trajectory.observed_totals.tokens_out > 0}
			<div class="token-usage" data-testid="token-usage">
				<span>Input: {trajectory.observed_totals.tokens_in.toLocaleString()} tokens</span>
				<span>Output: {trajectory.observed_totals.tokens_out.toLocaleString()} tokens</span>
			</div>
		{/if}

		{#if toolFacts.length > 0}
			<div class="tool-calls" data-testid="tool-calls">
				<h4>Tool Calls ({toolFacts.length})</h4>
				{#each toolFacts as fact, i (fact.attempt_id ?? i)}
					<div class="tool-call-entry" data-testid="tool-call-entry">
						<div class="tool-call-header">
							<span class="tool-name">{fact.tool_preview ?? 'tool'}</span>
							{#if fact.elapsed_ms != null}
								<span class="tool-duration">{fact.elapsed_ms}ms</span>
							{/if}
						</div>
						{#if fact.status && fact.status !== 'completed'}
							<div class="tool-error" data-testid="tool-call-error">
								{fact.error_category ? `${fact.status} (${fact.error_category})` : fact.status}
							</div>
						{/if}
					</div>
				{/each}
			</div>
		{/if}
	{/if}
</div>

<style>
	.trajectory-viewer {
		padding: 1rem;
		background: var(--ui-surface-secondary);
		border-radius: 8px;
		border: 1px solid var(--ui-border-subtle);
	}

	.loading {
		color: var(--ui-text-secondary);
		font-size: 0.875rem;
		padding: 1rem 0;
	}

	.error {
		color: var(--status-error, #ef4444);
		font-size: 0.875rem;
		padding: 0.5rem 0.75rem;
		background: var(--status-error-container, rgba(239, 68, 68, 0.15));
		border-radius: 4px;
	}

	.trajectory-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		margin-bottom: 1rem;
	}

	.trajectory-header h3 {
		margin: 0;
		font-size: 1rem;
		font-weight: 600;
		color: var(--ui-text-primary);
		text-transform: capitalize;
	}

	.trajectory-meta {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		font-size: 0.8125rem;
		color: var(--ui-text-secondary);
	}

	.outcome {
		display: inline-block;
		padding: 0.125rem 0.5rem;
		border-radius: 10px;
		font-size: 0.75rem;
		font-weight: 500;
		text-transform: capitalize;
		background: var(--status-success-container, rgba(34, 197, 94, 0.15));
		color: var(--status-success, #22c55e);
	}

	.token-usage {
		display: flex;
		gap: 1.5rem;
		padding: 0.5rem 0.75rem;
		margin-bottom: 1rem;
		background: var(--ui-surface-tertiary);
		border-radius: 4px;
		font-size: 0.8125rem;
		color: var(--ui-text-secondary);
	}

	.tool-calls {
		margin-top: 0.5rem;
	}

	.tool-calls h4 {
		margin: 0 0 0.75rem 0;
		font-size: 0.875rem;
		font-weight: 600;
		color: var(--ui-text-primary);
	}

	.tool-call-entry {
		padding: 0.5rem 0.75rem;
		margin-bottom: 0.5rem;
		background: var(--ui-surface-tertiary);
		border-radius: 4px;
		border-left: 3px solid var(--ui-interactive-primary, #78a9ff);
	}

	.tool-call-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		margin-bottom: 0.25rem;
	}

	.tool-name {
		font-family: monospace;
		font-size: 0.8125rem;
		font-weight: 600;
		color: var(--ui-interactive-primary, #78a9ff);
	}

	.tool-duration {
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
	}

	.tool-error {
		margin-top: 0.25rem;
		padding: 0.25rem 0.5rem;
		font-size: 0.75rem;
		color: var(--status-error, #ef4444);
		background: var(--status-error-container, rgba(239, 68, 68, 0.15));
		border-radius: 4px;
		text-transform: capitalize;
	}
</style>
