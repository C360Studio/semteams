export interface ChatArtifactContext {
  id: string;
  title: string;
  toolName: string;
  content: string;
}

export interface ChatDraft {
  id: number;
  content: string;
}

interface StageHandoffInput {
  content: string;
  artifact?: ChatArtifactContext;
}

function createChatHandoffStore() {
  let draft = $state<ChatDraft | null>(null);
  let artifactContext = $state<ChatArtifactContext | null>(null);
  let nextDraftID = 0;

  return {
    get draft() {
      return draft;
    },

    get artifactContext() {
      return artifactContext;
    },

    stage(input: StageHandoffInput) {
      if (input.artifact) artifactContext = input.artifact;
      draft = {
        id: ++nextDraftID,
        content: input.content,
      };
    },

    attachArtifact(artifact: ChatArtifactContext) {
      artifactContext = artifact;
    },

    consumeDraft(id: number) {
      if (draft?.id === id) draft = null;
    },

    clearArtifact() {
      artifactContext = null;
    },

    clearAll() {
      draft = null;
      artifactContext = null;
    },
  };
}

export const chatHandoff = createChatHandoffStore();
