import type { PageLoad } from "./$types";
import {
  isConnectivityError,
  getUserFriendlyErrorMessage,
} from "$lib/services/healthCheck";

export const load: PageLoad = async ({ fetch }) => {
  try {
    const response = await fetch("/flowbuilder/flows");
    if (!response.ok) {
      const contentType = response.headers.get("content-type");
      if (contentType?.includes("text/html")) {
        throw new Error(
          "Backend service unavailable (received HTML error page)",
        );
      }
      throw new Error(`Failed to load flows: ${response.statusText}`);
    }
    const data = await response.json();
    return {
      flows: data.flows || [],
    };
  } catch (error) {
    console.error("Failed to load flows:", error);

    let errorMessage: string;
    if (isConnectivityError(error)) {
      errorMessage = "Cannot connect to backend service";
    } else {
      errorMessage = getUserFriendlyErrorMessage(error);
    }

    return {
      flows: [],
      error: errorMessage,
    };
  }
};
