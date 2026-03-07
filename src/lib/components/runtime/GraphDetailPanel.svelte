<script lang="ts">
	/**
	 * GraphDetailPanel - Detail sidebar for selected entity
	 *
	 * Displays comprehensive information about the selected entity:
	 * - Entity ID breakdown (org.platform.domain.system.type.instance)
	 * - Properties table with confidence indicators
	 * - Outgoing and incoming relationships
	 * - Community membership
	 */

	import type { GraphEntity } from '$lib/types/graph';
	import { getEntityLabel, getEntityTypeLabel, parseEntityId } from '$lib/types/graph';
	import { getEntityColor, getPredicateColor, getConfidenceOpacity } from '$lib/utils/entity-colors';

	interface GraphDetailPanelProps {
		entity: GraphEntity | null;
		onClose?: () => void;
		onEntityClick?: (entityId: string) => void;
	}

	let { entity, onClose, onEntityClick }: GraphDetailPanelProps = $props();

	// Derived values
	const label = $derived(entity ? getEntityLabel(entity) : '');
	const typeLabel = $derived(entity ? getEntityTypeLabel(entity) : '');
	const entityColor = $derived(entity ? getEntityColor(entity.idParts) : '#888');

	// Format timestamp
	function formatTimestamp(ts: string | number): string {
		const date = new Date(typeof ts === 'number' ? ts : parseInt(ts, 10));
		return date.toLocaleString();
	}

	// Format confidence as percentage
	function formatConfidence(confidence: number): string {
		return `${(confidence * 100).toFixed(0)}%`;
	}

	// Get short predicate name
	function shortPredicate(predicate: string): string {
		const parts = predicate.split('.');
		return parts[parts.length - 1] || predicate;
	}

	// Handle clicking on a related entity
	function handleEntityClick(entityId: string) {
		onEntityClick?.(entityId);
	}
</script>

{#if entity}
	<div class="detail-panel" data-testid="graph-detail-panel">
		<!-- Header -->
		<div class="panel-header">
			<div class="entity-badge" style="background-color: {entityColor}">
				{entity.idParts.type.charAt(0).toUpperCase()}
			</div>
			<div class="entity-title">
				<h3 class="entity-label">{label}</h3>
				<span class="entity-type">{typeLabel}</span>
			</div>
			{#if onClose}
				<button class="close-button" onclick={onClose} aria-label="Close panel">×</button>
			{/if}
		</div>

		<!-- ID Breakdown -->
		<section class="section">
			<h4 class="section-title">Entity ID</h4>
			<div class="id-breakdown">
				<div class="id-part">
					<span class="id-label">org</span>
					<span class="id-value">{entity.idParts.org}</span>
				</div>
				<div class="id-part">
					<span class="id-label">platform</span>
					<span class="id-value">{entity.idParts.platform}</span>
				</div>
				<div class="id-part">
					<span class="id-label">domain</span>
					<span class="id-value">{entity.idParts.domain}</span>
				</div>
				<div class="id-part">
					<span class="id-label">system</span>
					<span class="id-value">{entity.idParts.system}</span>
				</div>
				<div class="id-part">
					<span class="id-label">type</span>
					<span class="id-value">{entity.idParts.type}</span>
				</div>
				<div class="id-part">
					<span class="id-label">instance</span>
					<span class="id-value">{entity.idParts.instance}</span>
				</div>
			</div>
		</section>

		<!-- Community -->
		{#if entity.community}
			<section class="section">
				<h4 class="section-title">Community</h4>
				<div class="community-info">
					<span
						class="community-badge"
						style="background-color: {entity.community.color}"
					></span>
					<span class="community-label">{entity.community.label || entity.community.id}</span>
				</div>
			</section>
		{/if}

		<!-- Properties -->
		{#if entity.properties.length > 0}
			<section class="section">
				<h4 class="section-title">Properties ({entity.properties.length})</h4>
				<div class="properties-list">
					{#each entity.properties as prop, idx (prop.predicate + idx)}
						<div class="property-row">
							<span class="property-predicate" style="color: {getPredicateColor(prop.predicate)}">
								{shortPredicate(prop.predicate)}
							</span>
							<span class="property-value" title={String(prop.object)}>{String(prop.object)}</span>
							<span
								class="property-confidence"
								style="opacity: {getConfidenceOpacity(prop.confidence)}"
								title="Confidence: {formatConfidence(prop.confidence)}"
							>
								{formatConfidence(prop.confidence)}
							</span>
						</div>
					{/each}
				</div>
			</section>
		{/if}

		<!-- Outgoing Relationships -->
		{#if entity.outgoing.length > 0}
			<section class="section">
				<h4 class="section-title">Outgoing ({entity.outgoing.length})</h4>
				<div class="relationships-list">
					{#each entity.outgoing as rel (rel.id)}
						<button
							class="relationship-row"
							onclick={() => handleEntityClick(rel.targetId)}
							title="Click to navigate"
						>
							<span class="rel-predicate" style="color: {getPredicateColor(rel.predicate)}">
								{shortPredicate(rel.predicate)}
							</span>
							<span class="rel-arrow">→</span>
							<span class="rel-target">{parseEntityId(rel.targetId).instance}</span>
							<span
								class="rel-confidence"
								style="opacity: {getConfidenceOpacity(rel.confidence)}"
							>
								{formatConfidence(rel.confidence)}
							</span>
						</button>
					{/each}
				</div>
			</section>
		{/if}

		<!-- Incoming Relationships -->
		{#if entity.incoming.length > 0}
			<section class="section">
				<h4 class="section-title">Incoming ({entity.incoming.length})</h4>
				<div class="relationships-list">
					{#each entity.incoming as rel (rel.id)}
						<button
							class="relationship-row"
							onclick={() => handleEntityClick(rel.sourceId)}
							title="Click to navigate"
						>
							<span class="rel-source">{parseEntityId(rel.sourceId).instance}</span>
							<span class="rel-arrow">←</span>
							<span class="rel-predicate" style="color: {getPredicateColor(rel.predicate)}">
								{shortPredicate(rel.predicate)}
							</span>
							<span
								class="rel-confidence"
								style="opacity: {getConfidenceOpacity(rel.confidence)}"
							>
								{formatConfidence(rel.confidence)}
							</span>
						</button>
					{/each}
				</div>
			</section>
		{/if}

		<!-- Timestamps -->
		{#if entity.properties.length > 0}
			{@const latestProp = entity.properties.reduce((latest, prop) =>
				prop.timestamp > latest.timestamp ? prop : latest
			)}
			<section class="section section-footer">
				<span class="timestamp">Last updated: {formatTimestamp(latestProp.timestamp)}</span>
			</section>
		{/if}
	</div>
{:else}
	<div class="detail-panel empty" data-testid="graph-detail-panel-empty">
		<p class="empty-message">Select an entity to view details</p>
	</div>
{/if}

<style>
	.detail-panel {
		display: flex;
		flex-direction: column;
		height: 100%;
		background: var(--ui-surface-secondary);
		border-left: 1px solid var(--ui-border-subtle);
		min-width: 280px;
		max-width: 320px;
		overflow-y: auto;
	}

	.detail-panel.empty {
		justify-content: center;
		align-items: center;
	}

	.empty-message {
		color: var(--ui-text-secondary);
		font-size: 13px;
		font-style: italic;
	}

	/* Header */
	.panel-header {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 12px;
		border-bottom: 1px solid var(--ui-border-subtle);
		background: var(--ui-surface-primary);
	}

	.entity-badge {
		width: 36px;
		height: 36px;
		border-radius: 50%;
		display: flex;
		align-items: center;
		justify-content: center;
		color: white;
		font-weight: 600;
		font-size: 16px;
		flex-shrink: 0;
	}

	.entity-title {
		flex: 1;
		min-width: 0;
	}

	.entity-label {
		margin: 0;
		font-size: 14px;
		font-weight: 600;
		color: var(--ui-text-primary);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.entity-type {
		font-size: 11px;
		color: var(--ui-text-secondary);
		text-transform: uppercase;
		letter-spacing: 0.5px;
	}

	.close-button {
		width: 28px;
		height: 28px;
		border: none;
		border-radius: 4px;
		background: transparent;
		color: var(--ui-text-secondary);
		font-size: 20px;
		cursor: pointer;
		display: flex;
		align-items: center;
		justify-content: center;
		flex-shrink: 0;
	}

	.close-button:hover {
		background: var(--ui-surface-tertiary);
		color: var(--ui-text-primary);
	}

	/* Sections */
	.section {
		padding: 12px;
		border-bottom: 1px solid var(--ui-border-subtle);
	}

	.section-title {
		margin: 0 0 8px 0;
		font-size: 11px;
		font-weight: 600;
		color: var(--ui-text-secondary);
		text-transform: uppercase;
		letter-spacing: 0.5px;
	}

	.section-footer {
		border-bottom: none;
	}

	/* ID Breakdown */
	.id-breakdown {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 6px;
	}

	.id-part {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.id-label {
		font-size: 9px;
		color: var(--ui-text-tertiary);
		text-transform: uppercase;
	}

	.id-value {
		font-size: 12px;
		color: var(--ui-text-primary);
		font-family: var(--font-mono);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	/* Community */
	.community-info {
		display: flex;
		align-items: center;
		gap: 8px;
	}

	.community-badge {
		width: 12px;
		height: 12px;
		border-radius: 3px;
	}

	.community-label {
		font-size: 13px;
		color: var(--ui-text-primary);
	}

	/* Properties */
	.properties-list {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.property-row {
		display: grid;
		grid-template-columns: 1fr 1fr auto;
		gap: 8px;
		align-items: center;
		padding: 4px 6px;
		background: var(--ui-surface-primary);
		border-radius: 4px;
		font-size: 11px;
	}

	.property-predicate {
		font-weight: 500;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.property-value {
		color: var(--ui-text-primary);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		font-family: var(--font-mono);
	}

	.property-confidence {
		font-size: 10px;
		color: var(--ui-text-secondary);
		min-width: 32px;
		text-align: right;
	}

	/* Relationships */
	.relationships-list {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.relationship-row {
		display: flex;
		align-items: center;
		gap: 6px;
		padding: 6px 8px;
		background: var(--ui-surface-primary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: 4px;
		font-size: 11px;
		cursor: pointer;
		transition: all 0.2s;
		text-align: left;
		width: 100%;
	}

	.relationship-row:hover {
		border-color: var(--ui-border-strong);
		background: var(--ui-surface-tertiary);
	}

	.rel-predicate {
		font-weight: 500;
		white-space: nowrap;
	}

	.rel-arrow {
		color: var(--ui-text-tertiary);
		flex-shrink: 0;
	}

	.rel-source,
	.rel-target {
		color: var(--ui-text-primary);
		font-family: var(--font-mono);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		flex: 1;
		min-width: 0;
	}

	.rel-confidence {
		font-size: 10px;
		color: var(--ui-text-secondary);
		flex-shrink: 0;
	}

	/* Footer */
	.timestamp {
		font-size: 10px;
		color: var(--ui-text-tertiary);
	}
</style>
