import type { ApiError, Order, Product, ReservedOrder } from "#lib/types";

export function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

function apiBase(): URL {
  const value = process.env.API_BASE_URL ?? "http://127.0.0.1:8082";
  const url = new URL(value);
  if (
    url.protocol !== "http:" ||
    !["127.0.0.1", "[::1]"].includes(url.hostname) ||
    !url.port ||
    url.username ||
    url.password ||
    url.pathname !== "/" ||
    url.search ||
    url.hash
  ) {
    throw new Error(
      "The click-and-collect backend must use an explicit loopback URL.",
    );
  }
  return url;
}

function validProduct(value: unknown): value is Product {
  return (
    isRecord(value) &&
    typeof value.id === "string" &&
    typeof value.title === "string" &&
    typeof value.description === "string" &&
    typeof value.price_cents === "number" &&
    Number.isSafeInteger(value.price_cents) &&
    value.price_cents > 0 &&
    typeof value.available_units === "number" &&
    Number.isSafeInteger(value.available_units) &&
    value.available_units >= 0 &&
    typeof value.reservation_ttl_seconds === "number" &&
    Number.isSafeInteger(value.reservation_ttl_seconds) &&
    value.reservation_ttl_seconds > 0
  );
}

function validOrder(value: unknown): value is Order {
  return (
    isRecord(value) &&
    typeof value.id === "string" &&
    typeof value.product_id === "string" &&
    typeof value.product_title === "string" &&
    typeof value.amount_cents === "number" &&
    Number.isSafeInteger(value.amount_cents) &&
    value.amount_cents > 0 &&
    typeof value.payment_state === "string" &&
    ["pending", "paid", "expired", "cancelled", "payment_exception"].includes(
      value.payment_state,
    ) &&
    typeof value.fulfillment_state === "string" &&
    ["awaiting_payment", "preparing", "ready_for_pickup", "picked_up"].includes(
      value.fulfillment_state,
    ) &&
    typeof value.expires_at === "string" &&
    Number.isFinite(Date.parse(value.expires_at)) &&
    typeof value.created_at === "string" &&
    Number.isFinite(Date.parse(value.created_at))
  );
}

function validReservedOrder(value: unknown): value is ReservedOrder {
  if (!isRecord(value)) {
    return false;
  }
  const accessToken = value.order_access_token;
  const pickupCode = value.pickup_code;
  return (
    validOrder(value) &&
    typeof accessToken === "string" &&
    /^[a-f0-9]{64}$/.test(accessToken) &&
    typeof pickupCode === "string" &&
    /^[A-F0-9]{8}$/.test(pickupCode)
  );
}

async function request(path: string, init?: RequestInit): Promise<unknown> {
  const url = new URL(path, apiBase());
  let response: Response;
  try {
    response = await fetch(url, {
      ...init,
      signal: AbortSignal.timeout(5000),
      headers: { "Content-Type": "application/json", ...init?.headers },
    });
  } catch {
    throw new Error("Não foi possível conectar ao backend local.");
  }
  let body: unknown;
  try {
    body = await response.json();
  } catch {
    throw new Error("O backend local retornou uma resposta inválida.");
  }
  if (!response.ok) {
    if (
      isRecord(body) &&
      typeof body.message === "string" &&
      typeof body.error_code === "string"
    ) {
      const error: ApiError = {
        message: body.message,
        error_code: body.error_code,
      };
      throw new Error(error.message);
    }
    throw new Error("Não foi possível concluir a operação.");
  }
  return body;
}

export async function listProducts(): Promise<Product[]> {
  const body = await request("/v1/products");
  if (!Array.isArray(body) || !body.every(validProduct)) {
    throw new Error("O catálogo retornado pelo backend é inválido.");
  }
  return body;
}

export async function reserveProduct(
  productID: string,
): Promise<ReservedOrder> {
  const body = await request("/v1/orders", {
    method: "POST",
    body: JSON.stringify({ product_id: productID }),
  });
  if (!validReservedOrder(body)) {
    throw new Error("A reserva retornada pelo backend é inválida.");
  }
  return body;
}

export async function getOrder(orderID: string, token: string): Promise<Order> {
  const body = await request(`/v1/orders/${encodeURIComponent(orderID)}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!validOrder(body)) {
    throw new Error("O pedido retornado pelo backend é inválido.");
  }
  return body;
}

export async function cancelOrder(
  orderID: string,
  token: string,
): Promise<Order> {
  const body = await request(`/v1/orders/${encodeURIComponent(orderID)}`, {
    method: "DELETE",
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!validOrder(body)) {
    throw new Error("O pedido retornado pelo backend é inválido.");
  }
  return body;
}

export async function simulatePayment(orderID: string): Promise<void> {
  const token = process.env.DEMO_SIMULATOR_TOKEN;
  if (!token || token.length < 32) {
    throw new Error("O simulador local não está configurado.");
  }
  const charge = await request(
    `/v1/simulator/orders/${encodeURIComponent(orderID)}/charge`,
    {
      method: "POST",
      headers: { "X-Demo-Token": token },
    },
  );
  if (!isRecord(charge) || typeof charge.id !== "string") {
    throw new Error("A cobrança simulada retornada pelo backend é inválida.");
  }
  await request(`/v1/simulator/orders/${encodeURIComponent(orderID)}/confirm`, {
    method: "POST",
    headers: { "X-Demo-Token": token },
  });
}

export async function listOperatorOrders(): Promise<Order[]> {
  const token = process.env.DEMO_OPERATOR_TOKEN;
  if (!token || token.length < 32) {
    throw new Error("A loja local não está configurada.");
  }
  const body = await request("/v1/operator/orders", {
    headers: { "X-Demo-Token": token },
  });
  if (!Array.isArray(body) || !body.every(validOrder)) {
    throw new Error("A fila retornada pelo backend é inválida.");
  }
  return body;
}

export async function markReady(orderID: string): Promise<Order> {
  const token = process.env.DEMO_OPERATOR_TOKEN;
  if (!token || token.length < 32) {
    throw new Error("A loja local não está configurada.");
  }
  const body = await request(
    `/v1/operator/orders/${encodeURIComponent(orderID)}/ready`,
    {
      method: "POST",
      headers: { "X-Demo-Token": token },
    },
  );
  if (!validOrder(body)) {
    throw new Error("O pedido retornado pelo backend é inválido.");
  }
  return body;
}

export async function redeemPickup(
  orderID: string,
  pickupCode: string,
): Promise<Order> {
  const token = process.env.DEMO_OPERATOR_TOKEN;
  if (!token || token.length < 32) {
    throw new Error("A loja local não está configurada.");
  }
  const body = await request(
    `/v1/operator/orders/${encodeURIComponent(orderID)}/pickup`,
    {
      method: "POST",
      headers: { "X-Demo-Token": token },
      body: JSON.stringify({ pickup_code: pickupCode }),
    },
  );
  if (!validOrder(body)) {
    throw new Error("O pedido retornado pelo backend é inválido.");
  }
  return body;
}
