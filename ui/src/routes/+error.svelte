<script lang="ts">
	import { page } from '$app/state';
</script>

<svelte:head>
	<title>{page.status} {page.error?.message || 'Error'} - SemTeams</title>
</svelte:head>

<div class="error-page" data-testid="error-page">
	<div class="error-content">
		<h1 class="error-status">{page.status}</h1>
		<p class="error-message">{page.error?.message || 'Something went wrong'}</p>
		<a href="/" class="error-home-link">← Back to Board</a>
	</div>
</div>

<style>
	.error-page {
		display: flex;
		align-items: center;
		justify-content: center;
		flex: 1;
		padding: 2rem;
	}

	.error-content {
		text-align: center;
		max-width: 32rem;
	}

	.error-status {
		font-size: 4rem;
		font-weight: 700;
		margin: 0 0 0.5rem;
		color: var(--ui-text-primary, #f3f4f6);
	}

	.error-message {
		font-size: 1.125rem;
		color: var(--ui-text-secondary, #9ca3af);
		margin: 0 0 2rem;
	}

	.error-home-link {
		display: inline-block;
		font-size: 0.875rem;
		color: var(--ui-interactive-primary, #3b82f6);
		text-decoration: none;
	}

	.error-home-link:hover {
		text-decoration: underline;
	}
</style>
