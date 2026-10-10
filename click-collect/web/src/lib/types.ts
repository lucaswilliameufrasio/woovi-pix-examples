export type Product = {
  id: string;
  title: string;
  description: string;
  price_cents: number;
  available_units: number;
  reservation_ttl_seconds: number;
};

export type Order = {
  id: string;
  product_id: string;
  product_title: string;
  amount_cents: number;
  payment_state:
    "pending" | "paid" | "expired" | "cancelled" | "payment_exception";
  fulfillment_state:
    "awaiting_payment" | "preparing" | "ready_for_pickup" | "picked_up";
  expires_at: string;
  created_at: string;
  picked_up_at?: string;
};

export type ReservedOrder = Order & {
  order_access_token: string;
  pickup_code: string;
};

export type ApiError = {
  message: string;
  error_code: string;
};
