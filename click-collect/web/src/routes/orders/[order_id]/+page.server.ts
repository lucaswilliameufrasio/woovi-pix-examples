import { fail, isRedirect, redirect } from "@sveltejs/kit";
import { resolve } from "$app/paths";
import type { Actions, PageServerLoad } from "./$types";
import { cancelOrder, getOrder, simulatePayment } from "#lib/server/backend";
import { orderAccess, pickupAccess } from "#lib/server/order-access";

export const load: PageServerLoad = async ({ cookies, params }) => {
  const access = orderAccess(cookies, params.order_id);
  if (!access) {
    return { order: undefined, pickupCode: undefined, unavailable: true };
  }
  try {
    return {
      order: await getOrder(params.order_id, access),
      pickupCode: pickupAccess(cookies, params.order_id),
      unavailable: false,
    };
  } catch {
    return { order: undefined, pickupCode: undefined, unavailable: true };
  }
};

export const actions: Actions = {
  refresh: async ({ cookies, params }) => {
    const access = orderAccess(cookies, params.order_id);
    if (!access) {
      return fail(401, { message: "Este navegador não tem acesso ao pedido." });
    }
    try {
      await getOrder(params.order_id, access);
    } catch (error) {
      if (isRedirect(error)) {
        throw error;
      }
      const message =
        error instanceof Error
          ? error.message
          : "Não foi possível consultar o pedido.";
      return fail(409, { message });
    }
    redirect(303, resolve("/orders/[order_id]", { order_id: params.order_id }));
  },
  simulate: async ({ cookies, params }) => {
    const access = orderAccess(cookies, params.order_id);
    if (!access) {
      return fail(401, { message: "Este navegador não tem acesso ao pedido." });
    }
    try {
      await simulatePayment(params.order_id);
    } catch (error) {
      if (isRedirect(error)) {
        throw error;
      }
      const message =
        error instanceof Error
          ? error.message
          : "Não foi possível simular agora.";
      return fail(409, { message });
    }
    redirect(303, resolve("/orders/[order_id]", { order_id: params.order_id }));
  },
  cancel: async ({ cookies, params }) => {
    const access = orderAccess(cookies, params.order_id);
    if (!access) {
      return fail(401, { message: "Este navegador não tem acesso ao pedido." });
    }
    try {
      await cancelOrder(params.order_id, access);
    } catch (error) {
      if (isRedirect(error)) {
        throw error;
      }
      const message =
        error instanceof Error
          ? error.message
          : "Não foi possível cancelar agora.";
      return fail(409, { message });
    }
    redirect(303, resolve("/orders/[order_id]", { order_id: params.order_id }));
  },
};
