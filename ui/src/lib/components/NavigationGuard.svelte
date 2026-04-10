<script lang="ts">
	import { beforeNavigate, goto } from '$app/navigation';
	import { browser } from '$app/environment';
	import type { SaveState } from '$lib/types/ui-state';

	interface Props {
		saveState: SaveState;
		showDialog?: boolean;
		onShowDialog?: (show: boolean) => void;
		onNavigationAllowed?: () => void;
	}

	let { saveState, showDialog = $bindable(false), onShowDialog, onNavigationAllowed }: Props = $props();

	// Store pending navigation
	let pendingNavigation: { url: string } | null = $state(null);
	let isNavigating = $state(false);

	// Browser navigation guard (close/reload)
	// Extract status to ensure reactivity tracks the property
	$effect(() => {
		if (!browser) return;

		// Explicitly read the status property to track it
		const status = saveState.status;

		if (status === 'dirty') {
			// Set beforeunload handler when dirty
			window.onbeforeunload = (e: BeforeUnloadEvent) => {
				e.preventDefault();
				e.returnValue = ''; // Chrome requires this
				return ''; // Some browsers need this
			};
		} else {
			// Clear handler when clean
			window.onbeforeunload = null;
		}

		return () => {
			// Cleanup on component unmount
			window.onbeforeunload = null;
		};
	});

	// SvelteKit navigation guard (client-side routing)
	beforeNavigate(({ to, cancel }) => {
		// Skip guard if we're in the process of navigating after user confirmation
		if (isNavigating) {
			isNavigating = false; // Reset flag
			return;
		}

		// Only intercept navigation if state is dirty AND we have a destination
		// Note: to is null/undefined for browser back/forward, which we can't prevent
		// (those are handled by the beforeunload handler instead)
		if (saveState.status === 'dirty' && to) {
			// Cancel navigation initially
			cancel();

			// Store the destination
			pendingNavigation = { url: to.url.pathname };

			// Show dialog
			showDialog = true;
			onShowDialog?.(true);
		}
	});

	// Allow navigation after user confirms
	export function allowNavigation() {
		if (pendingNavigation) {
			const destination = pendingNavigation.url;
			pendingNavigation = null;
			showDialog = false;
			onShowDialog?.(false);
			onNavigationAllowed?.();

			// Set flag to bypass guard on next navigation
			isNavigating = true;
			// eslint-disable-next-line svelte/no-navigation-without-resolve
			goto(destination);
		}
	}

	// Cancel navigation
	export function cancelNavigation() {
		pendingNavigation = null;
		showDialog = false;
		onShowDialog?.(false);
	}
</script>

<!-- This component has no visual output - it only manages navigation guards -->
