import type { Cookies } from "@sveltejs/kit";
import { resolve } from "$app/paths";
import { credential, record } from "./backend";

export const orderCookie = (id: string) => `order_access_${id}`;

export function parseOrderAccess(
  body: unknown,
): { token: string; expires_at: Date } | undefined {
  if (
    !record(body) ||
    !credential(body.order_access_token) ||
    typeof body.order_token_expires_at !== "string"
  ) {
    return undefined;
  }
  const expiry = new Date(body.order_token_expires_at);
  if (
    !Number.isFinite(expiry.getTime()) ||
    expiry.getTime() <= Date.now() ||
    expiry.getTime() > Date.now() + 21 * 60 * 1000
  ) {
    return undefined;
  }
  return { token: body.order_access_token, expires_at: expiry };
}

export function saveOrderAccess(
  cookies: Pick<Cookies, "set">,
  url: URL,
  id: string,
  access: { token: string; expires_at: Date },
): void {
  cookies.set(orderCookie(id), access.token, {
    path: resolve("/orders/[order_id]", { order_id: id }),
    httpOnly: true,
    sameSite: "strict",
    secure: url.protocol === "https:",
    expires: access.expires_at,
  });
}
