import { error } from "@sveltejs/kit";
import type { PageServerLoad } from "./$types";
import type { ApiError, DemoOrder } from "../../../lib/types";

function record(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

function orderState(value: unknown): value is DemoOrder["state"] {
  return (
    value === "pending_payment" ||
    value === "paid" ||
    value === "expired" ||
    value === "payment_exception"
  );
}

function parseOrder(body: unknown, orderId: string): DemoOrder | undefined {
  if (!record(body)) {
    return undefined;
  }
  if (
    body.id !== orderId ||
    typeof body.offer_id !== "string" ||
    !body.offer_id.trim() ||
    typeof body.amount_cents !== "number" ||
    !Number.isSafeInteger(body.amount_cents) ||
    body.amount_cents <= 0 ||
    !orderState(body.state) ||
    typeof body.expires_at !== "string" ||
    !Number.isFinite(Date.parse(body.expires_at))
  ) {
    return undefined;
  }
  // Project only display fields; do not forward arbitrary backend fields.
  return {
    id: orderId,
    offer_id: body.offer_id,
    amount_cents: body.amount_cents,
    state: body.state,
    expires_at: body.expires_at,
  };
}

export const load = (async ({
  params,
  fetch,
}: Pick<Parameters<PageServerLoad>[0], "params" | "fetch">) => {
  const orderId = params.order_id;
  if (!/^[A-Za-z0-9_-]{1,200}$/.test(orderId)) {
    error(404, "Pedido não encontrado.");
  }
  const baseUrl = process.env.API_BASE_URL || "http://127.0.0.1:8080";
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 8000);
  let response: Response;
  let body: unknown;
  try {
    response = await fetch(
      `${baseUrl}/v1/orders/${encodeURIComponent(orderId)}`,
      {
        signal: controller.signal,
        headers: { accept: "application/json" },
      },
    );
    try {
      body = await response.json();
    } catch (failure) {
      if (controller.signal.aborted) {
        throw failure;
      }
      body = undefined;
    }
  } catch {
    return {
      order: undefined,
      lookup_error: {
        message:
          "Não foi possível consultar o pedido. Tente atualizar; seu estado não foi alterado.",
        error_code: "DEPENDENCY_REQUEST",
      } satisfies ApiError,
    };
  } finally {
    clearTimeout(timer);
  }
  if (response.status === 404) {
    error(404, "Pedido não encontrado.");
  }
  const order = response.status === 200 ? parseOrder(body, orderId) : undefined;
  if (!order) {
    return {
      order: undefined,
      lookup_error: {
        message: "A API local não retornou um pedido válido. Tente atualizar.",
        error_code: "DEPENDENCY_INVALID_RESPONSE",
      } satisfies ApiError,
    };
  }
  return { order, lookup_error: undefined };
}) satisfies PageServerLoad;
