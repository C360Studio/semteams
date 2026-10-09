import type { RequestHandler } from "./$types";
import { respond } from "$lib/server/work/respond";
import { getPortfolio } from "$lib/server/work/service";

// GET only: the work board is read-only, so no other method is exported and
// SvelteKit answers 405 for them.
export const GET: RequestHandler = async () => respond(await getPortfolio());
