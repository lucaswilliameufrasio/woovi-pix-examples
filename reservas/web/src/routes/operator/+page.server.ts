import { fail } from "@sveltejs/kit";
import type { Actions, PageServerLoad } from "./$types";
import { completeReservation, listOperatorReservations } from "#lib/server/api";

export const load: PageServerLoad = async () => {
  try {
    return { reservations: await listOperatorReservations() };
  } catch (error) {
    return {
      reservations: [],
      unavailable:
        error instanceof Error ? error.message : "Operador indisponível.",
    };
  }
};

export const actions: Actions = {
  complete: async ({ request }) => {
    const form = await request.formData();
    const id = form.get("reservation_id");
    if (typeof id !== "string" || !/^[0-9a-f-]{36}$/.test(id)) {
      return fail(400, { message: "Reserva inválida." });
    }
    try {
      await completeReservation(id);
      return { completed: id };
    } catch (error) {
      return fail(409, {
        message:
          error instanceof Error ? error.message : "Não foi possível concluir.",
      });
    }
  },
};
