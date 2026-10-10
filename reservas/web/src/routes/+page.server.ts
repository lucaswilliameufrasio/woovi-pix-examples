import { fail, isRedirect, redirect } from "@sveltejs/kit";
import type { Actions, PageServerLoad } from "./$types";
import { createHold, getAvailability } from "#lib/server/api";

function localToday(): string {
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone: "America/Sao_Paulo",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).formatToParts(new Date());
  const values = new Map(parts.map((part) => [part.type, part.value]));
  return `${values.get("year")}-${values.get("month")}-${values.get("day")}`;
}

export const load: PageServerLoad = async ({ url }) => {
  const date = url.searchParams.get("date") ?? localToday();
  try {
    return {
      date,
      availability: await getAvailability(date),
      unavailable: false,
    };
  } catch (error) {
    return {
      date,
      availability: undefined,
      unavailable: true,
      message: error instanceof Error ? error.message : "Agenda indisponível.",
    };
  }
};

export const actions: Actions = {
  reserve: async ({ request, cookies }) => {
    const form = await request.formData();
    const startsAt = form.get("starts_at");
    if (
      typeof startsAt !== "string" ||
      !Number.isFinite(Date.parse(startsAt))
    ) {
      return fail(400, { message: "Escolha um horário válido." });
    }
    try {
      const created = await createHold(startsAt);
      cookies.set(`reservation_${created.reservation.id}`, created.capability, {
        path: `/reservations/${created.reservation.id}`,
        httpOnly: true,
        secure: false,
        sameSite: "strict",
        maxAge: 60 * 60 * 24 * 14,
      });
      redirect(303, `/reservations/${created.reservation.id}`);
    } catch (error) {
      if (isRedirect(error)) {
        throw error;
      }
      return fail(409, {
        message:
          error instanceof Error ? error.message : "Não foi possível reservar.",
      });
    }
  },
};
