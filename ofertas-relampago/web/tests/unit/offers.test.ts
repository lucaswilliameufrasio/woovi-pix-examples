import { afterEach, describe, expect, it, vi } from "vitest";
import { actions, load } from "../../src/routes/+page.server";

const pageLoad = load;
const reserve = actions.reserve;

const offer = {
  id: "offer-a",
  title: "Sacola da feira",
  price_cents: 2800,
  available_units: 2,
  reservation_ttl_seconds: 300,
};

const response = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });

function event(fetchImpl: typeof fetch, formData?: FormData) {
  return {
    fetch: fetchImpl,
    request: new Request("http://demo.test/", {
      method: "POST",
      body: formData,
    }),
  };
}

describe("offers page server routes", () => {
  afterEach(() => {
    vi.unstubAllEnvs();
    vi.useRealTimers();
  });

  it("Should load offers from the versioned backend endpoint", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValue(response([offer]));
    const result = await pageLoad(event(fetchMock));

    expect(result).toEqual({ offers: [offer], loadError: undefined });
    expect(fetchMock).toHaveBeenCalledWith(
      "http://127.0.0.1:8080/v1/offers",
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    );
  });

  it("Should use the server-only API_BASE_URL override", async () => {
    vi.stubEnv("API_BASE_URL", "http://127.0.0.1:9080");
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValue(response([offer]));

    await pageLoad(event(fetchMock));

    expect(fetchMock).toHaveBeenCalledWith(
      "http://127.0.0.1:9080/v1/offers",
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    );
  });

  it("Should return a helpful load error when the backend is unavailable", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockRejectedValue(new Error("connection refused"));

    const result = await pageLoad(event(fetchMock));

    expect(result).toMatchObject({
      offers: [],
      loadError: { error_code: "DEPENDENCY_REQUEST" },
    });
  });

  it("Should create only a local order with the server-selected offer id", async () => {
    const order = {
      id: "order-a",
      offer_id: offer.id,
      amount_cents: offer.price_cents,
      state: "pending_payment",
      expires_at: "2026-10-04T21:00:00Z",
    };
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValue(response(order, 201));
    const formData = new FormData();
    formData.set("offer_id", offer.id);

    const result = await reserve(event(fetchMock, formData));

    expect(result).toEqual({ order });
    expect(fetchMock).toHaveBeenCalledWith(
      "http://127.0.0.1:8080/v1/orders",
      expect.objectContaining({
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ offer_id: offer.id }),
      }),
    );
  });

  it("Should reject a missing offer id without calling the backend", async () => {
    const fetchMock = vi.fn<typeof fetch>();
    const result = await reserve(event(fetchMock, new FormData()));

    expect(result).toMatchObject({
      status: 422,
      data: { error_code: "INVALID_PARAMS" },
    });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("Should preserve backend conflict errors for unavailable stock", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValue(
        response(
          { message: "Oferta indisponível.", error_code: "OFFER_UNAVAILABLE" },
          412,
        ),
      );
    const formData = new FormData();
    formData.set("offer_id", offer.id);

    const result = await reserve(event(fetchMock, formData));

    expect(result).toMatchObject({
      status: 412,
      data: {
        message: "Oferta indisponível.",
        error_code: "OFFER_UNAVAILABLE",
      },
    });
  });

  it("Should not claim an order was created when the backend cannot be reached", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockRejectedValue(new Error("connection refused"));
    const formData = new FormData();
    formData.set("offer_id", offer.id);

    const result = await reserve(event(fetchMock, formData));

    expect(result).toMatchObject({
      status: 502,
      data: { error_code: "DEPENDENCY_REQUEST" },
    });
  });

  it.each([
    undefined,
    {},
    { ...offer, price_cents: -1 },
    { ...offer, available_units: 1.5 },
  ])("Should reject invalid catalog data: %j", async (item) => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(response([item]));
    expect(await pageLoad(event(fetchMock))).toMatchObject({
      offers: [],
      loadError: { error_code: "DEPENDENCY_INVALID_RESPONSE" },
    });
  });

  it.each([undefined, {}, { id: "order-a", amount_cents: "2500" }])(
    "Should not confirm a malformed order response: %j",
    async (body) => {
      const fetchMock = vi
        .fn<typeof fetch>()
        .mockResolvedValue(response(body, 201));
      const form = new FormData();
      form.set("offer_id", offer.id);
      expect(await reserve(event(fetchMock, form))).toMatchObject({
        status: 502,
        data: { error_code: "DEPENDENCY_INVALID_RESPONSE" },
      });
      expect(fetchMock).toHaveBeenCalledTimes(1);
    },
  );

  it("Should not forward malformed error fields from the backend", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValue(
        response(
          { message: { internal: "not user text" }, error_code: 123 },
          500,
        ),
      );
    const form = new FormData();
    form.set("offer_id", offer.id);
    expect(await reserve(event(fetchMock, form))).toMatchObject({
      status: 500,
      data: {
        message: "Não foi possível reservar esta sacola.",
        error_code: "UNEXPECTED_ERROR",
      },
    });
  });

  it("Should abort a slow POST without retrying or claiming the order was rejected", async () => {
    vi.useFakeTimers();
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockImplementation((input, options) => {
        expect(input).toBe("http://127.0.0.1:8080/v1/orders");
        const pending = Promise.withResolvers<Response>();
        options?.signal?.addEventListener(
          "abort",
          () => pending.reject(new Error("aborted")),
          { once: true },
        );
        return pending.promise;
      });
    const form = new FormData();
    form.set("offer_id", offer.id);
    const result = reserve(event(fetchMock, form));
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    await vi.advanceTimersByTimeAsync(8000);
    expect(await result).toMatchObject({
      status: 502,
      data: {
        error_code: "DEPENDENCY_REQUEST",
        message: expect.stringContaining("pode ter sido criado"),
      },
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});
