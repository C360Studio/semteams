import { getComposition } from "$lib/services/compositionApi";
import type { PageLoad } from "./$types";
import {
  isConnectivityError,
  getUserFriendlyErrorMessage,
} from "$lib/services/healthCheck";

export const load: PageLoad = async ({ fetch }) => {
  try {
    const { graph, validation } = await getComposition(fetch);
    return { components: graph.nodes, validation };
  } catch (error) {
    console.error("Failed to load flows:", error);

    let errorMessage: string;
    if (isConnectivityError(error)) {
      errorMessage = "Cannot connect to backend service";
    } else {
      errorMessage = getUserFriendlyErrorMessage(error);
    }

    return {
      components: [],
      validation: null,
      error: errorMessage,
    };
  }
};
