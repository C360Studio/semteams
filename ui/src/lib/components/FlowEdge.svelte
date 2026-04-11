<script lang="ts">
	/**
	 * FlowEdge - SVG edge component for flow visualization
	 *
	 * Renders a connection between two nodes with:
	 * - Bezier curve path
	 * - Color-coded validation state
	 * - Arrow marker
	 */

	import type { LayoutEdge } from '$lib/utils/d3-layout';
	import { edgeToPath, getPathStyle } from '$lib/utils/edge-paths';

	interface FlowEdgeProps {
		edge: LayoutEdge;
		selected?: boolean;
		markerId?: string;
	}

	let { edge, selected = false, markerId = 'arrow' }: FlowEdgeProps = $props();

	// Generate path
	const path = $derived(edgeToPath(edge));

	// Get style based on validation state
	const style = $derived(getPathStyle(edge));

	// Determine if this is an auto-discovered connection
	const isAuto = $derived(edge.original.source === 'auto');
</script>

<g class="flow-edge" class:selected class:auto={isAuto} data-connection-id={edge.id} data-source={isAuto ? 'auto' : 'manual'}>
	<!-- Invisible wider path for easier click targeting -->
	<path
		class="edge-hitbox"
		d={path}
		stroke="transparent"
		stroke-width="20"
		fill="none"
	/>

	<!-- Visible edge path -->
	<path
		class="edge-path"
		d={path}
		stroke={style.stroke}
		stroke-width={style.strokeWidth}
		stroke-dasharray={style.strokeDasharray}
		fill="none"
		marker-end={style.showArrow ? `url(#${markerId})` : undefined}
	/>

	<!-- Connection label (midpoint) -->
	{#if edge.original.validationState === 'error' || edge.original.validationState === 'warning'}
		{@const midX = (edge.sourceX + edge.targetX) / 2}
		{@const midY = (edge.sourceY + edge.targetY) / 2}
		<g transform="translate({midX}, {midY})">
			<circle
				class="edge-status-indicator"
				r="8"
				fill={edge.original.validationState === 'error' ? 'var(--status-error)' : 'var(--status-warning)'}
			/>
			<text
				class="edge-status-icon"
				text-anchor="middle"
				dominant-baseline="central"
				fill="white"
				font-size="12"
			>
				{edge.original.validationState === 'error' ? '!' : '?'}
			</text>
			{#if edge.original.validationMessage}
				<title>{edge.original.validationMessage}</title>
			{/if}
		</g>
	{/if}
</g>

<style>
	.flow-edge {
		pointer-events: stroke;
	}

	.edge-path {
		transition: stroke 0.2s, stroke-width 0.2s;
	}

	.flow-edge:hover .edge-path {
		stroke-width: 3;
	}

	.flow-edge.selected .edge-path {
		stroke-width: 3;
		stroke: var(--ui-interactive-primary);
	}

	.flow-edge.auto .edge-path {
		opacity: 0.7;
	}

	.edge-status-indicator {
		filter: drop-shadow(0 1px 2px rgba(0, 0, 0, 0.2));
	}

	.edge-status-icon {
		font-weight: bold;
	}
</style>
