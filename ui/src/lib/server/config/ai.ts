/**
 * AI Configuration
 *
 * Centralized configuration for AI flow generation.
 * All AI-related settings should be defined here.
 */

/**
 * Prompt validation configuration
 */
export const PROMPT_CONFIG = {
  /** Minimum prompt length in characters */
  minLength: 10,
  /** Maximum prompt length in characters */
  maxLength: 2000,
  /** Warning threshold (percentage of max length) */
  warningThreshold: 0.9,
};

/**
 * Rate limiting configuration
 */
export const RATE_LIMIT_CONFIG = {
  /** Maximum requests per window */
  maxRequests: 10,
  /** Window duration in milliseconds */
  windowMs: 60000, // 1 minute
};

/**
 * Claude API configuration
 */
export const CLAUDE_CONFIG = {
  /** Default model to use */
  defaultModel: "claude-sonnet-4-20250514",
  /** Maximum tokens for response */
  maxTokens: 4096,
  /** Request timeout in milliseconds */
  timeout: 30000,
  /** Maximum retry attempts */
  maxRetries: 3,
  /** Base delay for retry backoff in milliseconds */
  retryDelayMs: 1000,
};

/**
 * Cache configuration
 */
export const CACHE_CONFIG = {
  /** Component catalog cache TTL in milliseconds */
  componentCatalogTTL: 300000, // 5 minutes
};

/**
 * Validate prompt and return validation result
 *
 * @param prompt The prompt to validate
 * @returns Validation result with error message if invalid
 */
export function validatePrompt(prompt: unknown): {
  valid: boolean;
  error?: string;
  trimmedPrompt?: string;
} {
  // Check type
  if (!prompt || typeof prompt !== "string") {
    return {
      valid: false,
      error: "Prompt is required and must be a string",
    };
  }

  const trimmedPrompt = prompt.trim();

  // Check empty
  if (trimmedPrompt.length === 0) {
    return {
      valid: false,
      error: "Prompt cannot be empty",
    };
  }

  // Check minimum length
  if (trimmedPrompt.length < PROMPT_CONFIG.minLength) {
    return {
      valid: false,
      error: `Prompt is too short. Minimum ${PROMPT_CONFIG.minLength} characters required. Try being more specific about the flow you want to create.`,
    };
  }

  // Check maximum length
  if (trimmedPrompt.length > PROMPT_CONFIG.maxLength) {
    return {
      valid: false,
      error: `Prompt is too long. Maximum ${PROMPT_CONFIG.maxLength} characters allowed. Try breaking down your request into smaller, more focused prompts.`,
    };
  }

  return {
    valid: true,
    trimmedPrompt,
  };
}

/**
 * Error messages for user-facing errors
 */
export const ERROR_MESSAGES = {
  rateLimitExceeded: (retryAfter: number) =>
    `Too many requests. Please wait ${retryAfter} seconds before trying again.`,

  promptTooShort: `Your prompt is too short. Please provide more details about the flow you want to create. For example: "Create a flow that receives UDP data on port 5000, transforms it to JSON, and publishes to NATS."`,

  promptTooLong: `Your prompt is too long. Please try to be more concise or break down your request into smaller steps.`,

  invalidPrompt: `Invalid prompt. Please describe the flow you want to create using natural language.`,

  componentCatalogFailed:
    "Unable to fetch available components. Please check if the backend is running and try again.",

  claudeApiFailed:
    "AI generation failed. This might be a temporary issue. Please try again in a moment.",

  claudeRateLimited:
    "AI service is currently busy. Please wait a moment and try again.",

  validationFailed:
    "Flow validation failed. The generated flow may have issues. Please review and try again with a modified prompt.",

  noFlowGenerated:
    "The AI could not generate a flow from your prompt. Please try rephrasing your request with more specific details.",

  internalError:
    "An unexpected error occurred. Please try again. If the problem persists, contact support.",

  requestTimeout:
    "The request took too long to complete. Please try again with a simpler prompt.",

  requestCancelled: "The request was cancelled.",
};
