import { json } from "@sveltejs/kit";
import type { WorkResult } from "./service";

/** Work data is live state: never cache it. */
export function respond<T>(result: WorkResult<T>): Response {
  return json(result.body, {
    status: result.status,
    headers: { "Cache-Control": "no-store" },
  });
}
