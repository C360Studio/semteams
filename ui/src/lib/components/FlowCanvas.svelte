<script lang="ts">
	/**
	 * FlowCanvas - D3-based flow visualization canvas
	 *
	 * Renders flow nodes and edges using SVG with D3 for:
	 * - Automatic layout calculation
	 * - Zoom and pan controls
	 * - Click-to-edit interaction
	 *
	 * This replaces the XYFlow-based FlowCanvas with a simpler,
	 * visualization-focused implementation.
	 */

	import { onMount } from 'svelte';
	import * as d3 from 'd3';
	import type { FlowNode, FlowConnection } from '$lib/types/flow';
	import type { ValidatedPort } from '$lib/types/port';
	import {
		layoutNodes,
		layoutEdges,
		calculateCanvasBounds,
		createZoomBehavior,
		applyZoom,
		fitToContent
	} from '$lib/utils/d3-layout';

	import FlowNodeComponent from './FlowNode.svelte';
	import FlowEdge from './FlowEdge.svelte';

	interface PortsMap {
		[nodeId: string]: {
			input_ports: ValidatedPort[];
			output_ports: ValidatedPort[];
		};
	}

	interface FlowCanvasProps {
		nodes: FlowNode[];
		connections: FlowConnection[];
		/** Port metadata per node (from validation) */
		portsMap?: PortsMap;
		/** Currently selected node ID */
		selectedNodeId?: string | null;
		/** Callback when a node is clicked */
		onNodeClick?: (nodeId: string) => void;
	}

	let {
		nodes,
		connections,
		portsMap = {},
		selectedNodeId = null,
		onNodeClick
	}: FlowCanvasProps = $props();

	// SVG element references
	let svgElement: SVGSVGElement;
	let containerElement: HTMLDivElement;

	// Zoom transform state
	let transform = $state({ x: 0, y: 0, k: 1 });

	// Computed layout
	const layoutedNodes = $derived(layoutNodes(nodes, connections));
	const layoutedEdges = $derived(layoutEdges(connections, layoutedNodes));
	const bounds = $derived(calculateCanvasBounds(layoutedNodes));

	// Zoom behavior
	let zoomBehavior: d3.ZoomBehavior<SVGSVGElement, unknown> | null = null;

	onMount(() => {
		// Initialize zoom
		zoomBehavior = createZoomBehavior((newTransform) => {
			transform = {
				x: newTransform.x,
				y: newTransform.y,
				k: newTransform.k
			};
		});

		applyZoom(svgElement, zoomBehavior);

		// Initial fit
		requestAnimationFrame(() => {
			handleFitToContent();
		});

		// Window resize handler with debouncing
		let resizeTimeout: ReturnType<typeof setTimeout> | null = null;
		const handleResize = () => {
			if (resizeTimeout !== null) {
				clearTimeout(resizeTimeout);
			}
			resizeTimeout = setTimeout(() => {
				handleFitToContent();
				resizeTimeout = null;
			}, 200);
		};

		window.addEventListener('resize', handleResize);

		// Cleanup
		return () => {
			// Clean up resize listener
			window.removeEventListener('resize', handleResize);

			// Clear any pending resize timeout
			if (resizeTimeout !== null) {
				clearTimeout(resizeTimeout);
			}

			// Clean up D3 zoom behavior event listeners
			if (svgElement && zoomBehavior) {
				try {
					const selection = d3.select(svgElement);
					if (selection && typeof selection.on === 'function') {
						selection.on('.zoom', null);
					}
				} catch (error) {
					// Silently ignore cleanup errors in test environments
					console.debug('D3 zoom cleanup skipped:', error);
				}
			}
		};
	});

	// Re-fit when nodes change significantly
	$effect(() => {
		if (nodes.length > 0 && zoomBehavior && svgElement && containerElement) {
			// Debounce to avoid too many re-fits
			const timeout = setTimeout(() => {
				handleFitToContent();
			}, 100);
			return () => clearTimeout(timeout);
		}
	});

	function handleFitToContent() {
		if (!svgElement || !containerElement || !zoomBehavior) return;

		const rect = containerElement.getBoundingClientRect();
		fitToContent(svgElement, zoomBehavior, bounds, rect.width, rect.height);
	}

	function handleNodeClick(nodeId: string) {
		onNodeClick?.(nodeId);
	}

	// Get ports for a specific node
	function getNodePorts(nodeId: string): { input: ValidatedPort[]; output: ValidatedPort[] } {
		const ports = portsMap[nodeId];
		return {
			input: ports?.input_ports || [],
			output: ports?.output_ports || []
		};
	}

	// Arrow marker colors
	const arrowMarkers = [
		{ id: 'arrow-default', color: 'var(--ui-interactive-primary)' },
		{ id: 'arrow-error', color: 'var(--status-error)' },
		{ id: 'arrow-warning', color: 'var(--status-warning)' },
		{ id: 'arrow-auto', color: 'var(--ui-interactive-secondary)' }
	];
</script>

<div class="flow-canvas-container" bind:this={containerElement}>
	<svg
		id="flow-canvas"
		class="flow-canvas"
		bind:this={svgElement}
		role="img"
		aria-label="Flow diagram"
	>
		<!-- Marker definitions -->
		<defs>
			{#each arrowMarkers as marker (marker.id)}
				<marker
					id={marker.id}
					viewBox="0 0 10 10"
					refX="9"
					refY="5"
					markerWidth="6"
					markerHeight="6"
					orient="auto-start-reverse"
				>
					<path d="M 0 0 L 10 5 L 0 10 z" fill={marker.color} />
				</marker>
			{/each}
		</defs>

		<!-- Zoomable/pannable content -->
		<g class="canvas-content" transform="translate({transform.x}, {transform.y}) scale({transform.k})">
			<!-- Background grid -->
			<defs>
				<pattern id="grid" width="20" height="20" patternUnits="userSpaceOnUse">
					<path
						d="M 20 0 L 0 0 0 20"
						fill="none"
						stroke="var(--ui-border-subtle)"
						stroke-width="0.5"
						opacity="0.5"
					/>
				</pattern>
			</defs>
			<rect
				x="-5000"
				y="-5000"
				width="10000"
				height="10000"
				fill="url(#grid)"
			/>

			<!-- Edges (rendered first, behind nodes) -->
			{#each layoutedEdges as edge (edge.id)}
				<FlowEdge {edge} markerId="arrow-default" />
			{/each}

			<!-- Nodes -->
			{#each layoutedNodes as node (node.id)}
				{@const ports = getNodePorts(node.id)}
				<FlowNodeComponent
					{node}
					inputPorts={ports.input}
					outputPorts={ports.output}
					selected={selectedNodeId === node.id}
					onclick={handleNodeClick}
				/>
			{/each}
		</g>
	</svg>

	<!-- Canvas controls -->
	<div class="canvas-controls">
		<button
			type="button"
			class="control-button"
			onclick={() => {
				if (svgElement && zoomBehavior) {
					d3.select(svgElement).transition().call(zoomBehavior.scaleBy, 1.2);
				}
			}}
			aria-label="Zoom in"
		>
			+
		</button>
		<button
			type="button"
			class="control-button"
			onclick={() => {
				if (svgElement && zoomBehavior) {
					d3.select(svgElement).transition().call(zoomBehavior.scaleBy, 0.8);
				}
			}}
			aria-label="Zoom out"
		>
			−
		</button>
		<button
			type="button"
			class="control-button"
			onclick={handleFitToContent}
			aria-label="Fit to content"
		>
			⊡
		</button>
	</div>

	<!-- Empty state -->
	{#if nodes.length === 0}
		<div class="empty-state">
			<p>No components in this flow.</p>
			<p class="hint">Add components using the sidebar or describe your flow to the AI.</p>
		</div>
	{/if}
</div>

<style>
	.flow-canvas-container {
		position: relative;
		width: 100%;
		height: 100%;
		min-height: 400px;
		background: var(--ui-surface-primary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-md);
		overflow: hidden;
	}

	.flow-canvas {
		width: 100%;
		height: 100%;
		cursor: grab;
	}

	.flow-canvas:active {
		cursor: grabbing;
	}

	.canvas-controls {
		position: absolute;
		bottom: 1rem;
		left: 1rem;
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		z-index: 10;
	}

	.control-button {
		width: 32px;
		height: 32px;
		padding: 0;
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 18px;
		font-weight: bold;
		background: var(--ui-surface-primary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-md);
		cursor: pointer;
		transition: background 0.2s;
	}

	.control-button:hover {
		background: var(--ui-surface-secondary);
	}

	.empty-state {
		position: absolute;
		top: 50%;
		left: 50%;
		transform: translate(-50%, -50%);
		text-align: center;
		color: var(--ui-text-secondary);
	}

	.empty-state p {
		margin: 0.5rem 0;
	}

	.empty-state .hint {
		font-size: 0.875rem;
	}
</style>
