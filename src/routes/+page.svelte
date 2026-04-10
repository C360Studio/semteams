<script lang="ts">
	import DataView from '$lib/components/DataView.svelte';
	import { onMount } from 'svelte';
	import type { Flow } from '$lib/types/flow';

	// Auto-discover active flow from the backend config.
	// If the backend has flows (from a loaded config), we pick the running one
	// (or the first one) and pass its ID to DataView for context.
	// If no backend or no flows, DataView still works — just no flow context.
	let activeFlow = $state<Flow | null>(null);

	onMount(async () => {
		try {
			const response = await fetch('/flowbuilder/flows');
			if (response.ok) {
				const data = await response.json();
				const flows: Flow[] = data.flows || [];
				if (flows.length > 0) {
					// Prefer a running flow, otherwise take the first
					activeFlow = flows.find(f => f.runtime_state === 'running')
						?? flows[0];
				}
			}
		} catch {
			// Backend not available — DataView works standalone via GraphQL
		}
	});
</script>

<svelte:head>
	<title>SemStreams</title>
</svelte:head>

<div class="app-layout">
	<header class="app-header">
		<div class="header-content">
			<h1 class="app-title">SemStreams</h1>
			<nav class="header-nav">
				{#if activeFlow}
					<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
					<a href="/flows/{activeFlow.id}" class="nav-link">Flow: {activeFlow.name}</a>
				{/if}
				<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
				<a href="/flows" class="nav-link">Flows</a>
				<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
				<a href="/agents" class="nav-link">Agents</a>
			</nav>
		</div>
	</header>

	<div class="app-content">
		<DataView flowId={activeFlow?.id} />
	</div>
</div>

<style>
	.app-layout {
		display: flex;
		flex-direction: column;
		height: 100vh;
		overflow: hidden;
	}

	.app-header {
		padding: 0.5rem 1rem;
		border-bottom: 1px solid var(--ui-border-subtle);
		background: var(--ui-surface-primary);
		flex-shrink: 0;
	}

	.header-content {
		display: flex;
		align-items: center;
		gap: 1.5rem;
	}

	.app-title {
		margin: 0;
		font-size: 1.125rem;
		color: var(--ui-text-primary);
		font-weight: 600;
	}

	.header-nav {
		display: flex;
		gap: 1rem;
		align-items: center;
	}

	.nav-link {
		color: var(--ui-text-secondary);
		text-decoration: none;
		font-size: 0.875rem;
		font-weight: 500;
		padding: 0.25rem 0.5rem;
		border-radius: 4px;
		transition: all 0.15s;
	}

	.nav-link:hover {
		color: var(--ui-text-primary);
		background: var(--ui-surface-secondary);
	}

	.app-content {
		flex: 1;
		overflow: hidden;
	}
</style>
