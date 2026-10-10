import { error, fail } from "@sveltejs/kit";
import type { Actions, PageServerLoad } from "./$types";
import {
  cancelReservation,
  getReservation,
  simulatePaid,
} from "#lib/server/api";

export const load: PageServerLoad = async ({ cookies, params }) => {
  const capability = cookies.get(`reservation_${params.reservation_id}`);
  if (!capability) {
    error(404, "Reserva não encontrada.");
  }
  try {
    return {
      reservation: await getReservation(params.reservation_id, capability),
    };
  } catch {
    error(404, "Reserva não encontrada.");
  }
};

export const actions: Actions = {
  cancel: async ({ cookies, params }) => {
    const capability = cookies.get(`reservation_${params.reservation_id}`);
    if (!capability) {
      return fail(404, { message: "Reserva não encontrada." });
    }
    try {
      return {
        reservation: await cancelReservation(params.reservation_id, capability),
      };
    } catch (cause) {
      return fail(409, {
        message:
          cause instanceof Error ? cause.message : "Não foi possível cancelar.",
      });
    }
  },
  simulate: async ({ cookies, params }) => {
    const capability = cookies.get(`reservation_${params.reservation_id}`);
    if (!capability) {
      return fail(404, { message: "Reserva não encontrada." });
    }
    try {
      await simulatePaid(params.reservation_id);
      return {
        reservation: await getReservation(params.reservation_id, capability),
      };
    } catch (cause) {
      return fail(409, {
        message:
          cause instanceof Error ? cause.message : "Não foi possível simular.",
      });
    }
  },
  refresh: async ({ cookies, params }) => {
    const capability = cookies.get(`reservation_${params.reservation_id}`);
    if (!capability) {
      return fail(404, { message: "Reserva não encontrada." });
    }
    try {
      return {
        reservation: await getReservation(params.reservation_id, capability),
      };
    } catch (cause) {
      return fail(404, {
        message:
          cause instanceof Error ? cause.message : "Reserva não encontrada.",
      });
    }
  },
};
