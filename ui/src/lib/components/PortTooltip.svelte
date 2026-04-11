<script lang="ts">
	import type { PortTooltipContent } from '$lib/types/port';
	import { computePosition, flip, shift, offset } from '@floating-ui/dom';

	interface PortTooltipProps {
		content: PortTooltipContent;
		isVisible?: boolean;
		anchorElement?: HTMLElement;
	}

	let { content, isVisible = $bindable(false), anchorElement }: PortTooltipProps = $props();

	let tooltipElement: HTMLElement;
	let x = $state(0);
	let y = $state(0);

	// Update tooltip position using Floating UI
	$effect(() => {
		if (isVisible && anchorElement && tooltipElement) {
			computePosition(anchorElement, tooltipElement, {
				placement: 'top',
				middleware: [offset(8), flip(), shift({ padding: 8 })]
			}).then(({ x: newX, y: newY }) => {
				x = newX;
				y = newY;
			});
		}
	});

	const requirementColor = $derived(
		content.validationState === 'error'
			? '#B91C1C'
			: content.validationState === 'warning'
				? '#B45309'
				: '#047857'
	);

	const displayStyle = $derived(isVisible ? 'block' : 'none');

	const tooltipClasses = $derived(
		content.validationState === 'warning' ? 'port-tooltip tooltip-warning' : 'port-tooltip'
	);
</script>

<div
	bind:this={tooltipElement}
	data-testid="port-tooltip"
	class={tooltipClasses}
	style:left="{x}px"
	style:top="{y}px"
	style:display={displayStyle}
	role="tooltip"
	aria-live="polite"
>
		<div class="tooltip-header">
			<strong>{content.name}</strong>
			<span class="port-type">{content.type}</span>
		</div>
		<div class="tooltip-body">
			<div class="tooltip-row">
				<span class="label">Pattern:</span>
				<code>{content.pattern}</code>
			</div>
			<div class="tooltip-row">
				<span class="label">Requirement:</span>
				<span class="requirement" style:color={requirementColor}>
					{content.requirement}
				</span>
			</div>
			{#if content.description}
				<div class="tooltip-description">
					{content.description}
				</div>
			{/if}
			{#if content.validationMessage}
				<div
					class="validation-message"
					class:error={content.validationState === 'error'}
				>
					{content.validationMessage}
				</div>
			{/if}
		</div>
</div>

<style>
	.port-tooltip {
		position: absolute;
		z-index: 1000;
		background: white;
		border: 1px solid #e5e7eb;
		border-radius: 8px;
		padding: 12px;
		box-shadow: 0 4px 6px rgba(0, 0, 0, 0.1);
		max-width: 300px;
		font-size: 14px;
	}

	.tooltip-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 8px;
		padding-bottom: 8px;
		border-bottom: 1px solid #e5e7eb;
	}

	.port-type {
		font-size: 12px;
		color: #6b7280;
		text-transform: uppercase;
	}

	.tooltip-row {
		display: flex;
		justify-content: space-between;
		margin-bottom: 4px;
	}

	.label {
		color: #6b7280;
		margin-right: 8px;
	}

	.requirement {
		font-weight: 600;
	}

	.tooltip-description {
		margin-top: 8px;
		padding-top: 8px;
		border-top: 1px solid #e5e7eb;
		color: #374151;
	}

	.validation-message {
		margin-top: 8px;
		padding: 6px 8px;
		border-radius: 4px;
		background: #fef3c7;
		color: #92400e;
	}

	.validation-message.error {
		background: #fee2e2;
		color: #991b1b;
	}

	.tooltip-warning {
		border-color: #f59e0b;
	}
</style>
