import { redirect } from "@sveltejs/kit";
import type { PageLoad } from "./$types";

// /flows/[id] moved to /admin/flows/[id] in step 8.
export const load: PageLoad = ({ params }) => {
  redirect(308, `/admin/flows/${params.id}`);
};
