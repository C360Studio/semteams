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
  // per item), object (flat definition list). Top-level string fields go
  // through renderMarkdown — a dependency-free, escape-first, XSS-safe
  // renderer (see markdown.ts) — so a markdown body reads as formatted
  // prose. Short scalars nested in arrays/objects stay plain (Svelte
  // auto-escapes those).

  import {
    ArrowDownTray,
    CheckCircle,
    ClipboardDocument,
    Icon,
    PencilSquare,
    XCircle,
    ArrowPath,
  } from "svelte-hero-icons";
  import { chatHandoff } from "$lib/stores/chatHandoff.svelte";
  import { renderMarkdown } from "$lib/utils/markdown";
  import { createZipBlob } from "$lib/utils/zip";

  interface Props {
    toolName: string;
    args: Record<string, unknown> | undefined;
  }

  let { toolName, args }: Props = $props();

  type ReviewDecision = "approved" | "rejected" | "revision";
  interface OpenSpecFile {
    path: string;
    content: string;
  }

  let reviewMode = $state<"preview" | "edit">("preview");
  let reviewDecision = $state<ReviewDecision | null>(null);
  let editDraft = $state("");
  let editedOpenSpecMarkdown = $state<string | null>(null);
  let exportStatus = $state<string | null>(null);
  let artifactStatus = $state<string | null>(null);

  const HANDOFF_TEAMS = [
    { id: "research", label: "Research", prefix: "/research" },
    { id: "spec", label: "Spec", prefix: "/create-change" },
    { id: "optimize", label: "Optimize", prefix: "/optimize" },
    { id: "build", label: "Build", prefix: "/dev-via-test" },
  ] as const;

  // Header lifts `title` + `revision` out of args. Both are conventions
  // across emit_* tools (emit_plan.title + emit_*.revision in the
  // research/autoresearch pack fixtures).
  const titleValue = $derived(
    args && typeof args.title === "string" ? args.title : null,
  );
  const revisionValue = $derived(
    args &&
      (typeof args.revision === "number" || typeof args.revision === "string")
      ? String(args.revision)
      : null,
  );
  const isOpenSpecChange = $derived(
    toolName === "emit_change" && isPlainObject(args),
  );
  const isReusableArtifact = $derived(
    toolName.startsWith("emit_") && !isOpenSpecChange && isPlainObject(args),
  );
  const artifactTitle = $derived(
    titleValue ?? labelForKey(toolName.replace(/^emit_/, "") || "artifact"),
  );
  const openSpecSlug = $derived(
    isOpenSpecChange ? stringField(args, "slug", "change") : "",
  );
  const openSpecFiles = $derived(
    isOpenSpecChange && args ? renderOpenSpecFiles(args, openSpecSlug) : [],
  );
  const openSpecMarkdown = $derived(
    isOpenSpecChange ? renderOpenSpecDocument(openSpecFiles, openSpecSlug) : "",
  );
  const effectiveOpenSpecMarkdown = $derived(
    editedOpenSpecMarkdown ?? openSpecMarkdown,
  );

  // Everything except title + revision renders as a section. Stable
  // ordering: as the keys come off args (insertion order). Iterating an
  // unknown shape directly would need a $derived; do it inline below.
  const sectionKeys = $derived.by<string[]>(() => {
    if (!args) return [];
    if (isOpenSpecChange) return [];
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
          typeof x === "string" ||
          typeof x === "number" ||
          typeof x === "boolean",
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

  // Top-level string fields can carry markdown prose (research synthesis,
  // rollup narratives). renderMarkdown escapes first and emits a fixed tag
  // whitelist, so its output is XSS-safe for {@html}. Short scalar values
  // (inside arrays/objects) stay plain — Svelte auto-escapes those.

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

  function stringField(
    obj: Record<string, unknown> | undefined,
    key: string,
    fallback = "",
  ): string {
    const v = obj?.[key];
    return typeof v === "string" && v.trim() ? v : fallback;
  }

  function stringList(v: unknown): string[] {
    return Array.isArray(v)
      ? v.filter((x): x is string => typeof x === "string")
      : [];
  }

  function objectList(v: unknown): Array<Record<string, unknown>> {
    return Array.isArray(v)
      ? v.filter((x): x is Record<string, unknown> => isPlainObject(x))
      : [];
  }

  function titleFromSlug(slug: string): string {
    return slug
      .split("-")
      .filter(Boolean)
      .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
      .join(" ");
  }

  function linesForList(items: string[]): string[] {
    return items.length > 0 ? items.map((item) => `- ${item}`) : ["- none"];
  }

  function stepKeyword(step: Record<string, unknown>): string {
    const kw = step.kw ?? step.keyword;
    return typeof kw === "string" && kw.trim()
      ? kw.trim().toUpperCase()
      : "AND";
  }

  function renderRequirement(req: Record<string, unknown>): string {
    const name = stringField(req, "name", "Requirement");
    const statement = stringField(req, "statement");
    const scenarios = objectList(req.scenarios);
    const out = [`### Requirement: ${name}`, statement, ""];
    for (const scenario of scenarios) {
      out.push(`#### Scenario: ${stringField(scenario, "name", "Scenario")}`);
      for (const step of objectList(scenario.steps)) {
        out.push(`- ${stepKeyword(step)} ${stringField(step, "text")}`);
      }
      out.push("");
    }
    return out.join("\n").trimEnd();
  }

  function normalizeMarkdown(lines: string[]): string {
    return (
      lines
        .join("\n")
        .replace(/\n{3,}/g, "\n\n")
        .trim() + "\n"
    );
  }

  function renderProposalContent(
    change: Record<string, unknown>,
    slug: string,
  ): string {
    const proposal: Record<string, unknown> = isPlainObject(change.proposal)
      ? change.proposal
      : {};
    return normalizeMarkdown([
      `# Proposal: ${titleFromSlug(slug) || slug}`,
      "",
      "## Intent",
      stringField(proposal, "intent"),
      "",
      "## Scope",
      "In scope:",
      ...linesForList(stringList(proposal.scope_in)),
      "",
      "Out of scope:",
      ...linesForList(stringList(proposal.scope_out)),
      "",
      "## Approach",
      stringField(proposal, "approach"),
    ]);
  }

  function renderDesignContent(
    change: Record<string, unknown>,
    slug: string,
  ): string | null {
    const design: Record<string, unknown> | null = isPlainObject(change.design)
      ? change.design
      : null;
    if (!design) return null;

    const out = [
      `# Design: ${titleFromSlug(slug) || slug}`,
      "",
      "## Technical Approach",
      stringField(design, "technical_approach"),
    ];
    const decisions = objectList(design.decisions);
    if (decisions.length) {
      out.push("", "## Architecture Decisions", "");
      for (const decision of decisions) {
        out.push(`### Decision: ${stringField(decision, "name", "Decision")}`);
        out.push(stringField(decision, "body"));
        out.push("");
      }
    }
    if (stringField(design, "data_flow")) {
      out.push("## Data Flow", stringField(design, "data_flow"));
    }
    const files = objectList(design.file_changes);
    if (files.length) {
      out.push("", "## File Changes");
      for (const file of files) {
        const kind = stringField(file, "kind");
        const path = stringField(file, "path");
        out.push(`- \`${path}\`${kind ? ` (${kind})` : ""}`);
      }
    }
    return normalizeMarkdown(out);
  }

  function renderDeltaContent(delta: Record<string, unknown>): string {
    const capability = stringField(delta, "capability", "capability");
    const out = [
      `# Delta for ${titleFromSlug(capability) || capability}`,
      "",
      "## ADDED Requirements",
      "",
    ];
    const added = objectList(delta.added);
    out.push(...(added.length ? added.map(renderRequirement) : ["_None._"]));
    out.push("", "## MODIFIED Requirements", "");
    const modified = objectList(delta.modified);
    if (modified.length) {
      for (const req of modified) {
        out.push(renderRequirement(req));
        out.push(`(Previously: ${stringField(req, "previously")})`);
        out.push("");
      }
    } else {
      out.push("_None._", "");
    }
    out.push("## REMOVED Requirements", "");
    const removed = objectList(delta.removed);
    if (removed.length) {
      for (const req of removed) {
        out.push(`### Requirement: ${stringField(req, "name", "Requirement")}`);
        out.push(`(Rationale: ${stringField(req, "rationale")})`);
        out.push("");
      }
    } else {
      out.push("_None._", "");
    }
    return normalizeMarkdown(out);
  }

  function renderOpenSpecTasksContent(
    tasks: Array<Record<string, unknown>>,
  ): string {
    const out = ["# Tasks", ""];
    let currentSection: string | null = null;
    for (const task of tasks) {
      const section = stringField(task, "section");
      if (section !== currentSection) {
        if (out[out.length - 1] !== "") out.push("");
        out.push(`## ${section || "Tasks"}`);
        currentSection = section;
      }
      const done = task.done === true ? "x" : " ";
      const number = stringField(task, "number");
      const label = number
        ? `${number} ${stringField(task, "text")}`
        : stringField(task, "text");
      out.push(`- [${done}] ${label}`);
    }
    return normalizeMarkdown(out);
  }

  function renderOpenSpecFiles(
    change: Record<string, unknown>,
    slug: string,
  ): OpenSpecFile[] {
    const files: OpenSpecFile[] = [
      {
        path: `openspec/changes/${slug}/proposal.md`,
        content: renderProposalContent(change, slug),
      },
    ];
    const design = renderDesignContent(change, slug);
    if (design) {
      files.push({
        path: `openspec/changes/${slug}/design.md`,
        content: design,
      });
    }
    for (const delta of objectList(change.deltas)) {
      const capability = stringField(delta, "capability", "capability");
      files.push({
        path: `openspec/changes/${slug}/specs/${capability}/spec.md`,
        content: renderDeltaContent(delta),
      });
    }
    const tasks = objectList(change.tasks);
    if (tasks.length) {
      files.push({
        path: `openspec/changes/${slug}/tasks.md`,
        content: renderOpenSpecTasksContent(tasks),
      });
    }
    return files;
  }

  function renderOpenSpecDocument(files: OpenSpecFile[], slug: string): string {
    const out = [`# OpenSpec change: ${slug}`, ""];
    for (const file of files) {
      out.push(`<!-- ${file.path} -->`, file.content.trim(), "");
    }
    return normalizeMarkdown(out);
  }

  function startOpenSpecEdit() {
    editDraft = effectiveOpenSpecMarkdown;
    reviewMode = "edit";
    exportStatus = null;
  }

  function finishOpenSpecEdit() {
    editedOpenSpecMarkdown = editDraft;
    reviewMode = "preview";
    exportStatus = "Draft updated";
  }

  function setReviewDecision(decision: ReviewDecision) {
    reviewDecision = decision;
    exportStatus =
      decision === "approved"
        ? "Approved"
        : decision === "rejected"
          ? "Rejected"
          : "Revision requested";
  }

  function currentOpenSpecText(): string {
    return reviewMode === "edit" ? editDraft : effectiveOpenSpecMarkdown;
  }

  function markdownForObject(obj: Record<string, unknown>): string[] {
    const out: string[] = [];
    for (const [key, value] of Object.entries(obj)) {
      out.push(`- **${labelForKey(key)}:** ${formatScalar(value)}`);
    }
    return out;
  }

  function markdownForValue(value: unknown): string[] {
    if (typeof value === "string") return [value.trim() || "_empty_"];
    if (typeof value === "number" || typeof value === "boolean") {
      return [String(value)];
    }
    if (isArrayOfScalars(value)) {
      return value.length
        ? value.map((item) => `- ${formatScalar(item)}`)
        : ["_empty_"];
    }
    if (isArrayOfObjects(value)) {
      if (value.length === 0) return ["_empty_"];
      return value.flatMap((item, idx) => [
        `### Item ${idx + 1}`,
        ...markdownForObject(item),
        "",
      ]);
    }
    if (Array.isArray(value) && value.length === 0) return ["_empty_"];
    if (isPlainObject(value)) return markdownForObject(value);
    return [formatScalar(value)];
  }

  function genericArtifactMarkdown(): string {
    if (!args) return `# ${artifactTitle}\n\nArtifact tool: ${toolName}\n`;
    const out = [`# ${artifactTitle}`, "", `Artifact tool: ${toolName}`];
    if (revisionValue !== null) out.push(`Revision: ${revisionValue}`);
    for (const key of sectionKeys) {
      out.push(
        "",
        `## ${labelForKey(key)}`,
        "",
        ...markdownForValue(args[key]),
      );
    }
    return normalizeMarkdown(out);
  }

  function artifactContext() {
    return {
      id: `${toolName}:${artifactTitle}`,
      title: artifactTitle,
      toolName,
      content: genericArtifactMarkdown(),
    };
  }

  async function copyGenericArtifact() {
    artifactStatus = null;
    try {
      await navigator.clipboard.writeText(genericArtifactMarkdown());
      artifactStatus = "Copied";
    } catch {
      artifactStatus = "Copy failed";
    }
  }

  function useArtifactWith(prefix: string) {
    artifactStatus = null;
    chatHandoff.stage({
      content: `${prefix} Use the attached artifact as context to `,
      artifact: artifactContext(),
    });
    artifactStatus = "Added to chat";
  }

  function implementationCommand(): string {
    return `/implement-spec ${openSpecSlug || "change-slug"}`;
  }

  async function copyOpenSpec() {
    exportStatus = null;
    const text = currentOpenSpecText();
    try {
      await navigator.clipboard.writeText(text);
      exportStatus = "Copied";
    } catch {
      exportStatus = "Copy failed";
    }
  }

  async function copyImplementationCommand() {
    exportStatus = null;
    try {
      await navigator.clipboard.writeText(implementationCommand());
      exportStatus = "Implementation command copied";
    } catch {
      exportStatus = "Implementation command copy failed";
    }
  }

  function downloadOpenSpec() {
    exportStatus = null;
    downloadTextFile(
      `${openSpecSlug || "openspec-change"}.md`,
      currentOpenSpecText(),
      "text/markdown;charset=utf-8",
    );
    exportStatus = "Document downloaded";
  }

  function downloadOpenSpecArchive() {
    exportStatus = null;
    try {
      downloadBlob(
        `${openSpecSlug || "openspec-change"}.openspec.zip`,
        createZipBlob(openSpecFiles),
      );
      exportStatus = "Folder archive downloaded";
    } catch {
      exportStatus = "Folder archive failed";
    }
  }

  function downloadBlob(filename: string, blob: Blob) {
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
  }

  function downloadTextFile(filename: string, text: string, mimeType: string) {
    downloadBlob(filename, new Blob([text], { type: mimeType }));
  }
</script>

<article
  class="artifact-card"
  data-testid="artifact-card"
  data-tool-name={toolName}
>
  <header class="artifact-header">
    <span class="artifact-tool" data-testid="artifact-tool-name"
      >{toolName}</span
    >
    {#if revisionValue !== null}
      <span class="artifact-revision" data-testid="artifact-revision">
        rev {revisionValue}
      </span>
    {/if}
  </header>

  {#if titleValue}
    <h4 class="artifact-title" data-testid="artifact-title">{titleValue}</h4>
  {/if}

  {#if isReusableArtifact}
    <section
      class="artifact-handoff"
      data-testid="artifact-handoff-panel"
      aria-label="Artifact handoff actions"
    >
      <div class="artifact-handoff-copy">
        <p class="artifact-handoff-eyebrow">Use as context</p>
        <p class="artifact-handoff-help">
          Attach this artifact to a follow-up prompt for any team.
        </p>
      </div>
      <div class="artifact-handoff-actions">
        <button
          type="button"
          class="artifact-btn"
          data-testid="artifact-copy"
          onclick={copyGenericArtifact}
          title="Copy rendered artifact"
        >
          <Icon src={ClipboardDocument} size="15" />
          <span>Copy</span>
        </button>
        {#each HANDOFF_TEAMS as team (team.id)}
          <button
            type="button"
            class="artifact-btn"
            data-testid="artifact-use-{team.id}"
            onclick={() => useArtifactWith(team.prefix)}
            title="Use artifact with {team.label}"
          >
            {team.label}
          </button>
        {/each}
      </div>
      {#if artifactStatus}
        <p
          class="artifact-status"
          data-testid="artifact-handoff-state"
          role="status"
          aria-live="polite"
        >
          {artifactStatus}
        </p>
      {/if}
    </section>
  {/if}

  {#if isOpenSpecChange}
    <section class="openspec-review" data-testid="openspec-review-panel">
      <div class="openspec-summary">
        <div>
          <p class="openspec-eyebrow">OpenSpec change</p>
          <h4 class="openspec-title" data-testid="openspec-title">
            {openSpecSlug}
          </h4>
        </div>
        {#if stringField(args, "acceptance_command")}
          <p class="openspec-command" data-testid="openspec-acceptance">
            {stringField(args, "acceptance_command")}
          </p>
        {/if}
      </div>

      <div class="openspec-actions" aria-label="Spec review actions">
        <button
          type="button"
          class="openspec-btn"
          data-testid="openspec-edit"
          onclick={startOpenSpecEdit}
          title="Edit rendered OpenSpec"
        >
          <Icon src={PencilSquare} size="15" />
          <span>Edit</span>
        </button>
        <button
          type="button"
          class="openspec-btn"
          data-testid="openspec-approve"
          onclick={() => setReviewDecision("approved")}
          title="Approve spec"
        >
          <Icon src={CheckCircle} size="15" />
          <span>Approve</span>
        </button>
        <button
          type="button"
          class="openspec-btn"
          data-testid="openspec-revision"
          onclick={() => setReviewDecision("revision")}
          title="Request revision"
        >
          <Icon src={ArrowPath} size="15" />
          <span>Revise</span>
        </button>
        <button
          type="button"
          class="openspec-btn danger"
          data-testid="openspec-reject"
          onclick={() => setReviewDecision("rejected")}
          title="Reject spec"
        >
          <Icon src={XCircle} size="15" />
          <span>Reject</span>
        </button>
      </div>

      <div class="openspec-export-actions" aria-label="Spec export actions">
        <button
          type="button"
          class="openspec-btn"
          data-testid="openspec-copy"
          onclick={copyOpenSpec}
          title="Copy OpenSpec handoff document"
        >
          <Icon src={ClipboardDocument} size="15" />
          <span>Copy</span>
        </button>
        <button
          type="button"
          class="openspec-btn"
          data-testid="openspec-download"
          onclick={downloadOpenSpec}
          title="Download OpenSpec handoff document"
        >
          <Icon src={ArrowDownTray} size="15" />
          <span>Download Doc</span>
        </button>
        <button
          type="button"
          class="openspec-btn"
          data-testid="openspec-download-folder"
          onclick={downloadOpenSpecArchive}
          title="Download OpenSpec folder archive"
        >
          <Icon src={ArrowDownTray} size="15" />
          <span>Download Folder</span>
        </button>
      </div>

      {#if reviewDecision}
        <p
          class="openspec-state"
          data-testid="openspec-review-state"
          data-state={reviewDecision}
        >
          {reviewDecision === "approved"
            ? "Approved"
            : reviewDecision === "rejected"
              ? "Rejected"
              : "Revision requested"}
        </p>
      {/if}
      {#if reviewDecision === "approved"}
        <div
          class="openspec-handoff"
          data-testid="openspec-implementation-handoff"
          role="status"
          aria-live="polite"
        >
          <div class="handoff-copy">
            <p class="handoff-title">Approved spec handoff</p>
            <code
              class="handoff-command"
              data-testid="openspec-implementation-command"
            >
              {implementationCommand()}
            </code>
          </div>
          <button
            type="button"
            class="openspec-btn"
            data-testid="openspec-copy-implementation"
            onclick={copyImplementationCommand}
            title="Copy implementation command"
          >
            <Icon src={ClipboardDocument} size="15" />
            <span>Copy Command</span>
          </button>
        </div>
      {/if}
      {#if exportStatus}
        <p
          class="openspec-state"
          data-testid="openspec-export-state"
          role="status"
          aria-live="polite"
        >
          {exportStatus}
        </p>
      {/if}

      {#if reviewMode === "edit"}
        <textarea
          class="openspec-editor"
          data-testid="openspec-editor"
          bind:value={editDraft}
          aria-label="Edit OpenSpec artifact"
        ></textarea>
        <button
          type="button"
          class="openspec-btn primary"
          data-testid="openspec-save-edit"
          onclick={finishOpenSpecEdit}
        >
          Save Draft
        </button>
      {:else}
        <pre
          class="openspec-preview"
          data-testid="openspec-preview">{effectiveOpenSpecMarkdown}</pre>
      {/if}
    </section>
  {:else if !args || sectionKeys.length === 0}
    <p class="artifact-empty">No additional fields.</p>
  {:else}
    <dl class="artifact-fields">
      {#each sectionKeys as key (key)}
        {@const value = args[key]}
        <div class="field" data-testid="artifact-field" data-field-key={key}>
          <dt class="field-label">{labelForKey(key)}</dt>
          <dd class="field-value">
            {#if typeof value === "string"}
              <!-- LLM-authored prose; renderMarkdown escapes first and emits
                   a fixed tag whitelist (XSS-safe). -->
              <!-- eslint-disable-next-line svelte/no-at-html-tags -->
              <div class="value-text">{@html renderMarkdown(value)}</div>
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

  .artifact-handoff {
    display: flex;
    flex-direction: column;
    gap: 0.375rem;
    margin-bottom: 0.625rem;
    padding: 0.5rem;
    border: 1px solid var(--ui-border-subtle, #e5e7eb);
    border-radius: 4px;
    background: var(--ui-surface-primary, #fff);
  }

  .artifact-handoff-eyebrow {
    margin: 0;
    font-size: 0.6875rem;
    font-weight: 700;
    color: var(--ui-text-primary, #111827);
  }

  .artifact-handoff-help {
    margin: 0.125rem 0 0;
    font-size: 0.75rem;
    line-height: 1.35;
    color: var(--ui-text-secondary, #6b7280);
  }

  .artifact-handoff-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.375rem;
  }

  .artifact-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 0.25rem;
    min-height: 1.75rem;
    padding: 0.25rem 0.5rem;
    border: 1px solid var(--ui-border-muted, #d1d5db);
    border-radius: 4px;
    background: var(--ui-surface-secondary, #f9fafb);
    color: var(--ui-text-primary, #111827);
    font: inherit;
    font-size: 0.75rem;
    line-height: 1.2;
    cursor: pointer;
    white-space: nowrap;
  }

  .artifact-btn:hover {
    background: var(--ui-surface-tertiary, #e5e7eb);
  }

  .artifact-status {
    margin: 0;
    font-size: 0.75rem;
    color: var(--ui-text-secondary, #6b7280);
  }

  .openspec-review {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .openspec-summary {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 0.75rem;
    flex-wrap: wrap;
    padding-bottom: 0.5rem;
    border-bottom: 1px solid var(--ui-border-subtle, #e5e7eb);
  }

  .openspec-eyebrow {
    margin: 0 0 0.125rem;
    font-size: 0.6875rem;
    color: var(--ui-text-secondary, #6b7280);
    letter-spacing: 0;
  }

  .openspec-title {
    margin: 0;
    font-size: 0.875rem;
    line-height: 1.3;
    color: var(--ui-text-primary, #111827);
    word-break: break-word;
  }

  .openspec-command {
    margin: 0;
    max-width: 100%;
    padding: 0.25rem 0.375rem;
    border: 1px solid var(--ui-border-subtle, #e5e7eb);
    border-radius: 4px;
    background: var(--ui-surface-primary, #fff);
    color: var(--ui-text-secondary, #6b7280);
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.6875rem;
    overflow-wrap: anywhere;
  }

  .openspec-actions,
  .openspec-export-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.375rem;
  }

  .openspec-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 0.25rem;
    min-height: 1.75rem;
    max-width: 100%;
    padding: 0.25rem 0.5rem;
    border: 1px solid var(--ui-border-muted, #d1d5db);
    border-radius: 4px;
    background: var(--ui-surface-primary, #fff);
    color: var(--ui-text-primary, #111827);
    font: inherit;
    font-size: 0.75rem;
    line-height: 1.2;
    cursor: pointer;
    white-space: nowrap;
  }

  .openspec-btn :global(svg) {
    flex: 0 0 auto;
  }

  .openspec-btn:hover {
    background: var(--ui-surface-tertiary, #e5e7eb);
  }

  .openspec-btn.primary {
    background: var(--ui-interactive-primary, #2563eb);
    border-color: var(--ui-interactive-primary, #2563eb);
    color: #fff;
  }

  .openspec-btn.danger {
    color: var(--ui-danger, #b91c1c);
    border-color: color-mix(in srgb, var(--ui-danger, #b91c1c) 35%, #fff);
  }

  .openspec-state {
    margin: 0;
    padding: 0.25rem 0.375rem;
    border-radius: 4px;
    background: var(--ui-surface-primary, #fff);
    border: 1px solid var(--ui-border-subtle, #e5e7eb);
    color: var(--ui-text-secondary, #6b7280);
    font-size: 0.75rem;
  }

  .openspec-state[data-state="approved"] {
    color: var(--ui-success, #047857);
  }

  .openspec-state[data-state="rejected"] {
    color: var(--ui-danger, #b91c1c);
  }

  .openspec-handoff {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
    flex-wrap: wrap;
    padding: 0.5rem;
    border: 1px solid #bfdbfe;
    border-radius: 4px;
    background: #eff6ff;
  }

  .handoff-copy {
    min-width: 0;
  }

  .handoff-title {
    margin: 0 0 0.25rem;
    font-size: 0.75rem;
    font-weight: 600;
    color: #1d4ed8;
  }

  .handoff-command {
    display: block;
    max-width: 100%;
    overflow-wrap: anywhere;
    padding: 0.1875rem 0.3125rem;
    border: 1px solid var(--ui-border-subtle, #e5e7eb);
    border-radius: 4px;
    background: var(--ui-surface-primary, #fff);
    color: var(--ui-text-primary, #111827);
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.6875rem;
  }

  .openspec-editor,
  .openspec-preview {
    width: 100%;
    border: 1px solid var(--ui-border-subtle, #e5e7eb);
    border-radius: 4px;
    background: var(--ui-surface-primary, #fff);
    color: var(--ui-text-primary, #111827);
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.75rem;
    line-height: 1.45;
  }

  .openspec-editor {
    min-height: 16rem;
    padding: 0.5rem;
    resize: vertical;
  }

  .openspec-preview {
    margin: 0;
    max-height: 24rem;
    padding: 0.625rem;
    overflow: auto;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
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
    word-break: break-word;
    line-height: 1.5;
  }

  /* renderMarkdown() output styles (injected via {@html}; :global needed). */
  .value-text :global(.md-p) {
    margin: 0 0 0.375rem;
  }
  .value-text :global(.md-p:last-child) {
    margin-bottom: 0;
  }
  .value-text :global(.md-h) {
    margin: 0.375rem 0 0.25rem;
    font-size: 0.8125rem;
    font-weight: 600;
  }
  .value-text :global(.md-list) {
    margin: 0 0 0.375rem;
    padding-left: 1.25rem;
  }
  .value-text :global(.md-quote) {
    margin: 0 0 0.375rem;
    padding-left: 0.5rem;
    border-left: 2px solid var(--ui-border-subtle, #e5e7eb);
    color: var(--ui-text-secondary, #6b7280);
  }
  .value-text :global(code) {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.8125em;
    background: var(--ui-surface-tertiary, #e5e7eb);
    padding: 0.0625rem 0.25rem;
    border-radius: 3px;
  }
  .value-text :global(.md-code) {
    margin: 0 0 0.375rem;
    padding: 0.5rem 0.625rem;
    background: var(--ui-surface-primary, #fff);
    border: 1px solid var(--ui-border-subtle, #e5e7eb);
    border-radius: 4px;
    overflow-x: auto;
    font-size: 0.75rem;
  }
  .value-text :global(.md-code code) {
    background: none;
    padding: 0;
  }
  .value-text :global(a) {
    color: var(--ui-interactive-primary, #2563eb);
    text-decoration: underline;
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
