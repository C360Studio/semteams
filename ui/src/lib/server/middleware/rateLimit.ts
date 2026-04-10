/**
 * Rate Limiter Middleware
 *
 * Simple in-memory rate limiter for API endpoints.
 * Uses sliding window algorithm for accurate rate limiting.
 *
 * For production, consider using Redis-based rate limiting
 * for multi-instance deployments.
 */

/**
 * Rate limit configuration
 */
export interface RateLimitConfig {
  /** Maximum requests allowed in the window */
  maxRequests: number;
  /** Time window in milliseconds */
  windowMs: number;
}

/**
 * Rate limit result
 */
export interface RateLimitResult {
  /** Whether the request is allowed */
  allowed: boolean;
  /** Remaining requests in the current window */
  remaining: number;
  /** Time in seconds until the rate limit resets */
  retryAfter?: number;
  /** Total limit for the window */
  limit: number;
}

/**
 * Request record for tracking
 */
interface RequestRecord {
  timestamps: number[];
}

/**
 * In-memory store for rate limit tracking
 * Key is client identifier (IP address or user ID)
 */
const store = new Map<string, RequestRecord>();

/**
 * Cleanup interval handle
 */
let cleanupInterval: ReturnType<typeof setInterval> | null = null;

/**
 * Start cleanup interval if not already running
 */
function ensureCleanupRunning(windowMs: number): void {
  if (cleanupInterval) return;

  // Cleanup old entries every minute
  cleanupInterval = setInterval(
    () => {
      const now = Date.now();
      for (const [key, record] of store.entries()) {
        // Remove timestamps older than the window
        record.timestamps = record.timestamps.filter(
          (ts) => now - ts < windowMs,
        );
        // Remove empty records
        if (record.timestamps.length === 0) {
          store.delete(key);
        }
      }
    },
    60000, // 1 minute
  );
}

/**
 * Check rate limit for a client
 *
 * Uses sliding window algorithm:
 * - Tracks timestamps of all requests in the window
 * - Filters out expired timestamps
 * - Counts requests in current window
 *
 * @param clientId Client identifier (IP, user ID, etc.)
 * @param config Rate limit configuration
 * @returns Rate limit result
 */
export function checkRateLimit(
  clientId: string,
  config: RateLimitConfig,
): RateLimitResult {
  const now = Date.now();
  const { maxRequests, windowMs } = config;

  ensureCleanupRunning(windowMs);

  // Get or create record for client
  let record = store.get(clientId);
  if (!record) {
    record = { timestamps: [] };
    store.set(clientId, record);
  }

  // Filter out expired timestamps
  record.timestamps = record.timestamps.filter((ts) => now - ts < windowMs);

  // Check if limit exceeded
  if (record.timestamps.length >= maxRequests) {
    // Calculate when the oldest request will expire
    const oldestTimestamp = record.timestamps[0];
    const retryAfter = Math.ceil((oldestTimestamp + windowMs - now) / 1000);

    return {
      allowed: false,
      remaining: 0,
      retryAfter,
      limit: maxRequests,
    };
  }

  // Add current request timestamp
  record.timestamps.push(now);

  return {
    allowed: true,
    remaining: maxRequests - record.timestamps.length,
    limit: maxRequests,
  };
}

/**
 * Reset rate limit for a client
 *
 * Useful for testing or admin overrides.
 *
 * @param clientId Client identifier
 */
export function resetRateLimit(clientId: string): void {
  store.delete(clientId);
}

/**
 * Clear all rate limit records
 *
 * Useful for testing.
 */
export function clearAllRateLimits(): void {
  store.clear();
}

/**
 * Get current rate limit status for a client without incrementing
 *
 * @param clientId Client identifier
 * @param config Rate limit configuration
 * @returns Rate limit status
 */
export function getRateLimitStatus(
  clientId: string,
  config: RateLimitConfig,
): RateLimitResult {
  const now = Date.now();
  const { maxRequests, windowMs } = config;

  const record = store.get(clientId);
  if (!record) {
    return {
      allowed: true,
      remaining: maxRequests,
      limit: maxRequests,
    };
  }

  // Filter out expired timestamps (don't modify the record)
  const validTimestamps = record.timestamps.filter((ts) => now - ts < windowMs);
  const remaining = Math.max(0, maxRequests - validTimestamps.length);

  if (remaining === 0 && validTimestamps.length > 0) {
    const oldestTimestamp = validTimestamps[0];
    const retryAfter = Math.ceil((oldestTimestamp + windowMs - now) / 1000);

    return {
      allowed: false,
      remaining: 0,
      retryAfter,
      limit: maxRequests,
    };
  }

  return {
    allowed: true,
    remaining,
    limit: maxRequests,
  };
}

/**
 * Default rate limit configuration for AI endpoints
 */
export const AI_RATE_LIMIT_CONFIG: RateLimitConfig = {
  maxRequests: 10,
  windowMs: 60000, // 1 minute
};
