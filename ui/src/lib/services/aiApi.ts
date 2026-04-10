/**
 * AI API Service
 *
 * Client-side API service for AI-powered flow generation.
 * Provides methods for generating flows from natural language prompts.
 */

import type { Flow } from "$lib/types/flow";
import type { ValidationResult } from "$lib/types/validation";

/**
 * Response from AI flow generation endpoints
 */
export interface GenerateFlowResponse {
  flow: Flow;
  validationResult: ValidationResult;
}

/**
 * Custom error class for AI API errors
 */
export class AiApiError extends Error {
  name = "AiApiError";

  constructor(
    message: string,
    public statusCode?: number,
    public details?: any,
  ) {
    super(message);
    Object.setPrototypeOf(this, AiApiError.prototype);
  }
}

/**
 * Options for AI API requests
 */
export interface AiApiOptions {
  /** AbortSignal for request cancellation */
  signal?: AbortSignal;
}

/**
 * AI API client interface
 */
export interface AiApi {
  generateFlow(
    prompt: string,
    existingFlow?: Flow,
    options?: AiApiOptions,
  ): Promise<GenerateFlowResponse>;
  streamGenerateFlow(
    prompt: string,
    existingFlow?: Flow,
    onProgress?: (chunk: string) => void,
    options?: AiApiOptions,
  ): Promise<GenerateFlowResponse>;
}

/**
 * Generate flow from natural language prompt
 *
 * @param prompt Natural language description of the desired flow
 * @param existingFlow Optional existing flow to modify
 * @param options Optional request options (including AbortSignal for cancellation)
 * @returns Generated flow and validation result
 * @throws AiApiError if the request fails
 * @throws DOMException with name 'AbortError' if the request is cancelled
 */
async function generateFlow(
  prompt: string,
  existingFlow?: Flow,
  options?: AiApiOptions,
): Promise<GenerateFlowResponse> {
  const trimmedPrompt = prompt.trim();

  const requestBody: { prompt: string; existingFlow?: Flow } = {
    prompt: trimmedPrompt,
  };

  if (existingFlow) {
    requestBody.existingFlow = existingFlow;
  }

  try {
    const response = await fetch("/ai/generate-flow", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(requestBody),
      signal: options?.signal,
    });

    if (!response.ok) {
      let errorDetails: any = {};
      try {
        errorDetails = await response.json();
      } catch {
        // If JSON parsing fails, use empty object
      }

      throw new AiApiError(
        `Failed to generate flow: ${response.statusText}`,
        response.status,
        errorDetails,
      );
    }

    const result = await response.json();
    return result as GenerateFlowResponse;
  } catch (error) {
    if (error instanceof AiApiError) {
      throw error;
    }
    // Re-throw AbortError as-is for proper handling
    if (error instanceof DOMException && error.name === "AbortError") {
      throw error;
    }
    throw error;
  }
}

/**
 * Generate flow with streaming progress updates
 *
 * @param prompt Natural language description of the desired flow
 * @param existingFlow Optional existing flow to modify
 * @param onProgress Callback for progress updates
 * @param options Optional request options (including AbortSignal for cancellation)
 * @returns Generated flow and validation result
 * @throws AiApiError if the request fails
 * @throws DOMException with name 'AbortError' if the request is cancelled
 */
async function streamGenerateFlow(
  prompt: string,
  existingFlow?: Flow,
  onProgress?: (chunk: string) => void,
  options?: AiApiOptions,
): Promise<GenerateFlowResponse> {
  const trimmedPrompt = prompt.trim();

  const requestBody: { prompt: string; existingFlow?: Flow } = {
    prompt: trimmedPrompt,
  };

  if (existingFlow) {
    requestBody.existingFlow = existingFlow;
  }

  try {
    const response = await fetch("/ai/stream-generate-flow", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(requestBody),
      signal: options?.signal,
    });

    if (!response.ok) {
      let errorDetails: any = {};
      try {
        errorDetails = await response.json();
      } catch {
        // If JSON parsing fails, use empty object
      }

      throw new AiApiError(
        `Failed to generate flow: ${response.statusText}`,
        response.status,
        errorDetails,
      );
    }

    if (!response.body) {
      throw new Error("Response body is null");
    }

    // Read the stream
    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";
    let finalResult: GenerateFlowResponse | null = null;

    // Set up abort handler for stream reader
    const abortHandler = () => {
      reader.cancel().catch(() => {
        // Ignore cancel errors
      });
    };
    options?.signal?.addEventListener("abort", abortHandler);

    try {
      while (true) {
        const { done, value } = await reader.read();

        if (done) {
          break;
        }

        // Check if aborted during stream read
        if (options?.signal?.aborted) {
          throw new DOMException("Request was cancelled", "AbortError");
        }

        // Decode the chunk
        const chunk = decoder.decode(value, { stream: true });
        buffer += chunk;

        // Process complete SSE messages
        const lines = buffer.split("\n\n");
        buffer = lines[lines.length - 1]; // Keep incomplete message in buffer

        for (let i = 0; i < lines.length - 1; i++) {
          const line = lines[i].trim();
          if (line.startsWith("data: ")) {
            const data = line.slice(6); // Remove 'data: ' prefix

            // Try to parse as JSON (final result)
            try {
              const parsed = JSON.parse(data);
              if (parsed.flow && parsed.validationResult) {
                finalResult = parsed;
              } else {
                // Progress update
                if (onProgress) {
                  onProgress(data);
                }
              }
            } catch {
              // Not JSON, just a progress message
              if (onProgress) {
                onProgress(data);
              }
            }
          }
        }
      }
    } finally {
      options?.signal?.removeEventListener("abort", abortHandler);
      reader.releaseLock();
    }

    if (!finalResult) {
      throw new Error("No final result received from stream");
    }

    return finalResult;
  } catch (error) {
    if (error instanceof AiApiError) {
      throw error;
    }
    // Re-throw AbortError as-is for proper handling
    if (error instanceof DOMException && error.name === "AbortError") {
      throw error;
    }
    throw error;
  }
}

/**
 * AI API client instance
 */
export const aiApi: AiApi = {
  generateFlow,
  streamGenerateFlow,
};
