<script lang="ts">
	import LogsTab from './runtime/LogsTab.svelte';
	import MessagesTab from './runtime/MessagesTab.svelte';
	import MetricsTab from './runtime/MetricsTab.svelte';
	import HealthTab from './runtime/HealthTab.svelte';

	type TabId = 'logs' | 'messages' | 'metrics' | 'health';

	interface RuntimePanelProps {
		isOpen: boolean;
		height?: number;
		flowId: string;
		onClose?: () => void;
		/** Whether in full-screen monitor mode */
		isMonitorMode?: boolean;
		/** Toggle monitor mode callback */
		onToggleMonitorMode?: () => void;
	}

	let {
		isOpen = false,
		height = 300,
		flowId,
		onClose,
		isMonitorMode = false,
		onToggleMonitorMode
	}: RuntimePanelProps = $props();

	// Tab state
	let activeTab = $state<TabId>('logs');

	// Handle Esc key to close panel
	function handleKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape' && isOpen) {
			event.preventDefault();
			onClose?.();
		}
	}

	// Handle tab change
	function handleTabChange(tabId: TabId) {
		activeTab = tabId;
	}
</script>

<svelte:window onkeydown={handleKeydown} />

{#if isOpen}
	<div
		class="runtime-panel"
		class:monitor-mode={isMonitorMode}
		style={isMonitorMode ? '' : `height: ${height}px;`}
		data-testid="runtime-panel"
	>
		<header>
			<div class="header-content">
				<h3>Runtime Debugging</h3>
				<div class="tab-nav" role="tablist" aria-label="Runtime debugging tabs">
					<button
						role="tab"
						id="tab-logs"
						aria-selected={activeTab === 'logs'}
						aria-controls="logs-panel"
						class="tab-button"
						class:active={activeTab === 'logs'}
						onclick={() => handleTabChange('logs')}
						data-testid="tab-logs"
					>
						Logs
					</button>
					<button
						role="tab"
						id="tab-messages"
						aria-selected={activeTab === 'messages'}
						aria-controls="messages-panel"
						class="tab-button"
						class:active={activeTab === 'messages'}
						onclick={() => handleTabChange('messages')}
						data-testid="tab-messages"
					>
						Messages
					</button>
					<button
						role="tab"
						id="tab-metrics"
						aria-selected={activeTab === 'metrics'}
						aria-controls="metrics-panel"
						class="tab-button"
						class:active={activeTab === 'metrics'}
						onclick={() => handleTabChange('metrics')}
						data-testid="tab-metrics"
					>
						Metrics
					</button>
					<button
						role="tab"
						id="tab-health"
						aria-selected={activeTab === 'health'}
						aria-controls="health-panel"
						class="tab-button"
						class:active={activeTab === 'health'}
						onclick={() => handleTabChange('health')}
						data-testid="tab-health"
					>
						Health
					</button>
				</div>
			</div>
			<div class="header-actions">
				<button
					class="action-button"
					onclick={onToggleMonitorMode}
					aria-label={isMonitorMode ? 'Exit monitor mode' : 'Enter monitor mode'}
					title={isMonitorMode ? 'Exit full screen' : 'Full screen'}
					data-testid="monitor-mode-toggle"
				>
					{#if isMonitorMode}
						<span class="icon">⊖</span>
					{:else}
						<span class="icon">⊕</span>
					{/if}
				</button>
				<button class="action-button close-button" onclick={onClose} aria-label="Close runtime panel">
					✕
				</button>
			</div>
		</header>

		<div class="panel-body">
			{#if activeTab === 'logs'}
				<div
					id="logs-panel"
					role="tabpanel"
					aria-labelledby="tab-logs"
					class="tab-content"
					data-testid="logs-panel"
				>
					<LogsTab {flowId} isActive={activeTab === 'logs'} />
				</div>
			{:else if activeTab === 'messages'}
				<div
					id="messages-panel"
					role="tabpanel"
					aria-labelledby="tab-messages"
					class="tab-content"
					data-testid="messages-panel"
				>
					<MessagesTab {flowId} isActive={activeTab === 'messages'} />
				</div>
			{:else if activeTab === 'metrics'}
				<div
					id="metrics-panel"
					role="tabpanel"
					aria-labelledby="tab-metrics"
					class="tab-content"
					data-testid="metrics-panel"
				>
					<MetricsTab {flowId} isActive={activeTab === 'metrics'} />
				</div>
			{:else if activeTab === 'health'}
				<div
					id="health-panel"
					role="tabpanel"
					aria-labelledby="tab-health"
					class="tab-content"
					data-testid="health-panel"
				>
					<HealthTab {flowId} isActive={activeTab === 'health'} />
				</div>
			{/if}
		</div>
	</div>
{/if}

<style>
	.runtime-panel {
		display: flex;
		flex-direction: column;
		background: var(--ui-surface-primary);
		border-top: 2px solid var(--ui-border-emphasis);
		animation: slideUp 300ms ease-out;
		overflow: hidden;
	}

	.runtime-panel.monitor-mode {
		height: 100% !important;
		border-top: none;
		animation: none;
	}

	/* Slide up animation */
	@keyframes slideUp {
		from {
			transform: translateY(100%);
			opacity: 0;
		}
		to {
			transform: translateY(0);
			opacity: 1;
		}
	}

	header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 0.75rem 1rem;
		border-bottom: 1px solid var(--ui-border-subtle);
		background: var(--ui-surface-tertiary);
	}

	.header-content {
		display: flex;
		align-items: center;
		gap: 1.5rem;
		flex: 1;
	}

	header h3 {
		margin: 0;
		font-size: 0.875rem;
		font-weight: 600;
		color: var(--ui-text-primary);
	}

	/* Tab Navigation */
	.tab-nav {
		display: flex;
		gap: 0.5rem;
	}

	.tab-button {
		padding: 0.5rem 1rem;
		border: none;
		background: none;
		cursor: pointer;
		font-size: 0.875rem;
		font-weight: 500;
		color: var(--ui-text-secondary);
		border-bottom: 2px solid transparent;
		transition: all 0.2s;
		position: relative;
	}

	.tab-button:hover:not(.disabled) {
		color: var(--ui-text-primary);
		background: var(--ui-surface-secondary);
	}

	.tab-button.active {
		color: var(--ui-interactive-primary);
		border-bottom-color: var(--ui-interactive-primary);
	}

	.tab-button:focus-visible {
		outline: 2px solid var(--ui-focus-ring);
		outline-offset: 2px;
	}

	.header-actions {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.action-button {
		background: none;
		border: none;
		font-size: 1.25rem;
		cursor: pointer;
		padding: 0;
		width: 1.75rem;
		height: 1.75rem;
		display: flex;
		align-items: center;
		justify-content: center;
		border-radius: 4px;
		color: var(--ui-text-secondary);
		transition: all 0.2s;
	}

	.action-button:hover {
		background: var(--ui-surface-secondary);
		color: var(--ui-text-primary);
	}

	.action-button .icon {
		font-size: 1rem;
	}

	.panel-body {
		flex: 1;
		overflow: hidden;
		background: var(--ui-surface-primary);
	}

	.tab-content {
		height: 100%;
		overflow: hidden;
	}
</style>
