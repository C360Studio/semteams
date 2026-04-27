<script lang="ts">
	import favicon from '$lib/assets/favicon.svg';
	import '../styles/global.css';
	import { agentStore } from '$lib/stores/agentStore.svelte';
	import TopNav from '$lib/components/layout/TopNav.svelte';
	import ChatBar from '$lib/components/layout/ChatBar.svelte';

	let { children } = $props();

	// Tie the SSE connection lifecycle to the layout via $effect — Svelte
	// runs the cleanup (disconnect) on layout teardown, and if we ever
	// gate the connection on a reactive dep (config rune, auth token)
	// $effect re-runs naturally where onMount/onDestroy would not.
	$effect(() => {
		agentStore.connect();
		return () => agentStore.disconnect();
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
