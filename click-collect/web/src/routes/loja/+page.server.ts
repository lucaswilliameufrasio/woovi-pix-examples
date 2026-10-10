import { fail, isRedirect, redirect } from "@sveltejs/kit";
import {
  listOperatorOrders,
  markReady,
  redeemPickup,
} from "#lib/server/backend";
import type { Actions, PageServerLoad } from "./$types";

export const load: PageServerLoad = async () => {
  try {
    return { orders: await listOperatorOrders(), unavailable: false };
  } catch {
    return { orders: [], unavailable: true };
  }
};

export const actions: Actions = {
  ready: async ({ request }) => {
    const form = await request.formData();
    const orderID = form.get("order_id");
    if (typeof orderID !== "string" || !/^[a-f0-9]{32}$/.test(orderID)) {
      return fail(400, { message: "Pedido inválido." });
    }
    try {
      await markReady(orderID);
    } catch (error) {
      if (isRedirect(error)) {
        throw error;
      }
      const message =
        error instanceof Error
          ? error.message
          : "Não foi possível atualizar o preparo.";
      return fail(409, { message });
    }
    redirect(303, "/loja");
  },
  pickup: async ({ request }) => {
    const form = await request.formData();
    const orderID = form.get("order_id");
    const pickupCode = form.get("pickup_code");
    if (
      typeof orderID !== "string" ||
      !/^[a-f0-9]{32}$/.test(orderID) ||
      typeof pickupCode !== "string" ||
      !/^[A-Fa-f0-9]{8}$/.test(pickupCode)
    ) {
      return fail(400, { message: "Pedido ou código de retirada inválido." });
    }
    try {
      await redeemPickup(orderID, pickupCode.toUpperCase());
    } catch (error) {
      if (isRedirect(error)) {
        throw error;
      }
      const message =
        error instanceof Error
          ? error.message
          : "Não foi possível confirmar a retirada.";
      return fail(409, { message });
    }
    redirect(303, "/loja");
  },
};
