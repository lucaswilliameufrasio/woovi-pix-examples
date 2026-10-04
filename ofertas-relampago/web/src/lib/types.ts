export type Offer = {
  id: string;
  title: string;
  price_cents: number;
  available_units: number;
  reservation_ttl_seconds: number;
};

export type DemoOrder = {
  id: string;
  offer_id: string;
  amount_cents: number;
  state: "pending_payment" | "paid" | "expired" | "payment_exception";
  expires_at: string;
};

export type ApiError = {
  message: string;
  error_code: string;
  extra?: Record<string, unknown>;
};
