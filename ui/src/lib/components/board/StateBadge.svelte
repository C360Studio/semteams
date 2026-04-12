<script lang="ts">
  import type { AgentLoopState } from "$lib/types/agent";

  interface Props {
    state: AgentLoopState;
  }

  let { state }: Props = $props();
</script>

<span class="state-badge {state}" data-state={state}>
  {state.replace(/_/g, " ")}
</span>

<style>
  .state-badge {
    padding: 0.0625rem 0.375rem;
    border-radius: 9999px;
    font-size: 0.6875rem;
    font-weight: 600;
    text-transform: capitalize;
    white-space: nowrap;
  }

  .state-badge.exploring,
  .state-badge.planning,
  .state-badge.architecting {
    background: #dbeafe;
    color: #1e40af;
  }

  .state-badge.executing,
  .state-badge.reviewing {
    background: #ccfbf1;
    color: #0f766e;
  }

  .state-badge.paused {
    background: #fef3c7;
    color: #92400e;
  }

  .state-badge.awaiting_approval {
    background: #ffedd5;
    color: #9a3412;
  }

  .state-badge.complete {
    background: #d1fae5;
    color: #065f46;
  }

  .state-badge.failed {
    background: #fee2e2;
    color: #991b1b;
  }

  .state-badge.cancelled {
    background: #f3f4f6;
    color: #6b7280;
  }
</style>
