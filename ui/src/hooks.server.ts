import type { Handle } from "@sveltejs/kit";
import { env } from "$env/dynamic/private";

/**
 * handleFetch hook transforms URLs during server-side rendering.
 * This allows us to use relative URLs in load functions while SSR
 * can reach the backend service through the Docker network.
 */
export const handle: Handle = async ({ event, resolve }) => {
  return resolve(event);
};

/**
 * Transform fetch requests during SSR to use backend service URL.
 * Only applies when running in Docker (BACKEND_HOST is set).
 *
 * When running E2E tests, SSR is disabled via +layout.ts,
 * so client-side fetches use the Vite proxy instead.
 */
export async function handleFetch({ request, fetch }) {
  const url = new URL(request.url);
  const startTime = Date.now();

  // Transform relative backend API URLs to use backend service
  if (
    url.pathname.startsWith("/flowbuilder") ||
    url.pathname.startsWith("/components") ||
    url.pathname.startsWith("/health") ||
    url.pathname.startsWith("/graphql")
  ) {
    // BACKEND_HOST is set in docker-compose.dev.yml (e.g., "backend:8080")
    // If not set, we're probably in E2E tests (which disable SSR anyway)
    const backendHost = env.BACKEND_HOST;

    if (backendHost) {
      url.protocol = "http:";
      url.host = backendHost;

      console.log(
        `[handleFetch] Transforming ${request.method} ${request.url} → ${url.toString()}`,
      );

      const modifiedRequest = new Request(url.toString(), {
        method: request.method,
        headers: request.headers,
        body: request.body,
        // @ts-expect-error - duplex is valid but not in TypeScript types yet
        duplex: "half",
      });

      const response = await fetch(modifiedRequest);
      const elapsed = Date.now() - startTime;
      console.log(
        `[handleFetch] ${request.method} ${url.pathname} completed in ${elapsed}ms (status: ${response.status})`,
      );
      return response;
    }
  }

  // All other requests pass through unchanged
  return fetch(request);
}
