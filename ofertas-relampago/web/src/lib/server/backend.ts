import type { ApiError, DemoOrder } from "../types";

export function record(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

export function text(value: unknown): value is string {
  return typeof value === "string" && value.trim().length > 0;
}

export function integer(value: unknown, minimum: number): value is number {
  return (
    typeof value === "number" && Number.isSafeInteger(value) && value >= minimum
  );
}

export function credential(value: unknown): value is string {
  return typeof value === "string" && /^[a-f0-9]{64}$/.test(value);
}

function baseUrl(): string {
  const url = new URL(process.env.API_BASE_URL || "http://127.0.0.1:8080");
  if (
    url.protocol !== "http:" ||
    !["127.0.0.1", "[::1]"].includes(url.hostname) ||
    !url.port ||
    url.username ||
    url.password ||
    url.search ||
    url.hash ||
    url.pathname !== "/"
  ) {
    throw new Error("Only explicit loopback backend bases are supported");
  }
  return url.origin;
}

// Calls with credentials use the native server fetch, not SvelteKit event.fetch:
// it must never capture the upstream body for serialization during hydration.
export async function backendRequest(
  fetchImpl: typeof fetch,
  path: string,
  options: RequestInit = {},
): Promise<{ response: Response; body: unknown }> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 8000);
  try {
    const response = await fetchImpl(`${baseUrl()}${path}`, {
      ...options,
      signal: controller.signal,
      redirect: "error",
      credentials: "omit",
      cache: "no-store",
    });
    let body: unknown;
    try {
      body = await response.json();
    } catch (failure) {
      if (controller.signal.aborted) {
        throw failure;
      }
      body = undefined;
    }
    return { response, body };
  } finally {
    clearTimeout(timer);
  }
}

export function apiError(body: unknown, fallback: string): ApiError {
  if (
    record(body) &&
    text(body.message) &&
    text(body.error_code) &&
    /^[A-Z][A-Z0-9_]*$/.test(body.error_code)
  ) {
    // Only safe, human text and machine code. Never forward arbitrary metadata
    // that could contain upstream credentials into action data.
    return { message: body.message, error_code: body.error_code };
  }
  return { message: fallback, error_code: "UNEXPECTED_ERROR" };
}

function orderState(value: unknown): value is DemoOrder["state"] {
  return (
    value === "pending_payment" ||
    value === "paid" ||
    value === "expired" ||
    value === "payment_exception"
  );
}

export function parseOrder(
  body: unknown,
  orderId?: string,
): DemoOrder | undefined {
  if (
    !record(body) ||
    !text(body.id) ||
    !/^[A-Za-z0-9_-]{1,200}$/.test(body.id) ||
    (orderId !== undefined && body.id !== orderId) ||
    !text(body.offer_id) ||
    !integer(body.amount_cents, 1) ||
    !orderState(body.state) ||
    !text(body.expires_at) ||
    !Number.isFinite(Date.parse(body.expires_at))
  ) {
    return undefined;
  }
  return {
    id: body.id,
    offer_id: body.offer_id,
    amount_cents: body.amount_cents,
    state: body.state,
    expires_at: body.expires_at,
  };
}
