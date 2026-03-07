<script lang="ts">
	/**
	 * GraphEdge - SVG edge component for knowledge graph visualization
	 *
	 * Renders a relationship between two entities with:
	 * - Curved line connecting rectangular nodes
	 * - Opacity based on confidence
	 * - Predicate label (at high LOD)
	 * - Arrow marker at target
	 */

	import type { GraphLayoutNode, GraphLayoutEdge } from '$lib/types/graph';
	import type { LevelOfDetail } from '$lib/utils/graph-layout';
	import { getPredicateColor } from '$lib/utils/entity-colors';

	interface GraphEdgeProps {
		edge: GraphLayoutEdge;
		selected?: boolean;
		highlighted?: boolean;
		lod?: LevelOfDetail;
		markerId?: string;
	}

	let {
		edge,
		selected = false,
		highlighted = false,
		lod = 'medium',
		markerId = 'graph-arrow'
	}: GraphEdgeProps = $props();

	// Get source and target nodes
	const sourceNode = $derived(edge.source as GraphLayoutNode);
	const targetNode = $derived(edge.target as GraphLayoutNode);

	// Calculate attachment points for rectangular nodes
	// Source: right edge, vertically centered
	const sourceX = $derived(sourceNode ? sourceNode.x + sourceNode.width : 0);
	const sourceY = $derived(sourceNode ? sourceNode.y + sourceNode.height / 2 : 0);
	// Target: left edge, vertically centered
	const targetX = $derived(targetNode ? targetNode.x : 0);
	const targetY = $derived(targetNode ? targetNode.y + targetNode.height / 2 : 0);

	// Calculate midpoint for label
	const midX = $derived((sourceX + targetX) / 2);
	const midY = $derived((sourceY + targetY) / 2);

	// Calculate path with curve
	const path = $derived.by(() => {
		const dx = targetX - sourceX;

		// Horizontal offset from source (right edge) to start bezier
		const startX = sourceX;
		const startY = sourceY;
		// End before target (left edge) to leave room for arrow
		const endX = targetX - 8;
		const endY = targetY;

		// Control point offset (creates a smooth horizontal curve)
		const cpOffset = Math.min(Math.abs(dx) * 0.4, 60);

		// For left-to-right layout, use horizontal bezier control points
		const cp1x = startX + cpOffset;
		const cp1y = startY;
		const cp2x = endX - cpOffset;
		const cp2y = endY;

		return `M ${startX} ${startY} C ${cp1x} ${cp1y}, ${cp2x} ${cp2y}, ${endX} ${endY}`;
	});

	// Visual properties
	const showLabel = $derived(lod === 'high');
	const predicateColor = $derived(getPredicateColor(edge.relationship.predicate));
	const strokeWidth = $derived(selected ? 2.5 : highlighted ? 2 : 1.5);
	const opacity = $derived(edge.opacity * (selected || highlighted ? 1 : 0.7));

	// Extract short predicate label (last part of dotted notation)
	const shortPredicate = $derived.by(() => {
		const parts = edge.relationship.predicate.split('.');
		return parts[parts.length - 1] || edge.relationship.predicate;
	});
</script>

<g
	class="graph-edge"
	class:selected
	class:highlighted
	data-edge-id={edge.id}
	data-predicate={edge.relationship.predicate}
>
	<!-- Invisible wider path for easier selection -->
	<path class="edge-hitbox" d={path} stroke="transparent" stroke-width="15" fill="none" />

	<!-- Visible edge path -->
	<path
		class="edge-path"
		d={path}
		stroke={predicateColor}
		stroke-width={strokeWidth}
		stroke-opacity={opacity}
		fill="none"
		marker-end="url(#{markerId})"
	/>

	<!-- Predicate label (shown at high LOD) -->
	{#if showLabel}
		<g transform="translate({midX}, {midY})">
			<rect
				class="label-background"
				x="-30"
				y="-8"
				width="60"
				height="16"
				rx="3"
				fill="var(--ui-surface-primary)"
				fill-opacity="0.9"
			/>
			<text class="edge-label" text-anchor="middle" dominant-baseline="middle" fill={predicateColor}>
				{shortPredicate}
			</text>
		</g>
	{/if}

	<!-- Confidence indicator (small circle at midpoint) -->
	{#if edge.relationship.confidence < 1.0 && lod !== 'low'}
		<circle
			class="confidence-indicator"
			cx={midX}
			cy={midY + 15}
			r="4"
			fill="var(--ui-surface-secondary)"
			stroke={predicateColor}
			stroke-width="1"
			stroke-opacity={opacity}
		>
			<title>Confidence: {(edge.relationship.confidence * 100).toFixed(0)}%</title>
		</circle>
	{/if}
</g>

<style>
	.graph-edge {
		pointer-events: stroke;
	}

	.edge-hitbox {
		cursor: pointer;
	}

	.edge-path {
		transition:
			stroke-width 0.2s,
			stroke-opacity 0.2s;
	}

	.graph-edge.selected .edge-path,
	.graph-edge.highlighted .edge-path {
		stroke-opacity: 1;
	}

	.edge-label {
		font-size: 9px;
		font-weight: 500;
		pointer-events: none;
		user-select: none;
	}

	.label-background {
		pointer-events: none;
	}

	.confidence-indicator {
		pointer-events: none;
	}
</style>
