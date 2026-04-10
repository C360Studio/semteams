<script lang="ts">
	/**
	 * ViewSwitcher - Toggle between Flow editor and Data visualization views
	 *
	 * Only visible when the flow is running, allowing users to switch between:
	 * - Flow: The flow editor canvas for building/editing
	 * - Data: The knowledge graph visualization for semantic exploration
	 */

	import type { ViewMode } from '$lib/types/ui-state';

	interface ViewSwitcherProps {
		currentView: ViewMode;
		onViewChange: (view: ViewMode) => void;
		disabled?: boolean;
	}

	let { currentView, onViewChange, disabled = false }: ViewSwitcherProps = $props();

	function handleClick(view: ViewMode) {
		if (!disabled && view !== currentView) {
			onViewChange(view);
		}
	}

	function handleKeydown(event: KeyboardEvent, view: ViewMode) {
		if (event.key === 'Enter' || event.key === ' ') {
			event.preventDefault();
			handleClick(view);
		}
	}
</script>

<div class="view-switcher" class:disabled data-testid="view-switcher">
	<button
		class="view-option"
		class:active={currentView === 'flow'}
		onclick={() => handleClick('flow')}
		onkeydown={(e) => handleKeydown(e, 'flow')}
		{disabled}
		aria-pressed={currentView === 'flow'}
		data-testid="view-switch-flow"
	>
		<span class="view-icon">◫</span>
		<span class="view-label">Flow</span>
	</button>
	<button
		class="view-option"
		class:active={currentView === 'data'}
		onclick={() => handleClick('data')}
		onkeydown={(e) => handleKeydown(e, 'data')}
		{disabled}
		aria-pressed={currentView === 'data'}
		data-testid="view-switch-data"
	>
		<span class="view-icon">◉</span>
		<span class="view-label">Data</span>
	</button>
</div>

<style>
	.view-switcher {
		display: flex;
		background: var(--ui-surface-tertiary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: 6px;
		padding: 2px;
		gap: 2px;
	}

	.view-switcher.disabled {
		opacity: 0.5;
		pointer-events: none;
	}

	.view-option {
		display: flex;
		align-items: center;
		gap: 6px;
		padding: 6px 12px;
		border: none;
		border-radius: 4px;
		background: transparent;
		color: var(--ui-text-secondary);
		font-size: 13px;
		font-weight: 500;
		cursor: pointer;
		transition: all 0.15s ease;
	}

	.view-option:hover:not(:disabled) {
		background: var(--ui-surface-secondary);
		color: var(--ui-text-primary);
	}

	.view-option.active {
		background: var(--ui-surface-primary);
		color: var(--ui-text-primary);
		box-shadow: 0 1px 2px rgba(0, 0, 0, 0.1);
	}

	.view-option:focus-visible {
		outline: 2px solid var(--ui-focus-ring);
		outline-offset: 1px;
	}

	.view-option:disabled {
		cursor: not-allowed;
	}

	.view-icon {
		font-size: 14px;
		line-height: 1;
	}

	.view-label {
		line-height: 1;
	}
</style>
