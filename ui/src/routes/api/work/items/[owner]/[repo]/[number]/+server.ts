import type { RequestHandler } from "./$types";
import { respond } from "$lib/server/work/respond";
import { getItem } from "$lib/server/work/service";

// GET only (see ../../../../portfolio/+server.ts).
export const GET: RequestHandler = async ({ params }) =>
  respond(await getItem(params.owner, params.repo, params.number));
