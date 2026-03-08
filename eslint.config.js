import js from "@eslint/js";
import tseslint from "@typescript-eslint/eslint-plugin";
import tsparser from "@typescript-eslint/parser";
import svelte from "eslint-plugin-svelte";
import prettier from "eslint-config-prettier";
import globals from "globals";

export default [
  js.configs.recommended,
  ...svelte.configs["flat/recommended"],
  {
    files: ["**/*.svelte"],
    languageOptions: {
      parser: svelte.parser,
      parserOptions: {
        parser: tsparser,
      },
      globals: {
        ...globals.browser,
      },
    },
    plugins: {
      "@typescript-eslint": tseslint,
    },
    rules: {
      "no-unused-vars": "off", // Disable base rule in favor of TypeScript-aware version
      "@typescript-eslint/no-unused-vars": [
        "error",
        { argsIgnorePattern: "^_", varsIgnorePattern: "^_" },
      ],
    },
  },
  {
    files: ["**/*.{ts,tsx}", "**/*.svelte.ts"],
    languageOptions: {
      parser: tsparser,
      parserOptions: {
        ecmaVersion: 2022,
        sourceType: "module",
        extraFileExtensions: [".svelte"],
      },
      globals: {
        ...globals.browser,
        ...globals.node,
        // Svelte 5 runes — available as globals in .svelte.ts files
        $state: "readonly",
        $derived: "readonly",
        $effect: "readonly",
        $props: "readonly",
        $bindable: "readonly",
        $inspect: "readonly",
        $host: "readonly",
      },
    },
    plugins: {
      "@typescript-eslint": tseslint,
    },
    rules: {
      ...tseslint.configs.recommended.rules,
      "no-unused-vars": "off", // Disable base rule in favor of @typescript-eslint version
      "@typescript-eslint/no-unused-vars": [
        "error",
        { argsIgnorePattern: "^_", varsIgnorePattern: "^_" },
      ],
      "@typescript-eslint/no-explicit-any": "warn",
    },
  },
  {
    files: ["e2e/**/*.ts", "**/*.test.ts", "**/*.spec.ts"],
    languageOptions: {
      globals: {
        ...globals.browser,
        ...globals.node,
      },
    },
  },
  // Locked generated test files — DO NOT EDIT headers mean we cannot fix
  // unused imports in them. Suppress unused-vars for these files only.
  {
    files: [
      "src/lib/components/DataTable.test.ts",
      "src/lib/components/JsonEditor.test.ts",
      "src/lib/components/PropertiesPanel.attack.test.ts",
      "src/lib/components/runtime/NlqDebugBadge.test.ts",
    ],
    rules: {
      "@typescript-eslint/no-unused-vars": "off",
      "@typescript-eslint/no-explicit-any": "off",
    },
  },
  // chatApi.test.ts uses RequestInit as a TypeScript type annotation; the
  // no-undef rule fires because the test-file override block does not use the
  // TypeScript parser. Suppress no-undef for this locked generated file.
  {
    files: ["src/lib/services/chatApi.test.ts"],
    rules: {
      "no-undef": "off",
    },
  },
  prettier,
  {
    ignores: [
      ".svelte-kit/**",
      "build/**",
      "dist/**",
      "node_modules/**",
      "**/*.cjs",
      "vite.config.ts.timestamp-*",
    ],
  },
];
