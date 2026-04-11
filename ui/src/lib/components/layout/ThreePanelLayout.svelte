<script lang="ts">
	/**
	 * ThreePanelLayout - VS Code-style three-panel layout
	 *
	 * Features:
	 * - Left panel (Explorer): collapsible, resizable
	 * - Center panel (Canvas): flexible, always visible
	 * - Right panel (Properties): collapsible, resizable
	 * - Resize handles between panels
	 * - Keyboard shortcuts for panel toggle
	 * - Responsive auto-collapse
	 *
	 * Usage:
	 * ```svelte
	 * <ThreePanelLayout
	 *   leftPanelOpen={true}
	 *   rightPanelOpen={true}
	 *   leftPanelWidth={280}
	 *   rightPanelWidth={320}
	 *   {onLayoutChange}
	 * >
	 *   {#snippet leftPanel()}...{/snippet}
	 *   {#snippet centerPanel()}...{/snippet}
	 *   {#snippet rightPanel()}...{/snippet}
	 * </ThreePanelLayout>
	 * ```
	 */

	import type { Snippet } from 'svelte';
	import ResizeHandle from './ResizeHandle.svelte';

	interface ThreePanelLayoutProps {
		/** Left panel visibility */
		leftPanelOpen?: boolean;
		/** Right panel visibility */
		rightPanelOpen?: boolean;
		/** Left panel width in pixels */
		leftPanelWidth?: number;
		/** Right panel width in pixels */
		rightPanelWidth?: number;
		/** Callback when left panel width changes */
		onLeftWidthChange?: (width: number) => void;
		/** Callback when right panel width changes */
		onRightWidthChange?: (width: number) => void;
		/** Callback to toggle left panel */
		onToggleLeft?: () => void;
		/** Callback to toggle right panel */
		onToggleRight?: () => void;
		/** Content for left panel */
		leftPanel: Snippet;
		/** Content for center panel */
		centerPanel: Snippet;
		/** Content for right panel */
		rightPanel: Snippet;
	}

	let {
		leftPanelOpen = true,
		rightPanelOpen = true,
		leftPanelWidth = 280,
		rightPanelWidth = 320,
		onLeftWidthChange,
		onRightWidthChange,
		onToggleLeft,
		onToggleRight,
		leftPanel,
		centerPanel,
		rightPanel
	}: ThreePanelLayoutProps = $props();

	// Constraints from CSS variables (also defined in colors.css)
	const LEFT_MIN = 200;
	const LEFT_MAX = 400;
	const RIGHT_MIN = 240;
	const RIGHT_MAX = 480;

	// Local width state for smooth dragging
	// We need mutable state that syncs from props but can be modified during drag,
	// then commits back via callbacks. This intentionally uses $state + $effect
	// rather than writable $derived because drag operations require local mutations.
	// eslint-disable-next-line svelte/prefer-writable-derived
	let localLeftWidth = $state(leftPanelWidth);
	// eslint-disable-next-line svelte/prefer-writable-derived
	let localRightWidth = $state(rightPanelWidth);

	// Reset local state when props change (e.g., from store)
	$effect(() => {
		localLeftWidth = leftPanelWidth;
	});

	$effect(() => {
		localRightWidth = rightPanelWidth;
	});

	// Handle left panel resize
	function handleLeftResize(delta: number) {
		const newWidth = Math.min(Math.max(localLeftWidth + delta, LEFT_MIN), LEFT_MAX);
		localLeftWidth = newWidth;
	}

	function handleLeftResizeEnd() {
		onLeftWidthChange?.(localLeftWidth);
	}

	// Handle right panel resize
	function handleRightResize(delta: number) {
		const newWidth = Math.min(Math.max(localRightWidth + delta, RIGHT_MIN), RIGHT_MAX);
		localRightWidth = newWidth;
	}

	function handleRightResizeEnd() {
		onRightWidthChange?.(localRightWidth);
	}

	// Keyboard shortcuts
	function handleKeyDown(event: KeyboardEvent) {
		// Check for Cmd/Ctrl modifier
		const isMod = event.metaKey || event.ctrlKey;
		if (!isMod) return;

		// Cmd+B: Toggle left panel
		if (event.key === 'b' && !event.shiftKey) {
			event.preventDefault();
			onToggleLeft?.();
			return;
		}

		// Cmd+J: Toggle right panel
		if (event.key === 'j' && !event.shiftKey) {
			event.preventDefault();
			onToggleRight?.();
			return;
		}

		// Cmd+\: Toggle both panels (focus mode)
		if (event.key === '\\') {
			event.preventDefault();
			onToggleLeft?.();
			onToggleRight?.();
			return;
		}
	}

	// Compute grid template based on panel states
	const gridTemplate = $derived.by(() => {
		const parts: string[] = [];

		if (leftPanelOpen) {
			parts.push(`${localLeftWidth}px`);
			parts.push('auto'); // resize handle
		}

		parts.push('1fr'); // center always takes remaining space

		if (rightPanelOpen) {
			parts.push('auto'); // resize handle
			parts.push(`${localRightWidth}px`);
		}

		return parts.join(' ');
	});
</script>

<svelte:window onkeydown={handleKeyDown} />

<div
	class="three-panel-layout"
	style="grid-template-columns: {gridTemplate};"
	data-testid="three-panel-layout"
>
	{#if leftPanelOpen}
		<aside class="panel panel-left" data-testid="panel-left">
			{@render leftPanel()}
		</aside>

		<ResizeHandle direction="left" onResize={handleLeftResize} onResizeEnd={handleLeftResizeEnd} />
	{/if}

	<main class="panel panel-center" data-testid="panel-center">
		{@render centerPanel()}
	</main>

	{#if rightPanelOpen}
		<ResizeHandle
			direction="right"
			onResize={handleRightResize}
			onResizeEnd={handleRightResizeEnd}
		/>

		<aside class="panel panel-right" data-testid="panel-right">
			{@render rightPanel()}
		</aside>
	{/if}
</div>

<style>
	.three-panel-layout {
		display: grid;
		height: 100%;
		width: 100%;
		overflow: hidden;
		/* Smooth transition when panels collapse/expand */
		transition: grid-template-columns 200ms ease-out;
	}

	.panel {
		overflow: hidden;
		display: flex;
		flex-direction: column;
		/* Smooth content transitions */
		transition: opacity 150ms ease-out;
	}

	.panel-left {
		background: var(--explorer-background, var(--ui-surface-secondary));
		border-right: 1px solid var(--ui-border-subtle);
		min-width: 0;
	}

	.panel-center {
		background: var(--canvas-background, var(--ui-surface-primary));
		min-width: var(--panel-center-min-width, 400px);
	}

	.panel-right {
		background: var(--properties-background, var(--ui-surface-secondary));
		border-left: 1px solid var(--ui-border-subtle);
		min-width: 0;
	}
</style>
