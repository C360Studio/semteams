<script lang="ts">
	import DataView from '$lib/components/DataView.svelte';
	import { onMount } from 'svelte';
	import type { Flow } from '$lib/types/flow';

	// Auto-discover active flow from the backend config.
	let activeFlow = $state<Flow | null>(null);

	onMount(async () => {
		try {
			const response = await fetch('/flowbuilder/flows');
			if (response.ok) {
				const data = await response.json();
				const flows: Flow[] = data.flows || [];
				if (flows.length > 0) {
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
	<title>Graph - SemTeams</title>
</svelte:head>

<div class="graph-page">
	<DataView flowId={activeFlow?.id} />
</div>

<style>
	.graph-page {
		width: 100%;
		height: 100%;
		overflow: hidden;
	}
</style>
