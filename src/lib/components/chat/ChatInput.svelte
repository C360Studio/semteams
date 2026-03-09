<script lang="ts">
  import type { ContextChip } from "$lib/types/chat";
  import type { SlashCommand } from "$lib/types/slashCommand";
  import ContextChipBar from "./ContextChipBar.svelte";
  import SlashCommandMenu from "./SlashCommandMenu.svelte";

  interface Props {
    onSubmit: (content: string) => void;
    onCancel?: () => void;
    isStreaming?: boolean;
    disabled?: boolean;
    chips?: ContextChip[];
    onRemoveChip?: (chipId: string) => void;
    onClearChips?: () => void;
    commands?: SlashCommand[];
    onCommandSelect?: (command: SlashCommand) => void;
  }

  let {
    onSubmit,
    onCancel,
    isStreaming = false,
    disabled = false,
    chips,
    onRemoveChip,
    onClearChips,
    commands,
    onCommandSelect,
  }: Props = $props();

  let value = $state("");
  let menuDismissed = $state(false);

  // Show the slash menu when:
  // - commands prop is provided
  // - input starts with "/"
  // - menu has not been explicitly dismissed
  let showSlashMenu = $derived(
    !!(commands && commands.length > 0 && value.startsWith("/") && !menuDismissed),
  );

  // The filter text is everything after the leading "/"
  let slashFilter = $derived(value.startsWith("/") ? value.slice(1) : "");

  function handleKeydown(event: KeyboardEvent) {
    if (event.key === "Escape") {
      if (showSlashMenu) {
        menuDismissed = true;
        return;
      }
      (event.currentTarget as HTMLTextAreaElement).blur();
      return;
    }
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      const trimmed = value.trim();
      if (trimmed) {
        onSubmit(trimmed);
        value = "";
        menuDismissed = false;
      }
    }
  }

  function handleInput() {
    // Reset dismissed state when value changes so menu can reopen
    menuDismissed = false;
  }

  function handleSubmit() {
    const trimmed = value.trim();
    if (trimmed) {
      onSubmit(trimmed);
      value = "";
      menuDismissed = false;
    }
  }

  function handleCommandSelect(command: SlashCommand) {
    value = `/${command.name} `;
    menuDismissed = true;
    onCommandSelect?.(command);
  }
</script>

<div>
  {#if chips && chips.length > 0}
    <ContextChipBar {chips} onRemoveChip={onRemoveChip ?? (() => {})} onClearAll={onClearChips ?? (() => {})} />
  {/if}
  <textarea
    data-testid="chat-input"
    aria-label="Chat message"
    placeholder="Ask a question or type / for commands..."
    {disabled}
    bind:value
    onkeydown={handleKeydown}
    oninput={handleInput}
  ></textarea>
  {#if value.length > 0}
    <span data-testid="chat-char-count">{value.length}</span>
  {/if}
  {#if showSlashMenu}
    <SlashCommandMenu
      commands={commands ?? []}
      filter={slashFilter}
      onSelect={handleCommandSelect}
      onDismiss={() => (menuDismissed = true)}
    />
  {/if}
  {#if isStreaming}
    <button data-testid="chat-cancel" type="button" onclick={onCancel}>
      Cancel
    </button>
  {:else}
    <button
      data-testid="chat-submit"
      type="button"
      {disabled}
      onclick={handleSubmit}
    >
      Submit
    </button>
  {/if}
</div>
