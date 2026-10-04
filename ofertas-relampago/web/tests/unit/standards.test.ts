import { ESLint } from "eslint";
import { describe, expect, it } from "vitest";

const eslint = new ESLint();

describe("TypeScript standards enforcement", () => {
  it.each([
    "export const missing = null;",
    "export const value = input as string;",
    "export const value = <string>input;",
    "export type Missing = string | null;",
    "export const value = input!;",
    "items.forEach(consume);",
    "if (ready) consume();",
    "export function consume(_unused: string) {}",
    'it("accepts invalid input", () => {});',
    'it.each([1])("accepts %s", () => {});',
  ])("Should reject forbidden syntax: %s", async (code) => {
    const results = await eslint.lintText(code, {
      filePath: "tests/unit/fixture.ts",
    });
    expect(results.flatMap((result) => result.messages).length).toBeGreaterThan(
      0,
    );
    expect(
      results
        .flatMap((result) => result.messages)
        .some((message) =>
          [
            "no-restricted-syntax",
            "curly",
            "@typescript-eslint/no-non-null-assertion",
          ].includes(message.ruleId ?? ""),
        ),
    ).toBe(true);
  });

  it("Should accept annotations, satisfies, guards, and optional values", async () => {
    const results = await eslint.lintText(
      'export const absent: string | undefined = undefined; export const item = { id: "a" } satisfies { id: string }; export function isText(value: unknown): value is string { if (typeof value === "string") { return true; } return false; }',
      { filePath: "tests/unit/fixture.ts" },
    );
    expect(results.flatMap((result) => result.messages)).toEqual([]);
  });

  it("Should reject tests placed under src", async () => {
    const results = await eslint.lintText("export const value = 1;", {
      filePath: "src/fixture.test.ts",
    });
    expect(results.flatMap((result) => result.messages)).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ ruleId: "no-restricted-syntax" }),
      ]),
    );
  });

  it("Should require resolved navigation and stable each keys", async () => {
    const results = await eslint.lintText(
      '<script lang="ts">let { items }: { items: string[] } = $props();</script><a href="/">Home</a>{#each items as item}<p>{item}</p>{/each}',
      { filePath: "src/routes/fixture.svelte" },
    );
    const rules = results.flatMap((result) =>
      result.messages.map((message) => message.ruleId),
    );
    expect(rules).toContain("project/resolved-navigation");
    expect(rules).toContain("svelte/require-each-key");
  });

  it("Should accept navigation resolved by the real framework import", async () => {
    const results = await eslint.lintText(
      '<script lang="ts">import { resolve } from "$app/paths";</script><a href={resolve("/")}>Home</a>',
      { filePath: "src/routes/fixture.svelte" },
    );
    expect(results.flatMap((result) => result.messages)).toEqual([]);
  });

  it("Should reject an unrelated function named resolve", async () => {
    const results = await eslint.lintText(
      '<script lang="ts">const resolve = (path: string) => path;</script><a href={resolve("/")}>Home</a>',
      { filePath: "src/routes/fixture.svelte" },
    );
    expect(results.flatMap((result) => result.messages)).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ ruleId: "project/resolved-navigation" }),
      ]),
    );
  });
});
