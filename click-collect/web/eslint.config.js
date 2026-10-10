import js from "@eslint/js";
import ts from "typescript-eslint";
import svelte from "eslint-plugin-svelte";

// The upstream navigation rule currently gates execution to SvelteKit 1/2.
// Enforce the same project requirement explicitly for SvelteKit 3.
const navigationRule = {
  meta: {
    type: "problem",
    schema: [],
    messages: {
      resolve: "Use resolve() from $app/paths for internal navigation.",
    },
  },
  create(context) {
    function resolved(expression) {
      if (
        expression?.type !== "CallExpression" ||
        expression.callee.type !== "Identifier"
      ) {
        return false;
      }
      let scope = context.sourceCode.getScope(expression);
      while (scope) {
        const variable = scope.variables.find(
          (binding) => binding.name === expression.callee.name,
        );
        if (variable) {
          return variable.defs.some(
            (definition) =>
              definition.type === "ImportBinding" &&
              definition.node.type === "ImportSpecifier" &&
              definition.node.imported.name === "resolve" &&
              definition.parent.source.value === "$app/paths",
          );
        }
        scope = scope.upper;
      }
      return false;
    }
    return {
      SvelteAttribute(node) {
        const element = node.parent.parent;
        if (node.key.name !== "href" || element.name?.name !== "a") {
          return;
        }
        const value = node.value[0];
        if (
          value?.type === "SvelteLiteral" &&
          /^(?:https?:|mailto:|tel:|#)/.test(value.value)
        ) {
          return;
        }
        if (!resolved(value?.expression)) {
          context.report({ node, messageId: "resolve" });
        }
      },
      SvelteShorthandAttribute(node) {
        if (node.key.name === "href" && node.parent.parent.name?.name === "a") {
          context.report({ node, messageId: "resolve" });
        }
      },
      CallExpression(node) {
        if (
          node.callee.type === "Identifier" &&
          node.callee.name === "goto" &&
          !resolved(node.arguments[0])
        ) {
          context.report({ node, messageId: "resolve" });
        }
      },
    };
  },
};

const standards = {
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
    {
      selector: "TSAsExpression",
      message: "Use guards, annotations, or satisfies instead of casts.",
    },
    { selector: "TSTypeAssertion", message: "Use guards instead of casts." },
    {
      selector: "CallExpression[callee.property.name='forEach']",
      message: "Use for...of.",
    },
    {
      selector: "Identifier[name=/^_./]",
      message: "Remove unused bindings rather than prefixing them.",
    },
    {
      selector:
        "CallExpression[callee.name='it'] > Literal.arguments[value=/^(?!Should )/]",
      message: "Name tests Should ...",
    },
    {
      selector:
        "CallExpression[callee.callee.property.name='each'] > Literal.arguments[value=/^(?!Should )/]",
      message: "Name parameterized tests Should ...",
    },
  ],
};

export default ts.config(
  { ignores: [".svelte-kit/**", "build/**", "node_modules/**"] },
  js.configs.recommended,
  ts.configs.recommended,
  svelte.configs["flat/recommended"],
  {
    files: ["**/*.{ts,js,mjs,svelte}"],
    plugins: { project: { rules: { "resolved-navigation": navigationRule } } },
    languageOptions: { globals: { process: "readonly" } },
    rules: { ...standards, "project/resolved-navigation": "error" },
  },
  {
    files: ["**/*.svelte"],
    languageOptions: { parserOptions: { parser: ts.parser } },
    rules: {
      "svelte/require-each-key": "error",
      "svelte/no-navigation-without-resolve": "error",
    },
  },
  {
    files: ["src/**/*.test.{ts,js}"],
    rules: {
      "no-restricted-syntax": [
        "error",
        {
          selector: "Program",
          message: "Move tests to tests/unit or tests/browser.",
        },
      ],
    },
  },
);
