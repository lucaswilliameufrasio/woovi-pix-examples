import { fail } from "@sveltejs/kit";
import type { Actions, PageServerLoad } from "./$types";
import type { ApiError, DemoOrder, Offer } from "../lib/types";

const apiBaseUrl = () => process.env.API_BASE_URL || "http://127.0.0.1:8080";

function record(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

function text(value: unknown): value is string {
  return typeof value === "string" && value.trim().length > 0;
}

function integer(value: unknown, minimum: number): value is number {
  return (
    typeof value === "number" && Number.isSafeInteger(value) && value >= minimum
  );
}

function validOffer(value: unknown): value is Offer {
  return (
    record(value) &&
    text(value.id) &&
    text(value.title) &&
    integer(value.price_cents, 1) &&
    integer(value.available_units, 0) &&
    integer(value.reservation_ttl_seconds, 1)
  );
}

function validOrder(value: unknown, offerId: string): value is DemoOrder {
  return (
    record(value) &&
    text(value.id) &&
    value.offer_id === offerId &&
    integer(value.amount_cents, 1) &&
    text(value.state) &&
    ["pending_payment", "paid", "expired", "payment_exception"].includes(
      value.state,
    ) &&
    text(value.expires_at) &&
    Number.isFinite(Date.parse(value.expires_at))
  );
}

// Limit both response headers and body reading. Never retry POST automatically.
async function backendRequest(
  fetchImpl: typeof fetch,
  path: string,
  options: RequestInit = {},
): Promise<{ response: Response; body: unknown }> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 8000);
  try {
    const response = await fetchImpl(`${apiBaseUrl()}${path}`, {
      ...options,
      signal: controller.signal,
    });
    const body = await readJson(response);
    return { response, body };
  } finally {
    clearTimeout(timer);
  }
}

async function readJson(response: Response): Promise<unknown> {
  try {
    return await response.json();
  } catch {
    return undefined;
  }
}

function apiError(body: unknown, fallback: string): ApiError {
  if (
    record(body) &&
    text(body.message) &&
    text(body.error_code) &&
    /^[A-Z][A-Z0-9_]*$/.test(body.error_code)
  ) {
    return {
      message: body.message,
      error_code: body.error_code,
      ...(record(body.extra) ? { extra: body.extra } : {}),
    };
  }
  return { message: fallback, error_code: "UNEXPECTED_ERROR" };
}

const emptyOffers: Offer[] = [];

export const load = (async ({
  fetch,
}: Pick<Parameters<PageServerLoad>[0], "fetch">) => {
  try {
    const { response, body } = await backendRequest(fetch, "/v1/offers");
    if (response.status !== 200) {
      return {
        offers: emptyOffers,
        loadError: apiError(body, "Não foi possível carregar as ofertas."),
      };
    }
    if (!Array.isArray(body) || !body.every(validOffer)) {
      return {
        offers: emptyOffers,
        loadError: {
          message: "A API local retornou ofertas inválidas.",
          error_code: "DEPENDENCY_INVALID_RESPONSE",
        } satisfies ApiError,
      };
    }
    return { offers: body, loadError: undefined };
  } catch {
    return {
      offers: emptyOffers,
      loadError: {
        message: "A loja local não respondeu. Confira se a API está rodando.",
        error_code: "DEPENDENCY_REQUEST",
      } satisfies ApiError,
    };
  }
}) satisfies PageServerLoad;

export const actions = {
  reserve: async ({
    request,
    fetch,
  }: Pick<
    Parameters<NonNullable<Actions[string]>>[0],
    "request" | "fetch"
  >) => {
    const formData = await request.formData();
    const offerId = formData.get("offer_id");
    if (typeof offerId !== "string" || offerId.trim() === "") {
      return fail(422, {
        message: "Escolha uma oferta para reservar.",
        error_code: "INVALID_PARAMS",
      });
    }

    try {
      const { response, body } = await backendRequest(fetch, "/v1/orders", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ offer_id: offerId }),
      });
      if (!response.ok) {
        const error = apiError(body, "Não foi possível reservar esta sacola.");
        return fail(response.status, error);
      }
      if (response.status !== 201 || !validOrder(body, offerId)) {
        return fail(502, {
          message:
            "A resposta da API local é inválida. O pedido pode ter sido criado; não repita a reserva sem verificar.",
          error_code: "DEPENDENCY_INVALID_RESPONSE",
        });
      }
      return { order: body };
    } catch {
      return fail(502, {
        message:
          "A API local não respondeu. O pedido pode ter sido criado; não repita a reserva sem verificar.",
        error_code: "DEPENDENCY_REQUEST",
      });
    }
  },
} satisfies Actions;
