<script lang="ts">
	import DataView from '$lib/components/DataView.svelte';
</script>

<svelte:head>
	<title>Graph - SemTeams</title>
</svelte:head>

<div class="graph-page">
	<DataView />
</div>

<style>
	.graph-page {
		width: 100%;
		height: 100%;
		overflow: hidden;
	}
</style>
