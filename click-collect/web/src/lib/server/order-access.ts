import type { Cookies } from "@sveltejs/kit";
import { resolve } from "$app/paths";

const accessCookie = (orderID: string) => `click_collect_access_${orderID}`;
const pickupCookie = (orderID: string) => `click_collect_pickup_${orderID}`;

export function orderAccess(
  cookies: Cookies,
  orderID: string,
): string | undefined {
  const token = cookies.get(accessCookie(orderID));
  if (!token || !/^[a-f0-9]{64}$/.test(token)) {
    return undefined;
  }
  return token;
}

export function pickupAccess(
  cookies: Cookies,
  orderID: string,
): string | undefined {
  const code = cookies.get(pickupCookie(orderID));
  if (!code || !/^[A-F0-9]{8}$/.test(code)) {
    return undefined;
  }
  return code;
}

export function saveOrderAccess(
  cookies: Pick<Cookies, "set">,
  orderID: string,
  token: string,
  pickupCode: string,
): void {
  const path = resolve("/orders/[order_id]", { order_id: orderID });
  type CookieOptions = NonNullable<Parameters<Cookies["set"]>[2]>;
  const options: CookieOptions = {
    path,
    httpOnly: true,
    secure: false,
    sameSite: "strict",
    maxAge: 24 * 60 * 60,
  };
  cookies.set(accessCookie(orderID), token, options);
  cookies.set(pickupCookie(orderID), pickupCode, options);
}
