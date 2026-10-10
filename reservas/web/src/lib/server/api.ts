export type Slot = {
  starts_at: string;
  ends_at: string;
  local_label: string;
  time_zone: string;
};

export type Availability = {
  resource_id: string;
  resource_name: string;
  time_zone: string;
  price_cents: number;
  duration_minutes: number;
  buffer_minutes: number;
  slots: Slot[];
};

export type Reservation = {
  id: string;
  resource_id: string;
  starts_at: string;
  ends_at: string;
  local_label: string;
  time_zone: string;
  amount_cents: number;
  reservation_state: string;
  payment_state: string;
  expires_at: string;
};

function baseURL(): URL {
  const value = process.env.API_BASE_URL ?? "http://127.0.0.1:8084";
  const url = new URL(value);
  if (
    url.protocol !== "http:" ||
    !["127.0.0.1", "[::1]"].includes(url.hostname) ||
    url.port.length === 0 ||
    url.username.length > 0 ||
    url.password.length > 0 ||
    url.pathname !== "/" ||
    url.search.length > 0 ||
    url.hash.length > 0
  ) {
    throw new Error("A API deve usar um endereço explícito de loopback.");
  }
  return url;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

function isReservation(value: unknown): value is Reservation {
  return (
    isRecord(value) &&
    typeof value.id === "string" &&
    typeof value.resource_id === "string" &&
    typeof value.starts_at === "string" &&
    Number.isFinite(Date.parse(value.starts_at)) &&
    typeof value.ends_at === "string" &&
    Number.isFinite(Date.parse(value.ends_at)) &&
    typeof value.local_label === "string" &&
    typeof value.time_zone === "string" &&
    typeof value.amount_cents === "number" &&
    Number.isSafeInteger(value.amount_cents) &&
    typeof value.reservation_state === "string" &&
    ["held", "confirmed", "completed", "expired", "cancelled"].includes(
      value.reservation_state,
    ) &&
    typeof value.payment_state === "string" &&
    ["pending", "paid", "expired", "cancelled", "payment_exception"].includes(
      value.payment_state,
    ) &&
    typeof value.expires_at === "string" &&
    Number.isFinite(Date.parse(value.expires_at))
  );
}

async function request(path: string, init?: RequestInit): Promise<unknown> {
  let response: Response;
  try {
    response = await fetch(new URL(path, baseURL()), {
      ...init,
      signal: AbortSignal.timeout(5000),
      headers: { "Content-Type": "application/json", ...init?.headers },
    });
  } catch {
    throw new Error("Não foi possível conectar ao backend local.");
  }
  if (response.status === 202 || response.status === 204) {
    return undefined;
  }
  let body: unknown;
  try {
    body = await response.json();
  } catch {
    throw new Error("O backend retornou uma resposta inválida.");
  }
  if (!response.ok) {
    if (isRecord(body) && typeof body.message === "string") {
      throw new Error(body.message);
    }
    throw new Error("Não foi possível concluir a solicitação.");
  }
  return body;
}

export async function getAvailability(date: string): Promise<Availability> {
  const result = await request(
    `/v1/availability?date=${encodeURIComponent(date)}`,
  );
  if (
    !isRecord(result) ||
    typeof result.resource_id !== "string" ||
    typeof result.resource_name !== "string" ||
    typeof result.time_zone !== "string" ||
    typeof result.price_cents !== "number" ||
    typeof result.duration_minutes !== "number" ||
    typeof result.buffer_minutes !== "number" ||
    !Array.isArray(result.slots)
  ) {
    throw new Error("A disponibilidade retornada é inválida.");
  }
  const slots: Slot[] = [];
  for (const candidate of result.slots) {
    if (
      !isRecord(candidate) ||
      typeof candidate.starts_at !== "string" ||
      typeof candidate.ends_at !== "string" ||
      typeof candidate.local_label !== "string" ||
      typeof candidate.time_zone !== "string"
    ) {
      throw new Error("Um horário retornado é inválido.");
    }
    slots.push({
      starts_at: candidate.starts_at,
      ends_at: candidate.ends_at,
      local_label: candidate.local_label,
      time_zone: candidate.time_zone,
    });
  }
  return {
    resource_id: result.resource_id,
    resource_name: result.resource_name,
    time_zone: result.time_zone,
    price_cents: result.price_cents,
    duration_minutes: result.duration_minutes,
    buffer_minutes: result.buffer_minutes,
    slots,
  };
}

export async function createHold(
  startsAt: string,
): Promise<{ reservation: Reservation; capability: string }> {
  const result = await request("/v1/reservations", {
    method: "POST",
    body: JSON.stringify({ starts_at: startsAt }),
  });
  if (
    !isRecord(result) ||
    !isReservation(result.reservation) ||
    typeof result.capability !== "string"
  ) {
    throw new Error("A reserva retornada é inválida.");
  }
  return { reservation: result.reservation, capability: result.capability };
}

export async function getReservation(
  id: string,
  capability: string,
): Promise<Reservation> {
  const result = await request(`/v1/reservations/${encodeURIComponent(id)}`, {
    headers: { Authorization: `Bearer ${capability}` },
  });
  if (!isReservation(result)) {
    throw new Error("A reserva retornada é inválida.");
  }
  return result;
}

export async function cancelReservation(
  id: string,
  capability: string,
): Promise<Reservation> {
  const result = await request(
    `/v1/reservations/${encodeURIComponent(id)}/cancel`,
    {
      method: "POST",
      headers: { Authorization: `Bearer ${capability}` },
    },
  );
  if (!isReservation(result)) {
    throw new Error("A reserva retornada é inválida.");
  }
  return result;
}

export async function simulatePaid(id: string): Promise<void> {
  const token = process.env.DEMO_SIMULATOR_TOKEN;
  if (!token || token.length < 32) {
    throw new Error("O simulador local não está configurado.");
  }
  await request(`/v1/simulator/reservations/${encodeURIComponent(id)}/paid`, {
    method: "POST",
    headers: {
      Authorization: `Bearer ${token}`,
      "Idempotency-Key": crypto.randomUUID(),
    },
  });
}

export async function listOperatorReservations(): Promise<Reservation[]> {
  const token = process.env.DEMO_OPERATOR_TOKEN;
  if (!token || token.length < 32) {
    throw new Error("O operador local não está configurado.");
  }
  const result = await request("/v1/operator/reservations", {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!Array.isArray(result)) {
    throw new Error("A lista de reservas do operador é inválida.");
  }
  const reservations: Reservation[] = [];
  for (const candidate of result) {
    if (!isReservation(candidate)) {
      throw new Error("Uma reserva do operador é inválida.");
    }
    reservations.push(candidate);
  }
  return reservations;
}

export async function completeReservation(id: string): Promise<void> {
  const token = process.env.DEMO_OPERATOR_TOKEN;
  if (!token || token.length < 32) {
    throw new Error("O operador local não está configurado.");
  }
  await request(
    `/v1/operator/reservations/${encodeURIComponent(id)}/complete`,
    {
      method: "POST",
      headers: { Authorization: `Bearer ${token}` },
    },
  );
}
