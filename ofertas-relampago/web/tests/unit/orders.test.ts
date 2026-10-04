import { afterEach, describe, expect, it, vi } from "vitest";
import { load } from "../../src/routes/orders/[order_id]/+page.server";

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
const event = (fetchImpl: typeof fetch, orderId = "order-a") => ({
  params: { order_id: orderId },
  fetch: fetchImpl,
});

describe("web order tracking", () => {
  afterEach(() => {
    vi.unstubAllEnvs();
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
        "http://127.0.0.1:8080/v1/orders/order-a",
        expect.objectContaining({
          signal: expect.any(AbortSignal),
          headers: { accept: "application/json" },
        }),
      );
    },
  );

  it("Should use the server-only configured backend", async () => {
    vi.stubEnv("API_BASE_URL", "http://127.0.0.1:9080");
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(response(order));
    await run(event(fetchMock));
    expect(fetchMock).toHaveBeenCalledWith(
      "http://127.0.0.1:9080/v1/orders/order-a",
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

  it("Should return 404 for an order removed by the local demo reset", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValue(response({ error_code: "ORDER_NOT_FOUND" }, 404));
    await expect(run(event(fetchMock))).rejects.toMatchObject({ status: 404 });
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
        expect(input).toBe("http://127.0.0.1:8080/v1/orders/order-a");
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
});
