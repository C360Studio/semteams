<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import FlowList from '$lib/components/FlowList.svelte';
	import { flowApi } from '$lib/services/flowApi';
	import { checkBackendHealth, getUserFriendlyErrorMessage } from '$lib/services/healthCheck';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let backendHealthy = $state<boolean | null>(null);
	let backendHealthMessage = $state<string>('');

	onMount(async () => {
		const health = await checkBackendHealth();
		backendHealthy = health.healthy;
		backendHealthMessage = health.message;
	});

	async function handleCreateFlow() {
		try {
			const newFlow = await flowApi.createFlow({
				name: `Flow ${new Date().toISOString()}`,
				description: 'New flow created from UI'
			});
			// eslint-disable-next-line svelte/no-navigation-without-resolve
			await goto(`/flows/${newFlow.id}`);
		} catch (error) {
			console.error('Failed to create flow:', error);
			const message = getUserFriendlyErrorMessage(error);
			alert(`Failed to create flow: ${message}`);
		}
	}

	function handleFlowClick(flowId: string) {
		// eslint-disable-next-line svelte/no-navigation-without-resolve
		goto(`/flows/${flowId}`);
	}
</script>

<svelte:head>
	<title>Flows - SemStreams</title>
</svelte:head>

<main>
	<div class="page-header">
		<div class="header-row">
			<a href="/" class="back-link">← Graph</a>
			<h1>Flows</h1>
		</div>
		<p>Create and manage semantic stream processing flows</p>
	</div>

	{#if backendHealthy === false}
		<div class="connectivity-banner">
			<strong>Backend Not Available</strong>
			<p>{backendHealthMessage}</p>
			<p class="help-text">
				Please ensure your backend service is running.
			</p>
		</div>
	{/if}

	{#if data.error && backendHealthy !== false}
		<div class="error-banner">
			<strong>Error:</strong>
			{data.error}
		</div>
	{/if}

	<FlowList flows={data.flows} onCreate={handleCreateFlow} onFlowClick={handleFlowClick} />
</main>

<style>
	main {
		max-width: 1200px;
		margin: 0 auto;
		padding: 2rem;
	}

	.page-header {
		margin-bottom: 2rem;
	}

	.header-row {
		display: flex;
		align-items: center;
		gap: 1rem;
	}

	.back-link {
		color: var(--ui-interactive-primary);
		text-decoration: none;
		font-weight: 500;
		font-size: 0.875rem;
		padding: 0.25rem 0.5rem;
		border-radius: 4px;
		transition: background-color 0.2s;
	}

	.back-link:hover {
		background-color: var(--ui-surface-secondary);
	}

	.page-header h1 {
		font-size: 2rem;
		margin: 0;
		color: var(--ui-text-primary);
	}

	.page-header p {
		margin: 0.5rem 0 0 0;
		color: var(--ui-text-secondary);
		font-size: 1.125rem;
	}

	.connectivity-banner {
		padding: 1.5rem;
		margin-bottom: 1.5rem;
		background: var(--status-warning-container);
		color: var(--status-warning-on-container);
		border: 2px solid var(--status-warning);
		border-radius: 6px;
	}

	.connectivity-banner strong {
		display: block;
		font-size: 1.125rem;
		margin-bottom: 0.5rem;
	}

	.connectivity-banner p {
		margin: 0.5rem 0;
	}

	.connectivity-banner .help-text {
		font-size: 0.875rem;
		opacity: 0.8;
		margin-top: 1rem;
	}

	.error-banner {
		padding: 1rem;
		margin-bottom: 1.5rem;
		background: var(--status-error-container);
		color: var(--status-error-on-container);
		border: 1px solid var(--status-error);
		border-radius: 4px;
	}

	.error-banner strong {
		font-weight: 600;
	}
</style>
