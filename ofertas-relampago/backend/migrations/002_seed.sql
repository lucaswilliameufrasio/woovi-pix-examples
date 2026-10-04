INSERT INTO offers (id, title, price_cents, total_units, available_units, reservation_ttl_seconds)
VALUES ('demo-offer', 'Sacola surpresa — demonstração local', 2500, 1, 1, 120)
ON CONFLICT (id) DO NOTHING;
