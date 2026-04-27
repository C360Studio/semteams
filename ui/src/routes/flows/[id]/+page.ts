import { redirect } from "@sveltejs/kit";
import type { PageLoad } from "./$types";

// The flow editor was removed in favor of agent-authored flows
// (coordinator edits flows; humans approve/reject). Redirect any old
// per-flow bookmarks back to the inventory list.
export const load: PageLoad = () => {
  redirect(308, "/admin/flows");
};
