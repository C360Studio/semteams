<script lang="ts">
  import type { Overlay } from "$lib/types/work";

  interface Props {
    /** Test-id suffix and data hook, e.g. `execution-stage` -> `overlay-execution-stage`. */
    name: string;
    /** Visible lead-in so each badge names what it reports; omit where a column header does. */
    label?: string;
    overlay: Overlay<string | number | boolean>;
    /** Shown only for a `none` overlay, e.g. "no linked run". */
    noneLabel: string;
    /** Text for a known value; defaults to String(value). */
    format?: (value: string | number | boolean) => string;
  }

  let { name, label, overlay, noneLabel, format }: Props = $props();

  let knownValue = $derived(overlay.state === "known" ? overlay.value : undefined);
  let text = $derived(
    knownValue === undefined ? "" : (format ? format(knownValue) : String(knownValue)),
  );
</script>

<!--
  State is carried by text and border style, never by colour alone (WCAG 1.4.1):
  unknown is a dashed, italic "unknown" with a "?" glyph; none is a dotted
  badge with its explicit label. The reason is in the title and, for assistive
  technology and touch, as visually hidden text.
-->
<span
  class="overlay-badge"
  data-testid="overlay-{name}"
  data-state={overlay.state}
  data-value={knownValue === undefined ? undefined : String(knownValue)}
  title={overlay.reason}
>
  {#if label}
    <span class="overlay-label">{label}</span>
  {/if}
  <span class="overlay-value">
    {#if overlay.state === "known"}
      {text}
    {:else if overlay.state === "none"}
      <span class="glyph" aria-hidden="true">&ndash;</span>{noneLabel}
    {:else}
      <span class="glyph" aria-hidden="true">?</span>unknown
    {/if}
  </span>
  {#if overlay.reason}
    <span class="sr-only">({overlay.reason})</span>
  {/if}
</span>

<style>
  .overlay-badge {
    display: inline-flex;
    align-items: baseline;
    gap: 0.25rem;
    padding: 0.0625rem 0.4375rem;
    border: 1px solid var(--ui-border-strong, #9ca3af);
    border-radius: 9999px;
    background: var(--ui-surface-primary, #fff);
    color: var(--ui-text-primary, #111827);
    font-size: 0.6875rem;
    line-height: 1.4;
    white-space: nowrap;
  }

  .overlay-label {
    font-weight: 600;
    color: var(--ui-text-secondary, #6b7280);
  }

  .glyph {
    margin-right: 0.125rem;
    font-weight: 700;
  }

  .overlay-badge[data-state="unknown"] {
    border-style: dashed;
    font-style: italic;
    color: var(--ui-text-secondary, #6b7280);
  }

  .overlay-badge[data-state="none"] {
    border-style: dotted;
    color: var(--ui-text-secondary, #6b7280);
  }

  .overlay-badge[data-state="known"][data-value="failed"] {
    background: var(--status-error-container, #fee2e2);
    color: var(--status-error-on-container, #991b1b);
  }

  .overlay-badge[data-state="known"][data-value="true"] {
    background: var(--status-warning-container, #ffedd5);
    color: var(--status-warning-on-container, #9a3412);
    font-weight: 600;
  }

  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border-width: 0;
  }
</style>
