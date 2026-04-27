import { redirect } from "@sveltejs/kit";
import type { PageLoad } from "./$types";

// /flows moved to /admin/flows in step 8 of docs/proposals/ui-redesign.md.
// 308 (permanent redirect) so old bookmarks update on first hit.
export const load: PageLoad = () => {
  redirect(308, "/admin/flows");
};
