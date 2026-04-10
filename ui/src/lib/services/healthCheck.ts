// Backend health check service
// Provides utilities to check backend connectivity and health

export interface HealthCheckResult {
  healthy: boolean;
  message: string;
  statusCode?: number;
}

/**
 * Check if backend is reachable and healthy
 * Returns a user-friendly result object
 */
export async function checkBackendHealth(): Promise<HealthCheckResult> {
  try {
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), 5000); // 5 second timeout

    const response = await fetch("/health", {
      signal: controller.signal,
      cache: "no-store",
    });
    clearTimeout(timeoutId);

    if (response.ok) {
      return {
        healthy: true,
        message: "Backend is healthy",
        statusCode: response.status,
      };
    }

    return {
      healthy: false,
      message: `Backend returned status ${response.status}: ${response.statusText}`,
      statusCode: response.status,
    };
  } catch (error) {
    if (error instanceof Error) {
      if (error.name === "AbortError") {
        return {
          healthy: false,
          message:
            "Backend health check timed out. The backend may be down or unreachable.",
        };
      }

      if (
        error.message.includes("Failed to fetch") ||
        error.message.includes("NetworkError")
      ) {
        return {
          healthy: false,
          message:
            "Cannot connect to backend. Please ensure the backend service is running.",
        };
      }
    }

    return {
      healthy: false,
      message: `Health check failed: ${error instanceof Error ? error.message : "Unknown error"}`,
    };
  }
}

/**
 * Helper to detect if an error is a network/connectivity issue
 * vs an actual API error response
 */
export function isConnectivityError(error: unknown): boolean {
  if (!(error instanceof Error)) {
    return false;
  }

  return (
    error.name === "AbortError" ||
    error.message.includes("Failed to fetch") ||
    error.message.includes("NetworkError") ||
    error.message.includes("network") ||
    error.message.includes("ECONNREFUSED")
  );
}

/**
 * Extract user-friendly error message from fetch errors
 */
export function getUserFriendlyErrorMessage(error: unknown): string {
  if (isConnectivityError(error)) {
    return "Cannot connect to backend. Please ensure the backend service is running and accessible.";
  }

  if (error instanceof Error) {
    return error.message;
  }

  return "An unexpected error occurred";
}
