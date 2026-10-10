# Reservas

Demo independente de agenda e retenção de horários. O backend define preço, fuso, slots, hold, estados e autorização. O PostgreSQL possui banco/schema próprio; a UI web e Flutter nunca decidem elegibilidade nem autenticam usando somente o ID.

- [Backend e API](backend/README.md)
- [Web/BFF](web/README.md)
- [Flutter](mobile/README.md)
- [Contrato OpenAPI](backend/openapi.yaml)
- [Roteiro local](../docs/reservas-local.md)

Tudo é didático e loopback-only. Pagamento é sintético e não cria payload Pix nem chama PSP.
