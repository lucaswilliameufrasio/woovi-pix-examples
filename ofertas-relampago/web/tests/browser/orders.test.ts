import { expect, test as it } from "@playwright/test";
import type { BrowserContext } from "@playwright/test";

function loopback(name: string): string {
  const value = process.env[name];
  if (!value) {
    throw new Error("Missing isolated browser fixture configuration.");
  }
  const url = new URL(value);
  if (
    url.protocol !== "http:" ||
    url.hostname !== "127.0.0.1" ||
    !url.port ||
    url.username ||
    url.password ||
    url.pathname !== "/" ||
    url.search ||
    url.hash
  ) {
    throw new Error("Only explicit loopback fixtures are supported.");
  }
  return url.origin;
}

function record(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

async function resource(
  base: string,
  path: string,
  method: string,
  status: number,
  token?: string,
  body?: Record<string, unknown>,
): Promise<Record<string, unknown>> {
  const response = await fetch(`${base}${path}`, {
    method,
    redirect: "error",
    signal: AbortSignal.timeout(8000),
    headers: {
      ...(token ? { authorization: `Bearer ${token}` } : {}),
      ...(body ? { "content-type": "application/json" } : {}),
    },
    ...(body ? { body: JSON.stringify(body) } : {}),
  });
  // Do not attach request headers/body to assertions or logs.
  expect(response.status).toBe(status);
  const value: unknown = await response.json();
  if (!record(value)) {
    throw new Error("Invalid local fixture response (body omitted).");
  }
  return value;
}

it("Should reserve through the browser, keep credentials private and reflect local payment exactly once", async ({
  page,
  context,
  browser,
}) => {
  const web = loopback("BROWSER_SMOKE_WEB_BASE");
  const api = loopback("BROWSER_SMOKE_API_BASE");
  const simulator = loopback("BROWSER_SMOKE_SIM_BASE");
  const operator = process.env.DEMO_OPERATOR_TOKEN;
  if (!operator) {
    throw new Error(
      "Missing operator fixture; never put this secret in the page.",
    );
  }
  const remoteRequests: string[] = [];
  async function guard(local: BrowserContext): Promise<void> {
    await local.route("**/*", async (route) => {
      if (new URL(route.request().url()).origin !== web) {
        remoteRequests.push("blocked");
        await route.abort();
        return;
      }
      await route.continue();
    });
  }
  await guard(context);
  const pageErrors: string[] = [];
  page.on("pageerror", () => pageErrors.push("page error"));
  await page.goto(web);
  await expect(
    page.getByRole("button", { name: "Reservar sacola", exact: true }),
  ).toBeEnabled();
  await page
    .getByRole("button", { name: "Reservar sacola", exact: true })
    .click();
  await expect(page.getByText("Reserva criada", { exact: true })).toBeVisible();
  const link = page.getByRole("link", { name: "Acompanhar pedido" });
  const destination = await link.getAttribute("href");
  const target = destination ? new URL(destination, page.url()) : undefined;
  if (
    !target ||
    target.origin !== web ||
    !/^\/orders\/[a-f0-9]{32}$/.test(target.pathname)
  ) {
    throw new Error("Missing valid order reference.");
  }
  const href = target.pathname;
  const orderId = href.slice("/orders/".length);
  const cookies = await context.cookies(web + href);
  const access = cookies.find(
    (cookie) => cookie.name === `order_access_${orderId}`,
  );
  if (!access) {
    throw new Error("Missing HttpOnly order capability (values omitted).");
  }
  expect(access.httpOnly).toBe(true);
  expect(access.sameSite).toBe("Strict");
  expect(access.path).toBe(href);
  expect(access.expires).toBeGreaterThan(Date.now() / 1000);
  expect(access.expires).toBeLessThanOrEqual(Date.now() / 1000 + 21 * 60);
  expect((await page.content()).includes(access.value)).toBe(false);
  await link.click();
  await expect(
    page.getByRole("heading", { name: "Aguardando pagamento" }),
  ).toBeVisible();
  expect(
    await page.evaluate(() => document.cookie.includes("order_access_")),
  ).toBe(false);
  expect((await page.content()).includes(access.value)).toBe(false);
  expect((await page.content()).includes(operator)).toBe(false);

  const anonymous = await browser.newContext();
  try {
    await guard(anonymous);
    const stranger = await anonymous.newPage();
    const denied = await stranger.goto(web + href);
    expect(denied?.status()).toBe(401);
    await expect(
      stranger.getByText("Aguardando pagamento", { exact: true }),
    ).toHaveCount(0);
  } finally {
    await anonymous.close();
  }
  await resource(api, `/v1/orders/${orderId}`, "GET", 401);

  const sessions: string[] = [];
  for (let i = 0; i < 2; i++) {
    // Replay returns identical text; visibility alone can see the old result.
    await Promise.all([
      page.waitForNavigation({ waitUntil: "domcontentloaded" }),
      page.getByRole("button", { name: "Abrir sessão local sem Pix" }).click(),
    ]);
    await expect(page.getByRole("status")).toContainText("Sessão local");
    const text = await page.getByRole("status").textContent();
    const match = text?.match(/Sessão local ([a-f0-9-]{36})/);
    if (!match) {
      throw new Error("Missing local session reference.");
    }
    sessions.push(match[1]);
  }
  expect(sessions[0]).toBe(sessions[1]);
  expect((await page.content()).includes("access_token")).toBe(false);

  // Recovery uses storage state only in memory, never a file with cookies.
  const restarted = await browser.newContext({
    storageState: await context.storageState(),
  });
  try {
    await guard(restarted);
    const restored = await restarted.newPage();
    await restored.goto(web + href);
    await expect(
      restored.getByRole("heading", { name: "Aguardando pagamento" }),
    ).toBeVisible();
  } finally {
    await restarted.close();
  }

  await resource(simulator, `/v1/charges/${orderId}/pay`, "POST", 200);
  await expect
    .poll(
      async () => {
        const current = await resource(
          api,
          `/v1/customer/orders/${orderId}`,
          "GET",
          200,
          access.value,
        );
        return current.state;
      },
      { timeout: 12000 },
    )
    .toBe("paid");
  await page.getByRole("link", { name: "Atualizar estado" }).click();
  await expect(
    page.getByRole("heading", { name: "Pagamento registrado" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Abrir sessão local sem Pix" }),
  ).toHaveCount(0);
  const pickup = await resource(
    api,
    `/v1/operator/orders/${orderId}/pickup-token`,
    "POST",
    201,
    operator,
  );
  if (typeof pickup.pickup_token !== "string") {
    throw new Error("Invalid pickup fixture (values omitted).");
  }
  await resource(
    api,
    `/v1/operator/orders/${orderId}/pickup`,
    "POST",
    200,
    operator,
    { pickup_token: pickup.pickup_token },
  );
  const duplicate = await resource(
    api,
    `/v1/operator/orders/${orderId}/pickup`,
    "POST",
    409,
    operator,
    { pickup_token: pickup.pickup_token },
  );
  expect(duplicate.error_code).toBe("PICKUP_ALREADY_DONE");
  // Losing the browser capability must not render the previously paid state.
  await context.clearCookies();
  const revoked = await page.goto(web + href);
  expect(revoked?.status()).toBe(401);
  await expect(
    page.getByRole("heading", { name: "Pagamento registrado" }),
  ).toHaveCount(0);
  expect((await page.content()).includes(access.value)).toBe(false);
  expect(remoteRequests).toEqual([]);
  expect(pageErrors).toEqual([]);
});
