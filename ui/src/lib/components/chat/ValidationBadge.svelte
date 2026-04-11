<script lang="ts">
  interface Props {
    status: "valid" | "warnings" | "errors";
    warningCount?: number;
    errorCount?: number;
  }

  let { status, warningCount, errorCount }: Props = $props();
</script>

<span data-testid="validation-badge" aria-label={status === "valid" ? "valid" : undefined}>
  {#if status === "valid"}
    Valid
  {:else if status === "warnings"}
    {warningCount ?? 0} warning{(warningCount ?? 0) === 1 ? "" : "s"}
  {:else if status === "errors"}
    {errorCount ?? 0} error{(errorCount ?? 0) === 1 ? "" : "s"}
  {/if}
</span>
