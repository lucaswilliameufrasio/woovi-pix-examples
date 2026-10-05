import { afterEach, describe, expect, it, vi } from "vitest";
import { actions, load } from "../../src/routes/orders/[order_id]/+page.server";
import type { Cookies } from "@sveltejs/kit";
import { createHash } from "node:crypto";

const run = load;
const order = {
  id: "order-a",
  offer_id: "offer-a",
  amount_cents: 2800,
  state: "pending_payment",
  expires_at: "2026-10-04T22:00:00Z",
};
const response = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status });
const token = "a".repeat(64);
function event(
  fetchImpl: typeof fetch,
  orderId = "order-a",
  access: { token?: string } = { token },
) {
  vi.stubGlobal("fetch", fetchImpl);
  return {
    params: { order_id: orderId },
    cookies: { get: vi.fn<Cookies["get"]>().mockReturnValue(access.token) },
  };
}

describe("web order tracking", () => {
  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it.each(["pending_payment", "paid", "expired", "payment_exception"])(
    "Should read authoritative %s without changing the order",
    async (state) => {
      const fetchMock = vi
        .fn<typeof fetch>()
        .mockResolvedValue(
          response({ ...order, state, secret: "never forward" }),
        );
      expect(await run(event(fetchMock))).toEqual({
        order: { ...order, state },
        lookup_error: undefined,
      });
      expect(fetchMock).toHaveBeenCalledExactlyOnceWith(
        "http://127.0.0.1:8080/v1/customer/orders/order-a",
        expect.objectContaining({
          signal: expect.any(AbortSignal),
          headers: {
            accept: "application/json",
            authorization: `Bearer ${token}`,
          },
        }),
      );
    },
  );

  it("Should use the server-only configured backend", async () => {
    vi.stubEnv("API_BASE_URL", "http://127.0.0.1:9080");
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(response(order));
    await run(event(fetchMock));
    expect(fetchMock).toHaveBeenCalledWith(
      "http://127.0.0.1:9080/v1/customer/orders/order-a",
      expect.anything(),
    );
  });

  it.each(["../other", "bad/id", "", "a".repeat(201)])(
    "Should reject invalid ID before HTTP: %s",
    async (id) => {
      const fetchMock = vi.fn<typeof fetch>();
      await expect(run(event(fetchMock, id))).rejects.toMatchObject({
        status: 404,
      });
      expect(fetchMock).not.toHaveBeenCalled();
    },
  );

  it("Should deny access to an order removed by the local demo reset", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValue(response({ error_code: "ORDER_UNAUTHORIZED" }, 401));
    await expect(run(event(fetchMock))).rejects.toMatchObject({ status: 401 });
  });

  it.each([
    undefined,
    {},
    { ...order, id: "another-order" },
    { ...order, amount_cents: -1 },
    { ...order, state: "unknown" },
    { ...order, expires_at: "invalid" },
  ])("Should reject invalid order response: %j", async (body) => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(response(body));
    expect(await run(event(fetchMock))).toMatchObject({
      order: undefined,
      lookup_error: { error_code: "DEPENDENCY_INVALID_RESPONSE" },
    });
  });

  it("Should reject non-JSON data without presenting a cached order", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValue(new Response("not JSON"));
    expect(await run(event(fetchMock))).toMatchObject({
      order: undefined,
      lookup_error: { error_code: "DEPENDENCY_INVALID_RESPONSE" },
    });
  });

  it("Should show a retry direction after a network error", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockRejectedValue(new Error("offline"));
    expect(await run(event(fetchMock))).toMatchObject({
      order: undefined,
      lookup_error: { error_code: "DEPENDENCY_REQUEST" },
    });
  });

  it("Should abort a slow lookup without retry or order mutation", async () => {
    vi.useFakeTimers();
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockImplementation((input, options) => {
        expect(input).toBe("http://127.0.0.1:8080/v1/customer/orders/order-a");
        const pending = Promise.withResolvers<Response>();
        options?.signal?.addEventListener(
          "abort",
          () => pending.reject(new Error("aborted")),
          { once: true },
        );
        return pending.promise;
      });
    const pending = run(event(fetchMock));
    await vi.advanceTimersByTimeAsync(8000);
    expect(await pending).toMatchObject({
      order: undefined,
      lookup_error: { error_code: "DEPENDENCY_REQUEST" },
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("Should not treat a known ID as authorization without its HttpOnly cookie", async () => {
    const fetchMock = vi.fn<typeof fetch>();
    await expect(run(event(fetchMock, "order-a", {}))).rejects.toMatchObject({
      status: 401,
    });
    expect(fetchMock).not.toHaveBeenCalled();
    expect(
      await actions.checkout(event(fetchMock, "order-a", {})),
    ).toMatchObject({
      status: 401,
      data: { error_code: "ORDER_UNAUTHORIZED" },
    });
  });

  it("Should open a restricted local session with stable idempotency and never return its token", async () => {
    const checkout = {
      checkout_id: "11111111-1111-4111-8111-111111111111",
      order_id: order.id,
      mode: "local_simulation",
      access_token: "b".repeat(64),
    };
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValue(response(checkout, 201));
    const result = await actions.checkout(event(fetchMock));
    expect(result).toEqual({
      checkout: { checkout_id: checkout.checkout_id, mode: "local_simulation" },
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "http://127.0.0.1:8080/v1/customer/orders/order-a/checkout",
      expect.objectContaining({
        method: "POST",
        headers: {
          authorization: `Bearer ${token}`,
          "Idempotency-Key": `web-checkout-${createHash("sha256").update(order.id).digest("hex")}`,
        },
        redirect: "error",
        credentials: "omit",
        cache: "no-store",
      }),
    );
    expect(JSON.stringify(result)).not.toContain(checkout.access_token);
  });

  it("Should preserve a manual recovery direction without retrying an uncertain checkout POST", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockRejectedValue(new Error("offline"));
    expect(await actions.checkout(event(fetchMock))).toMatchObject({
      status: 502,
      data: {
        error_code: "DEPENDENCY_REQUEST",
        message: expect.stringContaining("mesmo botão"),
      },
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("Should reject unexpected Pix data rather than enabling a made-up payment", async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(
      response(
        {
          checkout_id: "11111111-1111-4111-8111-111111111111",
          order_id: order.id,
          mode: "local_simulation",
          access_token: "b".repeat(64),
          pix_copy_paste: "invalid-example",
        },
        201,
      ),
    );
    expect(await actions.checkout(event(fetchMock))).toMatchObject({
      status: 502,
      data: { error_code: "DEPENDENCY_INVALID_RESPONSE" },
    });
  });

  it("Should not send credentials to a remote or credential-bearing backend URL", async () => {
    const fetchMock = vi.fn<typeof fetch>();
    for (const base of [
      "https://example.com",
      "http://user:password@127.0.0.1:8080",
      "http://127.0.0.1:8080/path",
      "http://0.0.0.0:8080",
    ]) {
      vi.stubEnv("API_BASE_URL", base);
      expect(await run(event(fetchMock))).toMatchObject({
        order: undefined,
        lookup_error: { error_code: "DEPENDENCY_REQUEST" },
      });
    }
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
