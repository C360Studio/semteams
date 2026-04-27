<script lang="ts">
	import favicon from '$lib/assets/favicon.svg';
	import '../styles/global.css';
	import { agentStore } from '$lib/stores/agentStore.svelte';
	import { systemStatus } from '$lib/stores/systemStatus.svelte';
	import TopNav from '$lib/components/layout/TopNav.svelte';
	import ChatBar from '$lib/components/layout/ChatBar.svelte';

	let { children } = $props();

	// Tie SSE + status-poll lifecycles to the layout via $effect — Svelte
	// runs the cleanup on layout teardown. systemStatus polls /health on
	// an interval and reads agentStore reactively for the SSE leg.
	$effect(() => {
		agentStore.connect();
		systemStatus.start();
		return () => {
			agentStore.disconnect();
			systemStatus.stop();
		};
	});
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
</svelte:head>

<div class="app-shell">
	<TopNav />
	<ChatBar />
	<main class="app-main">
		{@render children?.()}
	</main>
</div>

<style>
	.app-shell {
		display: flex;
		flex-direction: column;
		height: 100vh;
		overflow: hidden;
	}

	.app-main {
		flex: 1;
		overflow: hidden;
		display: flex;
	}
</style>
