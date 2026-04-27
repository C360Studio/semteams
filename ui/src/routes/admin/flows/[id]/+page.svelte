<script lang="ts">
	import FlowCanvas from '$lib/components/FlowCanvas.svelte';
	import ThreePanelLayout from '$lib/components/layout/ThreePanelLayout.svelte';
	import ChatPanel from '$lib/components/chat/ChatPanel.svelte';
	import PropertiesPanel from '$lib/components/PropertiesPanel.svelte';
	import StatusBar from '$lib/components/StatusBar.svelte';
	import RuntimePanel from '$lib/components/RuntimePanel.svelte';
	import SaveStatusIndicator from '$lib/components/SaveStatusIndicator.svelte';
	import NavigationGuard from '$lib/components/NavigationGuard.svelte';
	import NavigationDialog from '$lib/components/NavigationDialog.svelte';
	import ValidationErrorDialog from '$lib/components/ValidationErrorDialog.svelte';
	import DeployErrorModal from '$lib/components/DeployErrorModal.svelte';
	import ValidationStatusModal from '$lib/components/ValidationStatusModal.svelte';
	import ViewSwitcher from '$lib/components/ViewSwitcher.svelte';
	import DataView from '$lib/components/DataView.svelte';
	import { chatStore } from '$lib/stores/chatStore.svelte';
	import { streamChat } from '$lib/services/chatApi';
	import type { PageData } from './$types';
	import type { ComponentInstance, FlowNode, FlowConnection } from '$lib/types/flow';
	import type { SaveState, RuntimeStateInfo, PropertiesPanelMode, ViewMode } from '$lib/types/ui-state';
	import type { ValidationResult as PortValidationResult, ValidatedPort } from '$lib/types/port';
	import type { ComponentType } from '$lib/types/component';
	import { saveFlow, deployFlow, startFlow, stopFlow, isValidationError } from '$lib/api/flows';
	import { flowHistory } from '$lib/stores/flowHistory.svelte';
	import { createPanelLayoutStore } from '$lib/stores/panelLayoutStore.svelte';
	import { runtimeWS } from '$lib/services/runtimeWebSocket';
	import { runtimeStore } from '$lib/stores/runtimeStore.svelte';
	import { onMount, onDestroy } from 'svelte';

	let { data }: { data: PageData } = $props();

	// Backend flow (domain model) - source of truth for persistence
	let backendFlow = $state(data.flow);

	// UI state
	let dirty = $state(false);
	let selectedComponent = $state<ComponentInstance | null>(null);

	// Component types for add modal (fetched from backend)
	let componentTypes = $state<ComponentType[]>([]);

	// Panel layout store
	const panelLayout = createPanelLayoutStore();

	// Properties panel state
	const propertiesPanelMode = $derived.by((): PropertiesPanelMode => {
		if (selectedComponent) return 'edit';
		return 'empty';
	});

	// Get component type for the selected node
	const selectedNodeComponentType = $derived.by(() => {
		const selected = selectedComponent;
		if (!selected) return null;
		return componentTypes.find((ct) => ct.id === selected.component) ?? null;
	});

	// Navigation dialog state
	let showNavigationDialog = $state(false);
	let navigationGuard: NavigationGuard;
	let shouldNavigateAfterSave = $state(false);

	// Validation error dialog state
	let showValidationDialog = $state(false);
	let deployValidationResult = $state<any>(null);

	// Deploy error modal state (Gate 3: Deploy-Time Validation)
	let showDeployErrorModal = $state(false);

	// Validation status modal state (Feature 015 - T014)
	let showValidationStatusModal = $state(false);

	// Real-time validation state (for port visualization)
	let validationResult = $state<PortValidationResult | null>(null);

	// Compute if flow is valid for deployment
	const isFlowValid = $derived(validationResult?.validation_status !== 'errors');

	// Deploy sequencing state
	let pendingDeploy = $state(false);

	// Save state tracking
	let saveState = $state<SaveState>({
		status: 'clean',
		lastSaved: null,
		error: null
	});

	// Runtime state tracking
	let runtimeState = $state<RuntimeStateInfo>({
		state: data.flow.runtime_state,
		message: null,
		lastTransition: null
	});

	// Runtime panel state
	let showRuntimePanel = $state(false);
	let runtimePanelHeight = $state(300);

	// View mode: flow editor or data visualization
	// Only allow data view when flow is running
	const isFlowRunning = $derived(runtimeState.state === 'running');

	// Reset to flow view when flow stops running
	$effect(() => {
		if (!isFlowRunning && panelLayout.state.viewMode === 'data') {
			panelLayout.setViewMode('flow');
		}
	});

	function handleViewModeChange(mode: ViewMode) {
		panelLayout.setViewMode(mode);
	}

	// Flow state - work directly with domain model
	let flowNodes = $state<FlowNode[]>(backendFlow.nodes);
	let flowConnections = $state<FlowConnection[]>(backendFlow.connections);

	// Port information from validation results
	type PortsMap = Record<string, { input_ports: ValidatedPort[]; output_ports: ValidatedPort[] }>;
	let portsMap = $state<PortsMap>({});


	// Fetch component types and set up viewport handling on mount
	onMount(async () => {
		// Initialize responsive layout based on current viewport
		panelLayout.handleViewportResize(window.innerWidth);

		// Connect to WebSocket for runtime data and subscribe to all message types
		runtimeWS.connect(backendFlow.id);
		runtimeWS.subscribe({
			messageTypes: ['flow_status', 'component_health', 'component_metrics', 'log_entry']
		});

		// Fetch component types
		try {
			const response = await fetch('/components/types');
			if (response.ok) {
				componentTypes = await response.json();
			}
		} catch (error) {
			console.error('Failed to fetch component types:', error);
		}
	});

	// Cleanup WebSocket connection and reset store on unmount
	onDestroy(() => {
		runtimeWS.disconnect();
		runtimeStore.reset();
	});

	// Handle viewport resize for responsive panel behavior
	function handleWindowResize() {
		panelLayout.handleViewportResize(window.innerWidth);
	}

	// Handle keyboard shortcuts for deselection
	function handleKeyDown(event: KeyboardEvent) {
		// Escape: Deselect component
		if (event.key === 'Escape' && selectedComponent) {
			selectedComponent = null;
		}
	}

	// Canvas event handlers
	function handleNodeClick(nodeId: string) {
		const flowNode = flowNodes.find((n) => n.id === nodeId);
		if (flowNode) {
			selectedComponent = {
				...flowNode,
				health: {
					status: 'not_running',
					lastUpdated: new Date().toISOString()
				}
			};
		}
	}

	// Debounced validation on canvas changes (500ms delay)
	let validationTimer: ReturnType<typeof setTimeout> | null = null;
	let lastValidatedSignature = $state('');

	$effect(() => {
		// Create signature from things that represent USER changes only
		// Ignore auto connections (they're generated by validation)
		const nodeIds = flowNodes.map(n => n.id).sort().join(',');
		const manualConnectionIds = flowConnections
			.filter(c => c.id.startsWith('conn_'))  // Only manual connections
			.map(c => c.id)
			.sort()
			.join(',');
		const signature = `${nodeIds}|${manualConnectionIds}`;

		// Skip if nothing changed since last validation
		// This prevents validation from triggering itself when it adds/removes auto connections
		if (signature === lastValidatedSignature) {
			return;
		}

		// Clear existing timer
		if (validationTimer) {
			clearTimeout(validationTimer);
		}

		// Schedule validation after 500ms of inactivity
		validationTimer = setTimeout(async () => {
			const result = await runFlowValidation(backendFlow.id);
			validationResult = result;

			if (result) {
				// Apply validation (populates portsMap and updates auto connections)
				applyValidationToNodes(result);
				updateAutoConnections(result);

				// Update signature to match current state after validation
				// When effect re-runs, it will see signature matches and skip
				const nodeIds = flowNodes.map(n => n.id).sort().join(',');
				const manualConnectionIds = flowConnections
					.filter(c => c.id.startsWith('conn_'))
					.map(c => c.id)
					.sort()
					.join(',');
				lastValidatedSignature = `${nodeIds}|${manualConnectionIds}`;

				// Update save state based on validation
				if (result.validation_status === 'errors' && !dirty && saveState.status === 'clean') {
					saveState = {
						status: 'draft',
						lastSaved: saveState.lastSaved,
						error: `${result.errors.length} error${result.errors.length > 1 ? 's' : ''}`,
						validationResult: result
					};
				} else if (result.validation_status !== 'errors' && saveState.status === 'draft') {
					saveState = {
						status: 'clean',
						lastSaved: saveState.lastSaved,
						error: null,
						validationResult: result
					};
				} else {
					saveState = { ...saveState, validationResult: result };
				}
			}
		}, 500);

		// Cleanup
		return () => {
			if (validationTimer) {
				clearTimeout(validationTimer);
			}
		};
	});

	/**
	 * Run flow validation via backend API
	 * Calls the real validation endpoint that returns port information
	 * Sends current flow state (may be unsaved) for real-time validation
	 */
	async function runFlowValidation(flowId: string): Promise<PortValidationResult | null> {
		try {
			const flowDefinition = {
				id: flowId,
				name: backendFlow.name,
				runtime_state: backendFlow.runtime_state,
				nodes: flowNodes,
				connections: flowConnections
			};

			console.log('[runFlowValidation] Sending node IDs to validation:', flowNodes.map(n => n.id));

			const response = await fetch(`/flowbuilder/flows/${flowId}/validate`, {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json'
				},
				body: JSON.stringify(flowDefinition)
			});

			if (!response.ok) {
				console.error('Validation request failed:', response.status, response.statusText);
				return null;
			}

			const result = await response.json();
			return result;
		} catch (error) {
			console.error('Validation failed:', error);
			return null;
		}
	}

	/**
	 * Update auto-discovered connections from validation results
	 * Removes old auto connections and creates new ones from FlowGraph pattern matching
	 */
	function updateAutoConnections(result: PortValidationResult) {
		// Step 1: Remove old auto-discovered connections by filtering
		flowConnections = flowConnections.filter(conn => !conn.id.startsWith('auto_'));

		// Step 2: Create new auto-discovered connections from validation result
		// Safety check: discovered_connections might be undefined or null
		if (!result.discovered_connections || result.discovered_connections.length === 0) {
			return;
		}

		const newAutoConnections = result.discovered_connections.map((conn) => {
			const connectionId = `auto_${conn.source_node_id}_${conn.source_port}_${conn.target_node_id}_${conn.target_port}`;

			const flowConnection: FlowConnection = {
				id: connectionId,
				source_node_id: conn.source_node_id,
				source_port: conn.source_port,
				target_node_id: conn.target_node_id,
				target_port: conn.target_port,
				source: 'auto',
				validationState: 'valid'
			};

			console.log('[updateAutoConnections] Created connection:', JSON.stringify(flowConnection, null, 2));
			return flowConnection;
		});

		// Step 3: Add new auto-discovered connections
		console.log('[updateAutoConnections] Adding', newAutoConnections.length, 'auto connections');
		flowConnections = [...flowConnections, ...newAutoConnections];
		console.log('[updateAutoConnections] Total connections after update:', flowConnections.length);
	}

	/**
	 * Apply validation results to populate portsMap
	 * Updates port information from backend validation for visualization
	 */
	function applyValidationToNodes(result: PortValidationResult) {
		// Build portsMap from validation result
		if (result.nodes && result.nodes.length > 0) {
			const newPortsMap: PortsMap = {};

			for (const validatedNode of result.nodes) {
				newPortsMap[validatedNode.id] = {
					input_ports: validatedNode.input_ports,
					output_ports: validatedNode.output_ports
				};
			}

			portsMap = newPortsMap;
		}

		// Mark connections with errors
		if (result.errors.length > 0) {
			flowConnections = flowConnections.map(conn => {
				const hasError = result.errors.some(
					(err) =>
						err.component_name === conn.source_node_id ||
						err.component_name === conn.target_node_id
				);

				if (hasError) {
					return {
						...conn,
						validationState: 'error' as const
					};
				}

				return conn;
			});
		}
	}

	function handleDeleteNode(nodeId: string) {
		flowNodes = flowNodes.filter((n) => n.id !== nodeId);
		// Also remove connections involving this node
		flowConnections = flowConnections.filter(
			(c) => c.source_node_id !== nodeId && c.target_node_id !== nodeId
		);
		// Clear selection if deleted node was selected
		if (selectedComponent?.id === nodeId) {
			selectedComponent = null;
		}
		dirty = true;
		saveState = { ...saveState, status: 'dirty' };
	}


	// PropertiesPanel handlers
	function handlePropertiesSave(nodeId: string, name: string, config: Record<string, unknown>) {
		flowNodes = flowNodes.map((node) =>
			node.id === nodeId ? { ...node, name, config } : node
		);
		dirty = true;
		saveState = { ...saveState, status: 'dirty' };

		// Update selected component if it was being edited
		if (selectedComponent?.id === nodeId) {
			const updated = flowNodes.find((n) => n.id === nodeId);
			if (updated) {
				selectedComponent = {
					...updated,
					health: selectedComponent.health
				};
			}
		}
	}

	function handlePropertiesDelete(nodeId: string) {
		handleDeleteNode(nodeId);
	}

	// Panel layout handlers
	function handleLeftWidthChange(width: number) {
		panelLayout.setLeftWidth(width);
	}

	function handleRightWidthChange(width: number) {
		panelLayout.setRightWidth(width);
	}

	function handleToggleLeftPanel() {
		panelLayout.toggleLeft();
	}

	function handleToggleRightPanel() {
		panelLayout.toggleRight();
	}


	// Chat handlers
	let chatAbortController = $state<AbortController | null>(null);

	function handleChatSubmit(content: string) {
		chatStore.setError(null);
		chatStore.addUserMessage(content);
		chatStore.setStreaming(true);

		chatAbortController = new AbortController();

		streamChat(
			{
				messages: chatStore.messages
					.filter((m) => m.role !== 'system')
					.map((m) => ({ role: m.role as 'user' | 'assistant', content: m.content })),
				// Updated to use new context shape instead of currentFlow
				context: {
					page: 'flow-builder',
					flowId: backendFlow.id,
					flowName: backendFlow.name,
					nodes: flowNodes,
					connections: flowConnections
				},
				chips: chatStore.chips
			},
			{
				onText: (chunk: string) => {
					chatStore.appendStreamContent(chunk);
				},
				onDone: ({ attachments }) => {
					chatStore.finalizeStream(chatStore.streamingContent, attachments);
					chatAbortController = null;
				},
				onError: (errorMsg: string) => {
					chatStore.setStreaming(false);
					chatStore.setError(errorMsg);
					chatAbortController = null;
				}
			},
			chatAbortController.signal
		);
	}

	function handleChatCancel() {
		chatAbortController?.abort();
		chatStore.setStreaming(false);
		chatAbortController = null;
	}

	function handleApplyFlow(messageId: string) {
		const message = chatStore.messages.find((m) => m.id === messageId);
		// Find the flow attachment in the new attachment-based API
		const flowAttachment = message?.attachments?.find(
			(a): a is import('$lib/types/chat').FlowAttachment => a.kind === 'flow'
		);
		if (!flowAttachment) return;

		// Save current state to history for undo
		flowHistory.push({
			...backendFlow,
			nodes: flowNodes,
			connections: flowConnections
		});

		// Apply flow to canvas
		if (flowAttachment.flow.nodes) {
			flowNodes = [...flowAttachment.flow.nodes];
		}
		if (flowAttachment.flow.connections) {
			flowConnections = [...flowAttachment.flow.connections];
		}

		// Mark as dirty and applied
		dirty = true;
		saveState = { ...saveState, status: 'dirty' };
		chatStore.updateAttachment(messageId, 'flow', { applied: true });
	}

	function handleExportJson() {
		const flowData = {
			id: backendFlow.id,
			name: backendFlow.name,
			description: backendFlow.description,
			nodes: flowNodes,
			connections: flowConnections
		};

		const blob = new Blob([JSON.stringify(flowData, null, 2)], { type: 'application/json' });
		const url = URL.createObjectURL(blob);
		const a = document.createElement('a');
		a.href = url;
		a.download = `${backendFlow.name || 'flow'}.json`;
		a.click();
		URL.revokeObjectURL(url);
	}

	function handleNewChat() {
		chatStore.clearConversation();
	}

	// Track if operations are in progress to prevent concurrent mutations
	let saveInProgress = $state(false);

	// Save handler using fetch API
	async function handleSave() {
		if (saveInProgress) {
			return;
		}

		saveInProgress = true;
		saveState = { ...saveState, status: 'saving' };

		try {
			// Validate before saving (Gate 2: Save-Time Validation)
			const validation = await runFlowValidation(backendFlow.id);

			// Save the flow (even if invalid - draft mode)
			const updated = await saveFlow(backendFlow.id, {
				id: backendFlow.id,
				name: backendFlow.name,
				description: backendFlow.description,
				version: backendFlow.version,
				runtime_state: backendFlow.runtime_state,
				nodes: flowNodes,
				connections: flowConnections
			});

			// Update only backend flow metadata (version, runtime_state)
			// Don't update nodes/connections - flow state is already correct
			backendFlow = {
				...backendFlow,
				version: updated.version,
				runtime_state: updated.runtime_state,
				updated_at: updated.updated_at
			};
			dirty = false;

			// Update save state based on validation result
			if (validation?.validation_status === 'errors') {
				// Draft mode - saved with errors
				saveState = {
					status: 'draft',
					lastSaved: new Date(),
					error: `${validation.errors.length} error${validation.errors.length > 1 ? 's' : ''}`,
					validationResult: validation
				};
			} else {
				// Clean - saved with no errors
				saveState = {
					status: 'clean',
					lastSaved: new Date(),
					error: null,
					validationResult: validation
				};
			}

			// Only update runtimeState if the state actually changed
			if (runtimeState.state !== updated.runtime_state) {
				runtimeState = {
					state: updated.runtime_state,
					message: null,
					lastTransition: new Date()
				};
			}

			// Handle pending operations
			if (pendingDeploy) {
				pendingDeploy = false;
				await handleDeploy();
			}

			if (shouldNavigateAfterSave) {
				shouldNavigateAfterSave = false;
				navigationGuard?.allowNavigation();
			}
		} catch (err) {
			const message = err instanceof Error ? err.message : 'Save failed';
			saveState = { ...saveState, status: 'error', error: message };
			pendingDeploy = false;
			shouldNavigateAfterSave = false;
		} finally {
			saveInProgress = false;
		}
	}

	// Deploy handler using fetch API
	async function handleDeploy() {
		// Gate 3: Deploy-Time Validation - Check for errors before deploying
		if (validationResult?.validation_status === 'errors') {
			showDeployErrorModal = true;
			return;
		}

		// Save first if dirty, otherwise deploy immediately
		if (dirty) {
			pendingDeploy = true;
			await handleSave();
			return;
		}

		try {
			const updated = await deployFlow(backendFlow.id);

			// Update only runtime_state - deploy doesn't change flow structure
			backendFlow = {
				...backendFlow,
				runtime_state: updated.runtime_state,
				updated_at: updated.updated_at
			};
			runtimeState = {
				state: updated.runtime_state,
				message: null,
				lastTransition: new Date()
			};
		} catch (err) {
			// Check if this is a validation error
			if (isValidationError(err)) {
				deployValidationResult = err.validationResult;
				showValidationDialog = true;
				runtimeState = { ...runtimeState, state: 'not_deployed', message: null };
			} else {
				const message = err instanceof Error ? err.message : 'Deploy failed';
				runtimeState = { ...runtimeState, state: 'error', message };
			}
		}
	}

	// Start handler using fetch API
	async function handleStart() {
		try {
			const updated = await startFlow(backendFlow.id);

			// Update only runtime_state - start doesn't change flow structure
			backendFlow = {
				...backendFlow,
				runtime_state: updated.runtime_state,
				updated_at: updated.updated_at
			};
			runtimeState = {
				state: updated.runtime_state,
				message: null,
				lastTransition: new Date()
			};
		} catch (err) {
			const message = err instanceof Error ? err.message : 'Start failed';
			runtimeState = { ...runtimeState, state: 'error', message };
		}
	}

	// Stop handler using fetch API
	async function handleStop() {
		try {
			const updated = await stopFlow(backendFlow.id);

			// Update only runtime_state - stop doesn't change flow structure
			backendFlow = {
				...backendFlow,
				runtime_state: updated.runtime_state,
				updated_at: updated.updated_at
			};
			runtimeState = {
				state: updated.runtime_state,
				message: null,
				lastTransition: new Date()
			};
		} catch (err) {
			const message = err instanceof Error ? err.message : 'Stop failed';
			runtimeState = { ...runtimeState, state: 'error', message };
		}
	}

	// Navigation dialog handlers
	async function handleNavigationSave() {
		shouldNavigateAfterSave = true;
		await handleSave();
	}

	function handleNavigationDiscard() {
		navigationGuard?.allowNavigation();
	}

	function handleNavigationCancel() {
		navigationGuard?.cancelNavigation();
	}

	// Validation dialog handlers
	function handleValidationDialogClose() {
		showValidationDialog = false;
		deployValidationResult = null;
	}

	// Deploy error modal handlers
	function handleDeployErrorModalClose() {
		showDeployErrorModal = false;
	}

	// Validation status modal handlers (Feature 015 - T014)
	function handleValidationStatusClick() {
		showValidationStatusModal = true;
	}

	function handleValidationStatusModalClose() {
		showValidationStatusModal = false;
	}

	// Runtime panel handlers
	function handleToggleRuntimePanel() {
		showRuntimePanel = !showRuntimePanel;
	}

	function handleCloseRuntimePanel() {
		showRuntimePanel = false;
		// Also exit monitor mode when closing
		if (panelLayout.state.monitorMode) {
			panelLayout.setMonitorMode(false);
		}
	}

	function handleToggleMonitorMode() {
		// Check current state BEFORE toggling
		const enteringMonitorMode = !panelLayout.state.monitorMode;
		panelLayout.toggleMonitorMode();
		// Ensure runtime panel is open when entering monitor mode
		if (enteringMonitorMode && !showRuntimePanel) {
			showRuntimePanel = true;
		}
	}

	// Calculate dynamic canvas height based on panel state
	// Canvas fills remaining space in center panel, accounting for status bar and runtime panel
	const statusBarHeight = 48;

	const canvasHeight = $derived(
		showRuntimePanel
			? `calc(100% - ${statusBarHeight}px - ${runtimePanelHeight}px)`
			: `calc(100% - ${statusBarHeight}px)`
	);
</script>

<svelte:head>
	<title>{backendFlow?.name || 'Flow Editor'} - SemTeams</title>
</svelte:head>

<!-- Window event handlers for responsive layout and keyboard shortcuts -->
<svelte:window onresize={handleWindowResize} onkeydown={handleKeyDown} />

<!-- Navigation guard for unsaved changes -->
<NavigationGuard
	bind:this={navigationGuard}
	bind:showDialog={showNavigationDialog}
	{saveState}
/>

<!-- Navigation warning dialog -->
<NavigationDialog
	isOpen={showNavigationDialog}
	onSave={handleNavigationSave}
	onDiscard={handleNavigationDiscard}
	onCancel={handleNavigationCancel}
/>

<!-- Validation error dialog -->
<ValidationErrorDialog
	isOpen={showValidationDialog}
	validationResult={deployValidationResult}
	onClose={handleValidationDialogClose}
/>

<!-- Deploy error modal (Gate 3: Deploy-Time Validation) -->
<DeployErrorModal
	isOpen={showDeployErrorModal}
	{validationResult}
	onClose={handleDeployErrorModalClose}
/>

<!-- Validation status modal (Feature 015 - T014) -->
<ValidationStatusModal
	isOpen={showValidationStatusModal}
	{validationResult}
	onClose={handleValidationStatusModalClose}
/>


<div class="editor-layout">
	<!-- Header -->
	<header class="editor-header">
		<div class="header-content">
			<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
			<a href="/admin/flows" class="back-button" aria-label="Back to flows">← Flows</a>
			<div class="header-text">
				<h1>{backendFlow?.name || 'Loading...'}</h1>
				{#if backendFlow?.description}
					<p>{backendFlow.description}</p>
				{/if}
			</div>
			{#if isFlowRunning}
				<ViewSwitcher
					currentView={panelLayout.state.viewMode}
					onViewChange={handleViewModeChange}
				/>
			{/if}
			<SaveStatusIndicator
				{saveState}
				onSave={handleSave}
				{validationResult}
				onValidationClick={handleValidationStatusClick}
			/>
		</div>
	</header>

	<!-- View Content: Flow Editor or Data Visualization -->
	<div class="panel-area">
		{#if panelLayout.state.viewMode === 'data' && isFlowRunning}
			<!-- Data View: Knowledge Graph Visualization -->
			<DataView flowId={backendFlow.id} />
		{:else}
			<!-- Flow View: Three-Panel Editor Layout -->
			<ThreePanelLayout
				leftPanelOpen={panelLayout.state.monitorMode ? false : panelLayout.state.leftPanelOpen}
				rightPanelOpen={panelLayout.state.monitorMode ? false : panelLayout.state.rightPanelOpen}
				leftPanelWidth={panelLayout.state.leftPanelWidth}
				rightPanelWidth={panelLayout.state.rightPanelWidth}
				onLeftWidthChange={handleLeftWidthChange}
				onRightWidthChange={handleRightWidthChange}
				onToggleLeft={handleToggleLeftPanel}
				onToggleRight={handleToggleRightPanel}
			>
				{#snippet leftPanel()}
					<ChatPanel
						messages={chatStore.messages}
						isStreaming={chatStore.isStreaming}
						streamingContent={chatStore.streamingContent}
						error={chatStore.error}
						onSubmit={handleChatSubmit}
						onCancel={handleChatCancel}
						onApplyFlow={handleApplyFlow}
						onExportJson={handleExportJson}
						onNewChat={handleNewChat}
					/>
				{/snippet}

				{#snippet centerPanel()}
					<div class="center-content" class:monitor-mode={panelLayout.state.monitorMode}>
						{#if !panelLayout.state.monitorMode}
							<div class="canvas-container" style="height: {canvasHeight};">
								<FlowCanvas
									nodes={flowNodes}
									connections={flowConnections}
									{portsMap}
									selectedNodeId={selectedComponent?.id || null}
									onNodeClick={handleNodeClick}
								/>
							</div>

							<StatusBar
								{runtimeState}
								{isFlowValid}
								{showRuntimePanel}
								onDeploy={handleDeploy}
								onStart={handleStart}
								onStop={handleStop}
								onToggleRuntimePanel={handleToggleRuntimePanel}
							/>
						{/if}

						<RuntimePanel
							isOpen={showRuntimePanel || panelLayout.state.monitorMode}
							height={runtimePanelHeight}
							flowId={backendFlow.id}
							onClose={handleCloseRuntimePanel}
							isMonitorMode={panelLayout.state.monitorMode}
							onToggleMonitorMode={handleToggleMonitorMode}
						/>
					</div>
				{/snippet}

				{#snippet rightPanel()}
					<PropertiesPanel
						mode={propertiesPanelMode}
						node={selectedComponent}
						nodeComponentType={selectedNodeComponentType}
						onSave={handlePropertiesSave}
						onDelete={handlePropertiesDelete}
					/>
			{/snippet}
			</ThreePanelLayout>
		{/if}
	</div>
</div>

<style>
	.editor-layout {
		display: flex;
		flex-direction: column;
		height: 100vh;
		overflow: hidden;
	}

	.editor-header {
		padding: 0.75rem 1rem;
		border-bottom: 1px solid var(--ui-border-subtle);
		background: var(--ui-surface-primary);
		flex-shrink: 0;
	}

	.header-content {
		display: flex;
		align-items: center;
		gap: 1rem;
	}

	.back-button {
		color: var(--ui-interactive-primary);
		text-decoration: none;
		font-weight: 500;
		font-size: 0.875rem;
		padding: 0.5rem 0.75rem;
		border-radius: 4px;
		transition: background-color 0.2s;
		white-space: nowrap;
	}

	.back-button:hover {
		background-color: var(--ui-surface-secondary);
	}

	.header-text {
		flex: 1;
		min-width: 0;
	}

	.editor-header h1 {
		margin: 0 0 0.125rem 0;
		font-size: 1.25rem;
		color: var(--ui-text-primary);
	}

	.editor-header p {
		margin: 0;
		color: var(--ui-text-secondary);
		font-size: 0.8125rem;
	}

	.panel-area {
		flex: 1;
		overflow: hidden;
	}

	.center-content {
		display: flex;
		flex-direction: column;
		height: 100%;
	}

	.canvas-container {
		flex: 1;
		position: relative;
		overflow: hidden;
		transition: height 300ms ease-out;
	}

</style>
