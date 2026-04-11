import adapter from "@sveltejs/adapter-node";
import { vitePreprocess } from "@sveltejs/vite-plugin-svelte";

/** @type {import('@sveltejs/kit').Config} */
const config = {
  // Consult https://svelte.dev/docs/kit/integrations
  // for more information about preprocessors
  preprocess: vitePreprocess(),

  kit: {
    // Build Node.js server for deployment behind Caddy reverse proxy
    // Enables server-side features: API routes, hooks, MCP server
    adapter: adapter({
      out: "build",
      precompress: false,
    }),
  },
};

export default config;
