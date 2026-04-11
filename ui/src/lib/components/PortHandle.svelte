<script lang="ts">
	import type { ValidatedPort } from '$lib/types/port';
	import { computePortVisualStyle } from '$lib/utils/port-utils';
	import { Icon, ArrowPath, Eye, ServerStack, DocumentText } from 'svelte-hero-icons';

	// Note: ArrowPathRoundedSquare doesn't exist in heroicons, using closest alternative
	// Research.md will need updating to use available icons

	interface PortHandleProps {
		port: ValidatedPort;
	}

	let { port }: PortHandleProps = $props();

	// Compute visual style using utility function
	const visualStyle = $derived(computePortVisualStyle(port));

	// Map icon names to icon data
	const iconData: Record<string, any> = {
		'arrow-path-rounded-square': ArrowPath, // Fallback: ArrowPathRoundedSquare doesn't exist
		'arrow-path': ArrowPath,
		eye: Eye,
		server: ServerStack,
		'document-text': DocumentText
	};

	const currentIcon = $derived(iconData[visualStyle.iconName] || DocumentText);
</script>

<div
	class={visualStyle.cssClasses.join(' ')}
	data-port-handle
	data-port-id={port.name}
	data-port-name={port.name}
	data-direction={port.direction}
	data-type={port.type}
	data-required={port.required}
	data-border-pattern={visualStyle.borderPattern}
	aria-label={visualStyle.ariaLabel}
	style:border-color={visualStyle.color}
	style:border-style={visualStyle.borderPattern === 'solid' ? 'solid' : 'dashed'}
	role="button"
	tabindex="0"
>
	<Icon
		src={currentIcon}
		data-icon={visualStyle.iconName}
		class="port-icon"
		style="color: {visualStyle.color}"
	/>
</div>

<style>
	:global(.port-handle) {
		width: 24px;
		height: 24px;
		border-width: 2px;
		border-radius: 4px;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		background: white;
		cursor: crosshair;
		transition:
			box-shadow 0.2s ease,
			transform 0.2s ease;
	}

	:global(.port-icon) {
		width: 16px;
		height: 16px;
	}

	/* Hover effects */
	:global(.port-handle:hover) {
		box-shadow: 0 0 8px currentColor;
		transform: scale(1.1);
	}

	/* Focus indicator for accessibility */
	:global(.port-handle:focus) {
		outline: 2px solid currentColor;
		outline-offset: 2px;
	}

	/* Port type specific classes */
	:global(.port-nats_stream) {
		/* Blue-700 from theme */
	}

	:global(.port-nats_request) {
		/* Purple-700 from theme */
	}

	:global(.port-kv_watch) {
		/* Emerald-700 from theme */
	}

	:global(.port-network) {
		/* Orange-700 from theme */
	}

	:global(.port-file) {
		/* Gray-700 from theme */
	}

	/* Border pattern classes */
	:global(.port-solid) {
		/* Solid border for required ports */
	}

	:global(.port-dashed) {
		border-style: dashed !important;
	}
</style>
