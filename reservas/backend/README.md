# Backend de Reservas

API Go `/v1`, PostgreSQL independente e worker durável para expiração de holds e eventos de pagamento sintéticos. O contrato é `openapi.yaml`.

## Requisitos e execução local

- Go definido em `mise.toml` (usar `mise exec`), PostgreSQL 18 e `btree_gist` instalável no schema `public`.
- Configure `DATABASE_URL`, `DEMO_OPERATOR_TOKEN` e `DEMO_SIMULATOR_TOKEN` com valores aleatórios locais de pelo menos 32 caracteres.
- `LISTEN_ADDR` é opcional e deve ser loopback; padrão `127.0.0.1:8084`.
- `mise exec -- go run ./cmd/server`

O arquivo `compose.yaml` publica Postgres apenas em loopback e exige `RESERVAS_DB_PASSWORD`; não contém credencial padrão. Veja `docs/reservas-local.md` para preparar o ambiente e rodar validação.

## Domínio e garantias

- Um recurso seedado (`demo-room`), preço demo R$ 120, sessão de 30 min, buffer de 15 min, grade de 15 min, seg–sex 09:00–17:00 em `America/Sao_Paulo`, hold de 5 min e disponibilidade de 14 dias.
- API retorna instantes UTC RFC3339, `local_label` com offset explícito e fuso IANA. Horários inexistentes não aparecem; ocorrências DST repetidas são instantes distintos. Atendimentos que cruzam mudança de offset não são oferecidos.
- `reservations_no_overlap` é exclusion constraint no banco para `[starts_at, ends_at + buffer)` de `held/confirmed`; request concorrente não depende de mutex em processo.
- Estado da reserva (`held`, `confirmed`, `completed`, `expired`, `cancelled`) é separado do pagamento (`pending`, `paid`, `expired`, `cancelled`, `payment_exception`). Pagamento atrasado não reabre slot.
- Capability aleatória é persistida apenas como SHA-256; ID isolado não autoriza acesso. Operador e simulador usam bearer local server-side.
- O worker persiste confirmação antes do ACK HTTP (`202`), deduplica por `Idempotency-Key` e processa/reconcilia após restart.
- Não existe chamada a PSP, QR code ou payload Pix.

## Testes

`TEST_DATABASE_URL` precisa apontar a PostgreSQL real. A suíte cria schema aleatório por teste e o remove no cleanup. Exemplo: `mise exec -- go test -race -count=3 ./...`.
