<script lang="ts">
	/**
	 * LogsTab Component - Real-time log display with filtering
	 * Uses runtimeStore for WebSocket-driven data
	 *
	 * Features:
	 * - Display logs from runtimeStore (WebSocket-driven)
	 * - Filter by log level (DEBUG, INFO, WARN, ERROR)
	 * - Filter by component/source
	 * - Auto-scroll toggle
	 * - Clear logs functionality
	 * - Color-coded log levels using design system
	 */

	import {
		runtimeStore,
		type LogLevel
	} from '$lib/stores/runtimeStore.svelte';

	interface LogsTabProps {
		flowId: string;
		isActive: boolean;
	}

	// Props passed from parent - may be used for future tab-specific logic
	let { flowId: _flowId, isActive: _isActive }: LogsTabProps = $props();

	// Local UI state
	let selectedLevel = $state<'all' | LogLevel>('all');
	let selectedComponent = $state<string>('all');
	let autoScroll = $state<boolean>(true);

	// Refs for DOM elements
	let logContainerRef: HTMLDivElement | null = null;

	// Filter out message-logger entries (those are shown in MessagesTab)
	const logsExcludingMessages = $derived(
		runtimeStore.logs.filter((log) => log.source !== 'message-logger')
	);

	// Extract unique sources from logs (excluding message-logger)
	const uniqueSources = $derived(
		Array.from(new Set(logsExcludingMessages.map((log) => log.source))).sort()
	);

	// Filter logs based on selected level and component
	const filteredLogs = $derived.by(() => {
		const levelOrder: Record<LogLevel, number> = {
			DEBUG: 0,
			INFO: 1,
			WARN: 2,
			ERROR: 3
		};

		return logsExcludingMessages.filter((log) => {
			// Filter by level
			if (selectedLevel !== 'all' && levelOrder[log.level] < levelOrder[selectedLevel]) {
				return false;
			}
			// Filter by source
			if (selectedComponent !== 'all' && log.source !== selectedComponent) {
				return false;
			}
			return true;
		});
	});

	/**
	 * Clear all logs
	 */
	function handleClearLogs() {
		runtimeStore.clearLogs();
		selectedLevel = 'all';
		selectedComponent = 'all';
	}

	/**
	 * Format timestamp for display (HH:MM:SS.mmm)
	 */
	function formatTimestamp(unixMs: number): string {
		try {
			const date = new Date(unixMs);
			const hours = date.getHours().toString().padStart(2, '0');
			const minutes = date.getMinutes().toString().padStart(2, '0');
			const seconds = date.getSeconds().toString().padStart(2, '0');
			const milliseconds = date.getMilliseconds().toString().padStart(3, '0');
			return `${hours}:${minutes}:${seconds}.${milliseconds}`;
		} catch {
			return String(unixMs);
		}
	}

	/**
	 * Get CSS color variable for log level
	 */
	function getLevelColor(level: LogLevel): string {
		const colors = {
			DEBUG: 'var(--ui-text-secondary)',
			INFO: 'var(--status-info)',
			WARN: 'var(--status-warning)',
			ERROR: 'var(--status-error)'
		};
		return colors[level];
	}

	// Effect: Scroll to bottom when filtered logs change (if auto-scroll enabled)
	$effect(() => {
		// Access filteredLogs to subscribe to changes
		const logs = filteredLogs;

		if (autoScroll && logContainerRef && logs.length > 0) {
			requestAnimationFrame(() => {
				if (logContainerRef) {
					logContainerRef.scrollTop = logContainerRef.scrollHeight;
				}
			});
		}
	});
</script>

<div class="logs-tab" data-testid="logs-tab">
	<!-- Filter Controls -->
	<div class="filter-bar">
		<div class="filter-controls">
			<label for="level-filter">
				<span class="filter-label">Level:</span>
				<select id="level-filter" bind:value={selectedLevel} data-testid="level-filter">
					<option value="all">All Levels</option>
					<option value="DEBUG">DEBUG</option>
					<option value="INFO">INFO</option>
					<option value="WARN">WARN</option>
					<option value="ERROR">ERROR</option>
				</select>
			</label>

			<label for="component-filter">
				<span class="filter-label">Source:</span>
				<select id="component-filter" bind:value={selectedComponent} data-testid="component-filter">
					<option value="all">All Sources</option>
					{#each uniqueSources as source (source)}
						<option value={source}>{source}</option>
					{/each}
				</select>
			</label>

			<button
				class="clear-button"
				onclick={handleClearLogs}
				data-testid="clear-logs-button"
				aria-label="Clear all logs"
			>
				Clear
			</button>
		</div>

		<div class="auto-scroll-control">
			<label for="auto-scroll">
				<input
					type="checkbox"
					id="auto-scroll"
					bind:checked={autoScroll}
					data-testid="auto-scroll-toggle"
				/>
				<span>Auto-scroll</span>
			</label>
		</div>
	</div>

	<!-- Connection Status -->
	{#if runtimeStore.error}
		<div class="connection-status error" data-testid="connection-error">
			<span class="status-icon">⚠</span>
			<span>{runtimeStore.error}</span>
		</div>
	{:else if !runtimeStore.connected}
		<div class="connection-status connecting" data-testid="connection-connecting">
			<span class="status-icon">⋯</span>
			<span>Connecting to runtime stream...</span>
		</div>
	{/if}

	<!-- Log Display -->
	<div class="log-container" bind:this={logContainerRef} data-testid="log-container">
		{#if filteredLogs.length === 0}
			<div class="empty-state">
				{#if logsExcludingMessages.length === 0}
					<p>No logs yet. Waiting for runtime events...</p>
				{:else}
					<p>No logs match current filters.</p>
				{/if}
			</div>
		{:else}
			<div class="log-entries" role="log" aria-live="polite" aria-atomic="false">
				{#each filteredLogs as log (log.id)}
					<div class="log-entry" data-level={log.level} data-testid="log-entry">
						<span class="log-timestamp">{formatTimestamp(log.timestamp)}</span>
						<span class="log-level" style="color: {getLevelColor(log.level)}">{log.level}</span>
						<span class="log-component">[{log.source}]</span>
						<span class="log-message">{log.message}</span>
					</div>
				{/each}
			</div>
		{/if}
	</div>
</div>

<style>
	.logs-tab {
		display: flex;
		flex-direction: column;
		height: 100%;
		background: var(--ui-surface-primary);
	}

	/* Filter Bar */
	.filter-bar {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 0.75rem 1rem;
		border-bottom: 1px solid var(--ui-border-subtle);
		background: var(--ui-surface-secondary);
		gap: 1rem;
		flex-wrap: wrap;
	}

	.filter-controls {
		display: flex;
		gap: 1rem;
		align-items: center;
		flex-wrap: wrap;
	}

	.filter-controls label {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.875rem;
	}

	.filter-label {
		color: var(--ui-text-secondary);
		font-weight: 500;
	}

	.filter-controls select {
		padding: 0.375rem 0.5rem;
		border: 1px solid var(--ui-border-subtle);
		border-radius: 4px;
		background: var(--ui-surface-primary);
		color: var(--ui-text-primary);
		font-size: 0.875rem;
		cursor: pointer;
		transition: all 0.2s;
	}

	.filter-controls select:hover {
		border-color: var(--ui-border-interactive);
	}

	.filter-controls select:focus {
		outline: none;
		border-color: var(--ui-focus-ring);
		box-shadow: 0 0 0 2px rgba(15, 98, 254, 0.1);
	}

	.clear-button {
		padding: 0.375rem 0.75rem;
		border: 1px solid var(--ui-border-subtle);
		border-radius: 4px;
		background: var(--ui-surface-primary);
		color: var(--ui-text-secondary);
		font-size: 0.875rem;
		cursor: pointer;
		transition: all 0.2s;
		font-weight: 500;
	}

	.clear-button:hover {
		background: var(--ui-surface-tertiary);
		color: var(--ui-text-primary);
		border-color: var(--ui-border-strong);
	}

	.auto-scroll-control {
		display: flex;
		align-items: center;
	}

	.auto-scroll-control label {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.875rem;
		color: var(--ui-text-secondary);
		cursor: pointer;
	}

	.auto-scroll-control input[type='checkbox'] {
		cursor: pointer;
	}

	/* Connection Status */
	.connection-status {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.5rem 1rem;
		font-size: 0.875rem;
		border-bottom: 1px solid var(--ui-border-subtle);
	}

	.connection-status.error {
		background: var(--status-error-container);
		color: var(--status-error-on-container);
	}

	.connection-status.connecting {
		background: var(--status-info-container);
		color: var(--status-info-on-container);
	}

	.status-icon {
		font-size: 1rem;
	}

	/* Log Container */
	.log-container {
		flex: 1;
		overflow-y: auto;
		overflow-x: hidden;
		padding: 0.5rem 1rem;
		background: var(--ui-surface-primary);
	}

	.empty-state {
		display: flex;
		align-items: center;
		justify-content: center;
		height: 100%;
		min-height: 150px;
	}

	.empty-state p {
		margin: 0;
		color: var(--ui-text-secondary);
		font-size: 0.875rem;
	}

	/* Log Entries */
	.log-entries {
		font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', 'Consolas', monospace;
		font-size: 0.75rem;
		line-height: 1.5;
	}

	.log-entry {
		display: grid;
		grid-template-columns: auto auto auto 1fr;
		gap: 0.75rem;
		padding: 0.25rem 0;
		border-bottom: 1px solid transparent;
		transition: background-color 0.1s;
	}

	.log-entry:hover {
		background: var(--ui-surface-secondary);
		border-bottom-color: var(--ui-border-subtle);
	}

	.log-timestamp {
		color: var(--ui-text-tertiary);
		font-weight: 500;
		white-space: nowrap;
	}

	.log-level {
		font-weight: 700;
		text-align: left;
		min-width: 4rem;
		white-space: nowrap;
	}

	.log-component {
		color: var(--ui-text-secondary);
		font-weight: 500;
		white-space: nowrap;
	}

	.log-message {
		color: var(--ui-text-primary);
		white-space: pre-wrap;
		word-break: break-word;
	}

	/* Log level specific styles */
	.log-entry[data-level='ERROR'] {
		background: var(--status-error-container);
	}

	.log-entry[data-level='WARN'] {
		background: var(--status-warning-container);
	}

	/* Scrollbar styling (optional, for better UX) */
	.log-container::-webkit-scrollbar {
		width: 8px;
	}

	.log-container::-webkit-scrollbar-track {
		background: var(--ui-surface-secondary);
	}

	.log-container::-webkit-scrollbar-thumb {
		background: var(--ui-border-strong);
		border-radius: 4px;
	}

	.log-container::-webkit-scrollbar-thumb:hover {
		background: var(--ui-interactive-secondary);
	}
</style>
