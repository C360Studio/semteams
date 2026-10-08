<script lang="ts">
	import favicon from '$lib/assets/favicon.svg';
	import '../styles/global.css';
	import { page } from '$app/state';
	import { agentStore } from '$lib/stores/agentStore.svelte';
	import { systemStatus } from '$lib/stores/systemStatus.svelte';
	import { taskRefs } from '$lib/stores/taskRefs.svelte';
	import { taskStore } from '$lib/stores/taskStore.svelte';
	import { runStatus } from '$lib/stores/runStatus.svelte';
	import { controlsStore } from '$lib/stores/controlsStore.svelte';
	import TopNav from '$lib/components/layout/TopNav.svelte';
	import ChatBar from '$lib/components/layout/ChatBar.svelte';
	import ControlsNotice from '$lib/components/board/ControlsNotice.svelte';

	let { children } = $props();

	// Tie SSE + status-poll lifecycles to the layout via $effect — Svelte
	// runs the cleanup on layout teardown. systemStatus polls /health on
	// an interval and reads agentStore reactively for the SSE leg.
	// runStatus polls /graph/triples for run-level pause markers (ADR-053
	// Phase 4b-2 / 4c) so the board can surface "Waiting on you" badges.
	// controlsStore reads each rule-fired loop's entity from the same endpoint
	// (once, cached) to tell run controls from run members, so the board can fold
	// only the controls into the card of the run that fired them.
	$effect(() => {
		agentStore.connect();
		systemStatus.start();
		runStatus.start();
		controlsStore.start();
		return () => {
			agentStore.disconnect();
			systemStatus.stop();
			runStatus.stop();
			controlsStore.stop();
		};
	});

	// Auto-assign #N short refs to board cards as they arrive.
	// $effect re-runs whenever agentStore.loops changes; ensure() is
	// idempotent so already-assigned loops are no-ops. Keeping this in
	// the layout (not the store) because $effect can't run at
	// module scope and we want refs minted as soon as loops appear,
	// before any consumer renders the card. A loop that is not a card for good
	// (a rule-fired candidate still being classified, or a control that folded into
	// its run's card) is not minted, so folded controls do not burn numbers; a
	// control that stays a card (taskStore.isRefEligible) is. The effect reads
	// controlsStore state, so resolution re-runs it.
	$effect(() => {
		for (const loop of agentStore.loopsList) {
			if (taskStore.isRefEligible(loop)) taskRefs.ensure(loop.loop_id);
		}
	});
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
</svelte:head>

<div class="app-shell">
	<TopNav />
	{#if page.route.id === '/'}
		<!-- The board is the only view that folds controls into cards. -->
		<ControlsNotice />
	{/if}
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
