<script lang="ts">
  // ArtifactCard renders the arguments of an `emit_*` tool call as
  // structured human-readable content instead of a raw JSON blob. Used
  // by TaskStory (and any future surface) when a trajectory step's
  // tool_name starts with "emit_" — the convention across the
  // pack-emitters: emit_plan, emit_research_artifact,
  // emit_autoresearch_baseline, emit_autoresearch_artifact.
  //
  // The emit-tools own their typed args; this renderer is shape-agnostic
  // — it walks the args object, lifts `title` + `revision` into a
  // header, and emits each remaining field as a labelled section. The
  // value renderer handles four shapes: scalar (string/number/boolean),
  // array-of-strings (bullet list), array-of-objects (definition list
  // per item), object (flat definition list). Multi-line strings preserve
  // their newlines via white-space: pre-wrap — enough to make a markdown
  // body readable without a full markdown parser (which would add a
  // dependency + XSS surface for an LLM-authored payload).

  interface Props {
    toolName: string;
    args: Record<string, unknown> | undefined;
  }

  let { toolName, args }: Props = $props();

  // Header lifts `title` + `revision` out of args. Both are conventions
  // across emit_* tools (emit_plan.title + emit_*.revision in the
  // research/autoresearch pack fixtures).
  const titleValue = $derived(
    args && typeof args.title === "string" ? args.title : null,
  );
  const revisionValue = $derived(
    args && (typeof args.revision === "number" || typeof args.revision === "string")
      ? String(args.revision)
      : null,
  );

  // Everything except title + revision renders as a section. Stable
  // ordering: as the keys come off args (insertion order). Iterating an
  // unknown shape directly would need a $derived; do it inline below.
  const sectionKeys = $derived.by<string[]>(() => {
    if (!args) return [];
    return Object.keys(args).filter((k) => k !== "title" && k !== "revision");
  });

  function isArrayOfObjects(v: unknown): v is Array<Record<string, unknown>> {
    return (
      Array.isArray(v) &&
      v.length > 0 &&
      v.every((x) => x !== null && typeof x === "object" && !Array.isArray(x))
    );
  }

  function isArrayOfScalars(v: unknown): v is Array<string | number | boolean> {
    return (
      Array.isArray(v) &&
      v.every(
        (x) =>
          typeof x === "string" || typeof x === "number" || typeof x === "boolean",
      )
    );
  }

  function isPlainObject(v: unknown): v is Record<string, unknown> {
    return v !== null && typeof v === "object" && !Array.isArray(v);
  }

  function labelForKey(key: string): string {
    // "integration_points" → "Integration points". "scope_in" → "Scope in".
    const spaced = key.replace(/_/g, " ");
    return spaced.charAt(0).toUpperCase() + spaced.slice(1);
  }

  function formatScalar(v: unknown): string {
    if (v === null) return "null";
    if (v === undefined) return "";
    if (typeof v === "string") return v;
    if (typeof v === "number" || typeof v === "boolean") return String(v);
    try {
      return JSON.stringify(v);
    } catch {
      return "(unserialisable)";
    }
  }
</script>

<article class="artifact-card" data-testid="artifact-card" data-tool-name={toolName}>
  <header class="artifact-header">
    <span class="artifact-tool" data-testid="artifact-tool-name">{toolName}</span>
    {#if revisionValue !== null}
      <span class="artifact-revision" data-testid="artifact-revision">
        rev {revisionValue}
      </span>
    {/if}
  </header>

  {#if titleValue}
    <h4 class="artifact-title" data-testid="artifact-title">{titleValue}</h4>
  {/if}

  {#if !args || sectionKeys.length === 0}
    <p class="artifact-empty">No additional fields.</p>
  {:else}
    <dl class="artifact-fields">
      {#each sectionKeys as key (key)}
        {@const value = args[key]}
        <div class="field" data-testid="artifact-field" data-field-key={key}>
          <dt class="field-label">{labelForKey(key)}</dt>
          <dd class="field-value">
            {#if typeof value === "string"}
              <p class="value-text">{value}</p>
            {:else if typeof value === "number" || typeof value === "boolean"}
              <p class="value-scalar">{formatScalar(value)}</p>
            {:else if isArrayOfScalars(value)}
              {#if value.length === 0}
                <p class="value-empty">(empty)</p>
              {:else}
                <ul class="value-list">
                  {#each value as item, idx (idx)}
                    <li>{formatScalar(item)}</li>
                  {/each}
                </ul>
              {/if}
            {:else if isArrayOfObjects(value)}
              <ol class="value-objects">
                {#each value as item, idx (idx)}
                  <li class="value-object">
                    <dl class="object-fields">
                      {#each Object.entries(item) as [k, v] (k)}
                        <div class="object-pair">
                          <dt class="object-key">{labelForKey(k)}</dt>
                          <dd class="object-val">{formatScalar(v)}</dd>
                        </div>
                      {/each}
                    </dl>
                  </li>
                {/each}
              </ol>
            {:else if Array.isArray(value) && value.length === 0}
              <p class="value-empty">(empty)</p>
            {:else if isPlainObject(value)}
              <dl class="object-fields">
                {#each Object.entries(value) as [k, v] (k)}
                  <div class="object-pair">
                    <dt class="object-key">{labelForKey(k)}</dt>
                    <dd class="object-val">{formatScalar(v)}</dd>
                  </div>
                {/each}
              </dl>
            {:else}
              <p class="value-scalar">{formatScalar(value)}</p>
            {/if}
          </dd>
        </div>
      {/each}
    </dl>
  {/if}
</article>

<style>
  .artifact-card {
    background: var(--ui-surface-secondary, #f9fafb);
    border: 1px solid var(--ui-border-subtle, #e5e7eb);
    border-radius: 4px;
    padding: 0.625rem 0.75rem;
    font-size: 0.8125rem;
  }

  .artifact-header {
    display: flex;
    align-items: baseline;
    gap: 0.5rem;
    margin-bottom: 0.375rem;
  }

  .artifact-tool {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.6875rem;
    color: var(--ui-text-tertiary, #9ca3af);
  }

  .artifact-revision {
    font-size: 0.625rem;
    color: var(--ui-text-secondary, #6b7280);
    padding: 0 0.25rem;
    background: var(--ui-surface-tertiary, #e5e7eb);
    border-radius: 2px;
    font-variant-numeric: tabular-nums;
  }

  .artifact-title {
    margin: 0 0 0.5rem;
    font-size: 0.875rem;
    font-weight: 600;
    color: var(--ui-text-primary, #111827);
  }

  .artifact-empty {
    margin: 0;
    color: var(--ui-text-tertiary, #9ca3af);
    font-style: italic;
    font-size: 0.75rem;
  }

  .artifact-fields {
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.125rem;
  }

  .field-label {
    font-size: 0.6875rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--ui-text-secondary, #6b7280);
    margin: 0;
  }

  .field-value {
    margin: 0;
    color: var(--ui-text-primary, #111827);
  }

  .value-text {
    margin: 0;
    white-space: pre-wrap;
    word-break: break-word;
    line-height: 1.5;
  }

  .value-scalar {
    margin: 0;
    font-variant-numeric: tabular-nums;
  }

  .value-empty {
    margin: 0;
    color: var(--ui-text-tertiary, #9ca3af);
    font-style: italic;
    font-size: 0.75rem;
  }

  .value-list {
    margin: 0;
    padding-left: 1.125rem;
    display: flex;
    flex-direction: column;
    gap: 0.125rem;
  }

  .value-list li {
    line-height: 1.5;
  }

  .value-objects {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.375rem;
  }

  .value-object {
    padding: 0.375rem 0.5rem;
    background: var(--ui-surface-primary, #fff);
    border: 1px solid var(--ui-border-subtle, #e5e7eb);
    border-radius: 3px;
  }

  .object-fields {
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.1875rem;
  }

  .object-pair {
    display: flex;
    gap: 0.375rem;
    align-items: baseline;
  }

  .object-key {
    margin: 0;
    flex-shrink: 0;
    min-width: 6rem;
    font-size: 0.6875rem;
    font-weight: 600;
    color: var(--ui-text-secondary, #6b7280);
  }

  .object-val {
    margin: 0;
    color: var(--ui-text-primary, #111827);
    line-height: 1.4;
    word-break: break-word;
  }
</style>
