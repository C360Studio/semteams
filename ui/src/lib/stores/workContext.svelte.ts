// Work-scoped prompt context store (OpenSpec work-inline-agentic-drawer, design D2).
//
// Syncs the currently selected work item on `/work` (and its Program / Project
// lineage from the configured portfolio) so `ChatBar` can display a scoped
// context chip and attach `[Work item: owner/repo#n — <title>]` to outbound
// coordinator prompts without manual copy-paste.

import type { Program, WorkItem } from "$lib/types/work";

export interface WorkChatScope {
  itemRef: string;
  title: string;
  repository: string;
  programName?: string;
  projectName?: string;
}

export function resolveWorkScope(item: WorkItem, programs: Program[]): WorkChatScope {
  for (const program of programs) {
    for (const project of program.projects) {
      for (const repo of project.repositories) {
        if (`${repo.owner}/${repo.name}` === item.repository) {
          return {
            itemRef: item.ref,
            title: item.title,
            repository: item.repository,
            programName: program.name,
            projectName: project.name,
          };
        }
      }
    }
  }
  return {
    itemRef: item.ref,
    title: item.title,
    repository: item.repository,
  };
}

export function formatWorkScopeHeader(scope: WorkChatScope): string {
  const lineage =
    scope.programName && scope.projectName
      ? ` (${scope.programName} / ${scope.projectName})`
      : "";
  return `[Work item: ${scope.itemRef}${lineage} — ${scope.title}]`;
}

function createWorkContextStore() {
  let scope = $state<WorkChatScope | null>(null);

  return {
    get scope(): WorkChatScope | null {
      return scope;
    },

    setScope(next: WorkChatScope | null): void {
      scope = next;
    },

    clear(): void {
      scope = null;
    },
  };
}

export const workContext = createWorkContextStore();
