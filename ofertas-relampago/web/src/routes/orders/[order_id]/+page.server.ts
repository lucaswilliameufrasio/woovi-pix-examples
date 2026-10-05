import { error, fail } from "@sveltejs/kit";
import { createHash } from "node:crypto";
import type { Actions, PageServerLoad } from "./$types";
import type { ApiError } from "../../../lib/types";
import {
  apiError,
  backendRequest,
  credential,
  parseOrder,
  record,
} from "../../../lib/server/backend";
import { orderCookie } from "../../../lib/server/order-access";

function requireOrderId(orderId: string): void {
  if (!/^[A-Za-z0-9_-]{1,200}$/.test(orderId)) {
    error(404, "Pedido não encontrado.");
  }
}

export const load = (async ({
  params,
  cookies,
}: Pick<Parameters<PageServerLoad>[0], "params"> & {
  cookies: Pick<Parameters<PageServerLoad>[0]["cookies"], "get">;
}) => {
  requireOrderId(params.order_id);
  const token = cookies.get(orderCookie(params.order_id));
  if (!credential(token)) {
    error(
      401,
      "Acesso ao pedido indisponível neste navegador ou expirado. O ID não autoriza consulta.",
    );
  }
  let result: Awaited<ReturnType<typeof backendRequest>>;
  try {
    result = await backendRequest(
      globalThis.fetch,
      `/v1/customer/orders/${encodeURIComponent(params.order_id)}`,
      {
        headers: {
          accept: "application/json",
          authorization: `Bearer ${token}`,
        },
      },
    );
  } catch {
    return {
      order: undefined,
      lookup_error: {
        message:
          "Não foi possível consultar o pedido. Tente atualizar; seu estado não foi alterado.",
        error_code: "DEPENDENCY_REQUEST",
      } satisfies ApiError,
    };
  }
  if (result.response.status === 401) {
    error(
      401,
      "A credencial do pedido expirou ou foi revogada. Não crie outro pedido para recuperar este acesso.",
    );
  }
  if (result.response.status === 404) {
    error(404, "Pedido não encontrado.");
  }
  const order =
    result.response.status === 200
      ? parseOrder(result.body, params.order_id)
      : undefined;
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

export const actions = {
  checkout: async ({
    params,
    cookies,
  }: Pick<Parameters<NonNullable<Actions[string]>>[0], "params"> & {
    cookies: Pick<
      Parameters<NonNullable<Actions[string]>>[0]["cookies"],
      "get"
    >;
  }) => {
    requireOrderId(params.order_id);
    const token = cookies.get(orderCookie(params.order_id));
    if (!credential(token)) {
      return fail(401, {
        message: "A credencial do pedido não está disponível neste navegador.",
        error_code: "ORDER_UNAUTHORIZED",
      });
    }
    try {
      const { response, body } = await backendRequest(
        globalThis.fetch,
        `/v1/customer/orders/${encodeURIComponent(params.order_id)}/checkout`,
        {
          method: "POST",
          headers: {
            authorization: `Bearer ${token}`,
            "Idempotency-Key": `web-checkout-${createHash("sha256").update(params.order_id).digest("hex")}`,
          },
        },
      );
      if (!response.ok) {
        return fail(
          response.status,
          apiError(body, "Não foi possível abrir a sessão local."),
        );
      }
      if (
        ![200, 201].includes(response.status) ||
        !record(body) ||
        body.order_id !== params.order_id ||
        typeof body.checkout_id !== "string" ||
        !/^[a-f0-9-]{36}$/.test(body.checkout_id) ||
        body.mode !== "local_simulation" ||
        !credential(body.access_token) ||
        "pix_copy_paste" in body
      ) {
        return fail(502, {
          message:
            "Resposta de sessão inválida; ela pode ter sido criada. Use este mesmo botão para recuperar a mesma referência, sem outra reserva.",
          error_code: "DEPENDENCY_INVALID_RESPONSE",
        });
      }
      // No checkout credential goes into action data, cookies or SSR markup.
      // The order capability is enough to recover this idempotent local resource.
      return {
        checkout: { checkout_id: body.checkout_id, mode: "local_simulation" },
      };
    } catch {
      return fail(502, {
        message:
          "A API não respondeu; a sessão pode ter sido criada. Use este mesmo botão para recuperar a mesma referência, sem outra reserva.",
        error_code: "DEPENDENCY_REQUEST",
      });
    }
  },
} satisfies Actions;
