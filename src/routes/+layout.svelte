<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import favicon from '$lib/assets/favicon.svg';
	import '../styles/global.css';
	import { agentStore } from '$lib/stores/agentStore.svelte';

	let { children } = $props();

	onMount(() => {
		agentStore.connect();
	});

	onDestroy(() => {
		agentStore.disconnect();
	});
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
</svelte:head>

{@render children?.()}
