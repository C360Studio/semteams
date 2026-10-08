<script lang="ts">
  import type { Snippet } from "svelte";
  import type { Program } from "$lib/types/work";
  import type { RepositoryBoard } from "$lib/stores/workStore.svelte";
  import { slug } from "$lib/utils/workView";
  import LookupNotice from "./LookupNotice.svelte";

  interface Props {
    programs: Program[];
    boards: Record<string, RepositoryBoard>;
    /** Test id of the whole lens: `work-board` or `work-table`. */
    testid: string;
    /** The body of one repository section, rendered once its items have been read. */
    repository: Snippet<[RepositoryBoard]>;
  }

  let { programs, boards, testid, repository }: Props = $props();
</script>

<!--
  program -> project -> repository. Membership is operator-authored, so a
  repository can appear under more than one project; each appearance is its own
  section.
-->
<div class="portfolio" data-testid={testid}>
  {#each programs as program (program.id)}
    <section class="program" aria-labelledby="program-{slug(testid)}-{slug(program.id)}">
      <h2 id="program-{slug(testid)}-{slug(program.id)}">{program.name}</h2>

      {#each program.projects as project (project.id)}
        {@const projectKey = `${slug(testid)}-${slug(program.id)}-${slug(project.id)}`}
        <section class="project" aria-labelledby="project-{projectKey}">
          <h3 id="project-{projectKey}">{project.name}</h3>

          {#each project.repositories as repo (`${repo.owner}/${repo.name}`)}
            {@const id = `${repo.owner}/${repo.name}`}
            {@const board = boards[id]}
            <section
              class="repository"
              data-testid="work-repository-{repo.owner}-{repo.name}"
              aria-labelledby="repository-{projectKey}-{slug(id)}"
            >
              <h4 id="repository-{projectKey}-{slug(id)}">{id}</h4>
              {#if !board || board.loading}
                <p class="loading" role="status">Loading items&hellip;</p>
              {:else}
                <LookupNotice
                  lookup={board.lookup}
                  reason={board.reason}
                  subject="Items"
                  testid="work-lookup"
                />
                {@render repository(board)}
              {/if}
            </section>
          {/each}
        </section>
      {/each}
    </section>
  {/each}
</div>

<style>
  .portfolio {
    display: flex;
    flex-direction: column;
    gap: 1.5rem;
  }

  .program > h2 {
    margin: 0 0 0.5rem;
    font-size: 1.125rem;
  }

  .project {
    margin-bottom: 1rem;
  }

  .project > h3 {
    margin: 0 0 0.5rem;
    font-size: 1rem;
    color: var(--ui-text-secondary, #6b7280);
  }

  .repository {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    margin-bottom: 1rem;
  }

  .repository > h4 {
    margin: 0;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.875rem;
  }

  .loading {
    margin: 0;
    font-size: 0.8125rem;
    color: var(--ui-text-secondary, #6b7280);
  }
</style>
