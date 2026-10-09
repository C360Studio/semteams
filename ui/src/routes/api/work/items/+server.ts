import type { RequestHandler } from "./$types";
import { respond } from "$lib/server/work/respond";
import { getItems } from "$lib/server/work/service";

// GET only (see ../portfolio/+server.ts).
export const GET: RequestHandler = async ({ url }) =>
  respond(await getItems(url.searchParams.get("repository")));
