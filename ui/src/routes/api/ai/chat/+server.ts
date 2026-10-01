import { json } from "@sveltejs/kit";
import type { RequestHandler } from "./$types";

// Flow authoring was removed upstream. Reject before reading input or creating
// any model/backend client; admitted composition is visible at /admin/flows.
export const POST: RequestHandler = async () => json({
  code: "FLOW_AUTHORING_RETIRED",
  error: "Runtime flow authoring is no longer available. View the configured composition at /admin/flows.",
}, { status: 410 });
