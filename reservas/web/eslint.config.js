import js from "@eslint/js";
import ts from "typescript-eslint";
import svelte from "eslint-plugin-svelte";

export default ts.config(
  { ignores: [".svelte-kit/**", "build/**", "node_modules/**"] },
  js.configs.recommended,
  ts.configs.recommended,
  svelte.configs["flat/recommended"],
  {
    files: ["**/*.{ts,js,mjs,svelte}"],
    languageOptions: { globals: { process: "readonly" } },
    rules: {
      curly: ["error", "all"],
      "@typescript-eslint/no-unused-vars": "error",
      "@typescript-eslint/no-non-null-assertion": "error",
      "no-restricted-syntax": [
        "error",
        {
          selector: "Literal[raw='null']",
          message: "Use undefined or optional fields.",
        },
        {
          selector: "TSNullKeyword",
          message: "Use optional types instead of null.",
        },
        { selector: "TSAsExpression", message: "Use guards instead of casts." },
        {
          selector: "TSTypeAssertion",
          message: "Use guards instead of casts.",
        },
        {
          selector: "CallExpression[callee.property.name='forEach']",
          message: "Use for...of.",
        },
        {
          selector: "Identifier[name=/^_./]",
          message: "Remove unused bindings rather than prefixing them.",
        },
      ],
    },
  },
  {
    files: ["**/*.svelte"],
    languageOptions: { parserOptions: { parser: ts.parser } },
    rules: { "svelte/require-each-key": "error" },
  },
);
