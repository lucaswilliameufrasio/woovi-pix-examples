import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { listProducts, reserveProduct } from "#lib/server/backend";

describe("click and collect backend adapter", () => {
  beforeEach(() => {
    vi.stubEnv("API_BASE_URL", "http://127.0.0.1:8082");
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.unstubAllEnvs();
  });

  it("Should validate the catalog response at the boundary", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() =>
        Promise.resolve(
          new Response(
            JSON.stringify([
              {
                id: "house-cake",
                title: "Bolo de fubá da casa",
                description: "Fatia preparada hoje.",
                price_cents: 890,
                available_units: 4,
                reservation_ttl_seconds: 180,
              },
            ]),
            { status: 200, headers: { "Content-Type": "application/json" } },
          ),
        ),
      ),
    );

    await expect(listProducts()).resolves.toMatchObject([
      { id: "house-cake", price_cents: 890 },
    ]);
  });

  it("Should reject an invalid backend origin before making a request", async () => {
    vi.stubEnv("API_BASE_URL", "https://example.com");
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);

    await expect(listProducts()).rejects.toThrow("explicit loopback URL");
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("Should reject malformed catalog values", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() =>
        Promise.resolve(new Response(JSON.stringify([{ id: "bad" }]))),
      ),
    );

    await expect(listProducts()).rejects.toThrow(
      "O catálogo retornado pelo backend é inválido.",
    );
  });

  it("Should not accept a reservation without private credentials", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() =>
        Promise.resolve(
          new Response(
            JSON.stringify({
              id: "0123456789abcdef0123456789abcdef",
              product_id: "house-cake",
              product_title: "Bolo de fubá da casa",
              amount_cents: 890,
              payment_state: "pending",
              fulfillment_state: "awaiting_payment",
              expires_at: "2026-10-09T15:00:00Z",
              created_at: "2026-10-09T14:57:00Z",
            }),
            { status: 201 },
          ),
        ),
      ),
    );

    await expect(reserveProduct("house-cake")).rejects.toThrow(
      "A reserva retornada pelo backend é inválida.",
    );
  });
});
