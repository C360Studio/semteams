<script lang="ts">
  import type { SlashCommand } from "$lib/types/slashCommand";

  interface Props {
    commands: SlashCommand[];
    filter: string;
    onSelect: (command: SlashCommand) => void;
    onDismiss?: () => void;
  }

  let { commands, filter, onSelect, onDismiss }: Props = $props();

  // Filter commands by name prefix or alias prefix (case-insensitive)
  let filtered = $derived(
    filter
      ? commands.filter(
          (cmd) =>
            cmd.name.startsWith(filter.toLowerCase()) ||
            cmd.aliases.some((alias) => alias.startsWith(filter.toLowerCase())),
        )
      : commands,
  );

  function handleKeydown(event: KeyboardEvent, command: SlashCommand) {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      onSelect(command);
    } else if (event.key === "Escape") {
      onDismiss?.();
    }
  }

  function handleBlur() {
    onDismiss?.();
  }
</script>

<div
  data-testid="slash-command-menu"
  role="listbox"
  aria-label="Slash commands"
  onblur={handleBlur}
>
  {#each filtered as command (command.name)}
    <button
      role="option"
      aria-selected="false"
      onclick={() => onSelect(command)}
      onkeydown={(e) => handleKeydown(e, command)}
      type="button"
      class="slash-command-item"
    >
      <span class="slash-command-name">/{command.name}</span>
      {#if command.aliases.length > 0}
        <span class="slash-command-aliases">
          {command.aliases.map((a) => `/${a}`).join(", ")}
        </span>
      {/if}
      <span class="slash-command-description">{command.description}</span>
    </button>
  {/each}
</div>

<style>
  [data-testid="slash-command-menu"] {
    display: flex;
    flex-direction: column;
    background: var(--pico-background-color, #fff);
    border: 1px solid var(--pico-muted-border-color, #ccc);
    border-radius: 4px;
    overflow: hidden;
  }

  .slash-command-item {
    display: flex;
    align-items: baseline;
    gap: 0.5rem;
    padding: 0.5rem 0.75rem;
    background: none;
    border: none;
    cursor: pointer;
    text-align: left;
    width: 100%;
  }

  .slash-command-item:hover,
  .slash-command-item:focus {
    background: var(--pico-primary-focus, rgba(0, 0, 0, 0.05));
  }

  .slash-command-name {
    font-weight: 600;
    flex-shrink: 0;
  }

  .slash-command-aliases {
    font-size: 0.8em;
    color: var(--pico-muted-color, #888);
    flex-shrink: 0;
  }

  .slash-command-description {
    font-size: 0.875em;
    color: var(--pico-muted-color, #666);
  }
</style>
