<script lang="ts">
	import { SvelteSet } from 'svelte/reactivity';
	/**
	 * MessagesTab Component - NATS message flow visualization
	 * Uses runtimeStore logs filtered by source="message-logger"
	 *
	 * Features:
	 * - Display NATS message traffic from filtered log entries
	 * - Filter by direction (published, received, processed)
	 * - Expandable metadata for each message
	 * - Auto-scroll toggle
	 * - Clear messages functionality (clears store logs)
	 * - Color-coded direction indicators using design system
	 * - Monospace font for NATS subjects
	 * - Millisecond precision timestamps
	 */

	import {
		runtimeStore,
		type LogEntry
	} from '$lib/stores/runtimeStore.svelte';
	import {
		messagesApi,
		type RuntimeMessage,
		MessagesApiError
	} from '$lib/services/messagesApi';

	interface MessagesTabProps {
		flowId: string;
		isActive: boolean;
	}

	// Props passed from parent - may be used for future tab-specific logic
	let { flowId: _flowId, isActive: _isActive }: MessagesTabProps = $props();

	// Local UI state
	let directionFilter = $state<'all' | 'published' | 'received' | 'processed'>('all');
	let traceFilter = $state<string>('');
	let autoScroll = $state<boolean>(true);
	let expandedMessageIds = new SvelteSet<string>();
	let copiedTraceId = $state<string | null>(null);

	// Historical messages state
	let historicalMessages = $state<RuntimeMessage[]>([]);
	let loadingHistory = $state<boolean>(false);
	let historyError = $state<string | null>(null);

	// Refs for DOM elements
	let messagesContainerRef: HTMLDivElement | null = null;

	// Check if message-logger service is available in health components
	// Service name is just 'message-logger' (not 'message-logger-service')
	const messageLoggerAvailable = $derived(
		runtimeStore.healthComponents.some((c) => c.name === 'message-logger')
	);

	// Filter logs to only message-logger entries
	const messageLoggerLogs = $derived(
		runtimeStore.logs.filter((log) => log.source === 'message-logger')
	);

	// Convert RuntimeMessage to MessageInfo
	function convertRuntimeMessageToInfo(msg: RuntimeMessage): MessageInfo {
		return {
			id: msg.message_id,
			timestamp: msg.timestamp,
			subject: msg.subject,
			direction: msg.direction,
			summary: `${msg.direction} message`,
			component: msg.component,
			traceId: msg.message_id,
			metadata: Object.keys(msg)
				.filter((k) => !['message_id', 'timestamp', 'subject', 'direction', 'component'].includes(k))
				.reduce((acc, key) => {
					acc[key] = msg[key];
					return acc;
				}, {} as Record<string, unknown>)
		};
	}

	// Merge historical and live messages, deduplicate cross-source only
	const allMessages = $derived.by(() => {
		const liveMessages: MessageInfo[] = [];
		for (const log of messageLoggerLogs) {
			const info = extractMessageInfo(log);
			if (info) {
				liveMessages.push(info);
			}
		}

		const historical = historicalMessages.map(convertRuntimeMessageToInfo);

		// Build set of live message keys (traceId + timestamp)
		const liveKeys = new SvelteSet<string>();
		for (const msg of liveMessages) {
			if (msg.traceId) {
				liveKeys.add(`${msg.traceId}-${msg.timestamp}`);
			}
		}

		// Include all live messages (they have unique log IDs)
		const merged: MessageInfo[] = [...liveMessages];

		// Add historical messages that don't overlap with live
		for (const msg of historical) {
			const key = msg.traceId ? `${msg.traceId}-${msg.timestamp}` : msg.id;
			if (!liveKeys.has(key)) {
				merged.push(msg);
			}
		}

		// Sort by timestamp (oldest first)
		merged.sort((a, b) => a.timestamp - b.timestamp);

		return merged;
	});

	// Extract message info from log fields
	interface MessageInfo {
		id: string;
		timestamp: number;
		subject: string;
		direction: 'published' | 'received' | 'processed';
		summary: string;
		component: string;
		traceId: string | null;
		metadata?: Record<string, unknown>;
	}

	function extractMessageInfo(log: LogEntry): MessageInfo | null {
		const fields = log.fields || {};
		const subject = (fields.subject as string) || 'unknown';
		const direction = (fields.direction as 'published' | 'received' | 'processed') || 'processed';
		const component = (fields.component as string) || log.source;
		const traceId = (fields.message_id as string) || (fields.trace_id as string) || null;

		return {
			id: log.id,
			timestamp: log.timestamp,
			subject,
			direction,
			summary: log.message,
			component,
			traceId,
			metadata: Object.keys(fields).length > 0 ? fields : undefined
		};
	}

	// Convert logs to messages and apply filters
	const filteredMessages = $derived.by(() => {
		const messages: MessageInfo[] = [];
		for (const msg of allMessages) {
			// Apply direction filter
			if (directionFilter !== 'all' && msg.direction !== directionFilter) {
				continue;
			}

			// Apply trace ID filter
			if (traceFilter) {
				// Skip messages without trace IDs when filtering
				if (!msg.traceId) {
					continue;
				}
				// Case-insensitive partial match
				if (!msg.traceId.toLowerCase().includes(traceFilter.toLowerCase())) {
					continue;
				}
			}

			messages.push(msg);
		}
		return messages;
	});

	/**
	 * Format timestamp for display (HH:MM:SS.mmm)
	 */
	function formatTime(unixMs: number): string {
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
	 * Truncate trace ID for display
	 */
	function truncateTraceId(id: string): string {
		return id.length > 8 ? `${id.substring(0, 8)}...` : id;
	}

	/**
	 * Get direction icon
	 */
	function getDirectionIcon(direction: MessageInfo['direction']): string {
		const icons = {
			published: '→',
			received: '←',
			processed: '⟳'
		};
		return icons[direction];
	}

	/**
	 * Get direction color from design system
	 */
	function getDirectionColor(direction: MessageInfo['direction']): string {
		const colors = {
			published: 'var(--status-info)',
			received: 'var(--status-success)',
			processed: 'var(--ui-text-secondary)'
		};
		return colors[direction];
	}

	/**
	 * Toggle metadata visibility for a message
	 */
	function toggleMetadata(messageId: string) {
		if (expandedMessageIds.has(messageId)) {
			expandedMessageIds.delete(messageId);
		} else {
			expandedMessageIds.add(messageId);
		}
	}

	/**
	 * Clear all messages (clears store logs, NOT historical messages)
	 */
	function clearMessages() {
		runtimeStore.clearLogs();
		directionFilter = 'all';
		traceFilter = '';
		expandedMessageIds.clear();
	}

	/**
	 * Load historical messages from API
	 */
	async function loadHistory() {
		loadingHistory = true;
		historyError = null;
		try {
			const response = await messagesApi.fetchMessages(_flowId, { limit: 100 });

			// Deduplicate new messages with existing historical messages
			const existingKeys = new Set(
				historicalMessages.map((m) => `${m.message_id}-${m.timestamp}`)
			);
			const newMessages = response.messages.filter(
				(m) => !existingKeys.has(`${m.message_id}-${m.timestamp}`)
			);

			historicalMessages = [...historicalMessages, ...newMessages];
		} catch (error) {
			// Show specific error from MessagesApiError, generic message otherwise
			if (error instanceof MessagesApiError) {
				historyError = error.message;
			} else {
				historyError = 'Failed to load history';
			}
		} finally {
			loadingHistory = false;
		}
	}

	/**
	 * Set trace filter (from clicking trace ID)
	 */
	function setTraceFilter(traceId: string) {
		traceFilter = traceId;
	}

	/**
	 * Clear trace filter
	 */
	function clearTraceFilter() {
		traceFilter = '';
	}

	/**
	 * Copy trace ID to clipboard
	 */
	async function copyTraceId(traceId: string) {
		try {
			await navigator.clipboard.writeText(traceId);
			copiedTraceId = traceId;
			setTimeout(() => {
				copiedTraceId = null;
			}, 2000);
		} catch (error) {
			console.error('Failed to copy trace ID:', error);
		}
	}

	// Effect: Auto-scroll when filtered messages change
	$effect(() => {
		const messages = filteredMessages;

		if (autoScroll && messagesContainerRef && messages.length > 0) {
			requestAnimationFrame(() => {
				if (messagesContainerRef) {
					messagesContainerRef.scrollTop = messagesContainerRef.scrollHeight;
				}
			});
		}
	});
</script>

<div class="messages-tab" data-testid="messages-tab">
	<!-- Control Bar -->
	<div class="control-bar">
		<div class="filter-controls">
			<label for="direction-filter">
				<span class="filter-label">Direction:</span>
				<select
					id="direction-filter"
					bind:value={directionFilter}
					data-testid="direction-filter"
					aria-label="Filter by direction"
				>
					<option value="all">All</option>
					<option value="published">Published</option>
					<option value="received">Received</option>
					<option value="processed">Processed</option>
				</select>
			</label>

			<label for="trace-id-search">
				<span class="filter-label">Trace ID:</span>
				<input
					type="text"
					id="trace-id-search"
					bind:value={traceFilter}
					data-testid="trace-id-search"
					placeholder="Filter by trace ID..."
				/>
			</label>

			{#if traceFilter}
				<div class="trace-filter-badge" data-testid="trace-filter-badge">
					<span>Filtered: {traceFilter.length > 20 ? traceFilter.substring(0, 8) + '...' : traceFilter}</span>
					<button
						class="clear-trace-filter-button"
						onclick={clearTraceFilter}
						data-testid="clear-trace-filter-button"
						aria-label="Clear trace filter"
					>
						✕
					</button>
				</div>
			{/if}

			<button
				class="clear-button"
				onclick={clearMessages}
				data-testid="clear-messages-button"
				aria-label="Clear all messages"
			>
				Clear
			</button>

			{#if runtimeStore.connected && messageLoggerAvailable}
				<button
					class="load-history-button"
					onclick={loadHistory}
					disabled={loadingHistory}
					data-testid="load-history-button"
				>
					Load History
				</button>
			{/if}
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
		<div class="error-message" role="alert">
			<span class="error-icon">⚠</span>
			<span>{runtimeStore.error}</span>
		</div>
	{:else if !runtimeStore.connected}
		<div class="connecting-message">
			<span class="connecting-icon">⋯</span>
			<span>Connecting to runtime stream...</span>
		</div>
	{/if}

	<!-- History Loading Indicator -->
	{#if loadingHistory}
		<div class="history-loading" data-testid="history-loading">
			<span class="loading-icon">⋯</span>
			<span>Loading historical messages...</span>
		</div>
	{/if}

	<!-- History Error -->
	{#if historyError}
		<div class="history-error" role="alert" data-testid="history-error">
			<span class="error-icon">⚠</span>
			<span>{historyError}</span>
		</div>
	{/if}

	<!-- Messages Container -->
	<div class="messages-container" bind:this={messagesContainerRef} data-testid="messages-container">
		{#if !messageLoggerAvailable}
			<div class="service-unavailable" data-testid="service-unavailable">
				<p class="unavailable-title">Message Logger service not enabled</p>
				<p class="unavailable-hint">
					Enable the message-logger service in your backend configuration to capture NATS messages.
				</p>
			</div>
		{:else if filteredMessages.length === 0}
			<div class="empty-state">
				{#if allMessages.length === 0}
					<p>No messages yet. Waiting for NATS traffic...</p>
				{:else}
					<p>No messages match current filters.</p>
				{/if}
			</div>
		{:else}
			<div class="message-entries" role="log" aria-live="polite" aria-atomic="false">
				{#each filteredMessages as message (message.id)}
					<div class="message-entry" data-testid="message-entry">
						<span class="timestamp">{formatTime(message.timestamp)}</span>
						<span
							class="direction"
							class:published={message.direction === 'published'}
							class:received={message.direction === 'received'}
							class:processed={message.direction === 'processed'}
							style="color: {getDirectionColor(message.direction)}"
							aria-label={message.direction}
						>
							{getDirectionIcon(message.direction)}
						</span>
						<span class="component">[{message.component}]</span>
						<span class="subject">{message.subject}</span>
						{#if message.traceId}
							<div class="trace-id-container">
								<button
									class="trace-id"
									onclick={() => setTraceFilter(message.traceId!)}
									tabindex={0}
								>
									{truncateTraceId(message.traceId)}
								</button>
								<button
									class="copy-trace-button"
									onclick={() => copyTraceId(message.traceId!)}
									aria-label="Copy trace ID"
								>
									{copiedTraceId === message.traceId ? 'Copied!' : '📋'}
								</button>
							</div>
						{/if}
						<span class="summary">{message.summary}</span>
						{#if message.metadata}
							<button
								class="metadata-toggle"
								onclick={() => toggleMetadata(message.id)}
								aria-expanded={expandedMessageIds.has(message.id)}
								aria-label="Toggle metadata"
							>
								{expandedMessageIds.has(message.id) ? '▼' : '▶'}
							</button>
						{/if}
					</div>
					{#if expandedMessageIds.has(message.id) && message.metadata}
						<div class="metadata">
							<pre>{JSON.stringify(message.metadata, null, 2)}</pre>
						</div>
					{/if}
				{/each}
			</div>
		{/if}
	</div>
</div>

<style>
	.messages-tab {
		display: flex;
		flex-direction: column;
		height: 100%;
		background: var(--ui-surface-primary);
	}

	/* Control Bar */
	.control-bar {
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

	.filter-controls select,
	.filter-controls input[type='text'] {
		padding: 0.375rem 0.5rem;
		border: 1px solid var(--ui-border-subtle);
		border-radius: 4px;
		background: var(--ui-surface-primary);
		color: var(--ui-text-primary);
		font-size: 0.875rem;
		cursor: pointer;
		transition: all 0.2s;
	}

	.filter-controls input[type='text'] {
		min-width: 200px;
	}

	.filter-controls select:hover,
	.filter-controls input[type='text']:hover {
		border-color: var(--ui-border-interactive);
	}

	.filter-controls select:focus,
	.filter-controls input[type='text']:focus {
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

	.clear-button:active {
		transform: scale(0.98);
	}

	.load-history-button {
		padding: 0.375rem 0.75rem;
		border: 1px solid var(--ui-border-subtle);
		border-radius: 4px;
		background: var(--ui-interactive-primary);
		color: var(--ui-on-interactive-primary);
		font-size: 0.875rem;
		cursor: pointer;
		transition: all 0.2s;
		font-weight: 500;
	}

	.load-history-button:hover:not(:disabled) {
		background: var(--ui-interactive-primary-hover);
	}

	.load-history-button:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.load-history-button:active:not(:disabled) {
		transform: scale(0.98);
	}

	.trace-filter-badge {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.375rem 0.75rem;
		background: var(--status-info-container);
		color: var(--status-info-on-container);
		border-radius: 4px;
		font-size: 0.875rem;
		font-weight: 500;
	}

	.clear-trace-filter-button {
		background: none;
		border: none;
		color: var(--status-info-on-container);
		cursor: pointer;
		padding: 0 0.25rem;
		font-size: 1rem;
		font-weight: 700;
		transition: opacity 0.2s;
	}

	.clear-trace-filter-button:hover {
		opacity: 0.7;
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

	/* Error/Connecting Messages */
	.error-message,
	.connecting-message {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.5rem 1rem;
		font-size: 0.875rem;
		border-bottom: 1px solid var(--ui-border-subtle);
	}

	.error-message {
		background: var(--status-error-container);
		color: var(--status-error-on-container);
	}

	.connecting-message {
		background: var(--status-info-container);
		color: var(--status-info-on-container);
	}

	.history-loading {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.5rem 1rem;
		font-size: 0.875rem;
		border-bottom: 1px solid var(--ui-border-subtle);
		background: var(--status-info-container);
		color: var(--status-info-on-container);
	}

	.history-error {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.5rem 1rem;
		font-size: 0.875rem;
		border-bottom: 1px solid var(--ui-border-subtle);
		background: var(--status-error-container);
		color: var(--status-error-on-container);
	}

	.loading-icon {
		font-size: 1rem;
	}

	.error-icon,
	.connecting-icon {
		font-size: 1rem;
	}

	/* Messages Container */
	.messages-container {
		flex: 1;
		overflow-y: auto;
		overflow-x: hidden;
		padding: 0.5rem 1rem;
		background: var(--ui-surface-primary);
	}

	.empty-state,
	.service-unavailable {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		height: 100%;
		min-height: 150px;
		gap: 0.5rem;
	}

	.empty-state p {
		margin: 0;
		color: var(--ui-text-secondary);
		font-size: 0.875rem;
	}

	.service-unavailable {
		text-align: center;
		padding: 2rem;
	}

	.unavailable-title {
		margin: 0;
		color: var(--ui-text-secondary);
		font-size: 1rem;
		font-weight: 600;
	}

	.unavailable-hint {
		margin: 0;
		color: var(--ui-text-tertiary);
		font-size: 0.875rem;
		max-width: 400px;
	}

	/* Message Entries */
	.message-entries {
		font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', 'Consolas', monospace;
		font-size: 0.75rem;
		line-height: 1.5;
	}

	.message-entry {
		display: grid;
		grid-template-columns: auto auto auto auto auto 1fr auto;
		gap: 0.75rem;
		padding: 0.25rem 0;
		border-bottom: 1px solid transparent;
		transition: background-color 0.1s;
		align-items: center;
	}

	.message-entry:hover {
		background: var(--ui-surface-secondary);
		border-bottom-color: var(--ui-border-subtle);
	}

	.timestamp {
		color: var(--ui-text-tertiary);
		font-weight: 500;
		white-space: nowrap;
	}

	.direction {
		font-size: 1rem;
		font-weight: 700;
		white-space: nowrap;
		min-width: 1.5rem;
		text-align: center;
	}

	.component {
		color: var(--ui-text-secondary);
		font-weight: 500;
		white-space: nowrap;
	}

	.subject {
		font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', 'Consolas', monospace;
		color: var(--ui-text-primary);
		font-weight: 600;
		white-space: nowrap;
	}

	.summary {
		color: var(--ui-text-primary);
		white-space: pre-wrap;
		word-break: break-word;
	}

	.metadata-toggle {
		background: none;
		border: none;
		color: var(--ui-text-secondary);
		cursor: pointer;
		padding: 0.25rem;
		font-size: 0.75rem;
		transition: color 0.2s;
	}

	.metadata-toggle:hover {
		color: var(--ui-text-primary);
	}

	.trace-id-container {
		display: flex;
		align-items: center;
		gap: 0.25rem;
	}

	.trace-id {
		font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', 'Consolas', monospace;
		font-size: 0.75rem;
		color: var(--ui-text-secondary);
		background: var(--ui-surface-secondary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: 3px;
		padding: 0.125rem 0.375rem;
		cursor: pointer;
		transition: all 0.2s;
		white-space: nowrap;
	}

	.trace-id:hover {
		color: var(--ui-text-primary);
		border-color: var(--ui-border-interactive);
		background: var(--ui-surface-tertiary);
	}

	.copy-trace-button {
		background: none;
		border: none;
		color: var(--ui-text-secondary);
		cursor: pointer;
		padding: 0.125rem 0.25rem;
		font-size: 0.875rem;
		transition: color 0.2s;
	}

	.copy-trace-button:hover {
		color: var(--ui-text-primary);
	}

	/* Metadata Display */
	.metadata {
		background: var(--ui-surface-secondary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: 4px;
		padding: 0.75rem;
		margin: 0.25rem 0 0.5rem 0;
	}

	.metadata pre {
		margin: 0;
		font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', 'Consolas', monospace;
		font-size: 0.75rem;
		color: var(--ui-text-primary);
		white-space: pre-wrap;
		word-break: break-word;
	}

	/* Scrollbar styling (optional, for better UX) */
	.messages-container::-webkit-scrollbar {
		width: 8px;
	}

	.messages-container::-webkit-scrollbar-track {
		background: var(--ui-surface-secondary);
	}

	.messages-container::-webkit-scrollbar-thumb {
		background: var(--ui-border-strong);
		border-radius: 4px;
	}

	.messages-container::-webkit-scrollbar-thumb:hover {
		background: var(--ui-interactive-secondary);
	}
</style>
