import { fail, isRedirect, redirect } from "@sveltejs/kit";
import { resolve } from "$app/paths";
import type { Actions, PageServerLoad } from "./$types";
import { listProducts, reserveProduct } from "#lib/server/backend";
import { saveOrderAccess } from "#lib/server/order-access";
import type { ReservedOrder } from "#lib/types";

export const load: PageServerLoad = async () => {
  try {
    return { products: await listProducts(), unavailable: false };
  } catch {
    return { products: [], unavailable: true };
  }
};

export const actions: Actions = {
  reserve: async ({ request, cookies }) => {
    const form = await request.formData();
    const productID = form.get("product_id");
    if (typeof productID !== "string" || !/^[a-z0-9-]{1,80}$/.test(productID)) {
      return fail(400, { message: "Escolha um produto válido." });
    }
    let order: ReservedOrder;
    try {
      order = await reserveProduct(productID);
    } catch (error) {
      if (isRedirect(error)) {
        throw error;
      }
      const message =
        error instanceof Error
          ? error.message
          : "Não foi possível reservar agora.";
      return fail(409, { message });
    }
    saveOrderAccess(
      cookies,
      order.id,
      order.order_access_token,
      order.pickup_code,
    );
    redirect(303, resolve("/orders/[order_id]", { order_id: order.id }));
  },
};
