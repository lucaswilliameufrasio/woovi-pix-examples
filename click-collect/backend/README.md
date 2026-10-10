# API Click & collect

API local do Balcão, prefixo `/v1`, JSON e nomes `snake_case`. Erros: `{ "message", "error_code" }`. Timestamps são ISO-8601 UTC. Os pedidos usam IDs opacos; conhecer o ID não autoriza consulta.

## Rotas

| Método e rota | Acesso | Resultado |
| --- | --- | --- |
| `GET /v1/products` | Público local | Catálogo e disponibilidade atual. |
| `POST /v1/orders` | Público local | Body `{ "product_id": "house-cake" }`; reserva uma unidade e devolve o pedido, `order_access_token` e `pickup_code` uma única vez. |
| `GET /v1/orders/{order_id}` | `Authorization: Bearer <order_access_token>` | Estado do próprio pedido. ID isolado recebe `401/404`. |
| `DELETE /v1/orders/{order_id}` | Capability do pedido | Cancela somente pagamento pendente; libera a unidade atomicamente. Repetição é idempotente. |
| `POST /v1/orders/{order_id}/demo-payment` | Capability do pedido | Simulação local idempotente; persiste cobrança/evento e responde `202`. Não é autorização de pagamento real. |
| `POST /v1/simulator/orders/{order_id}/charge` | `X-Demo-Token` | Cria cobrança sintética pelo preço autoritativo do pedido. |
| `POST /v1/simulator/orders/{order_id}/confirm` | `X-Demo-Token` | Persiste evento sintético; o worker aplica-o depois. `202` significa aceito para processamento, não pagamento concluído. |
| `GET /v1/operator/orders` | `X-Demo-Token` da loja | Fila limitada a pedidos confirmados. |
| `POST /v1/operator/orders/{order_id}/ready` | Token da loja | Marca pronto após a cozinha entrar em preparo. |
| `POST /v1/operator/orders/{order_id}/pickup` | Token da loja + `{ "pickup_code": "..." }` | Confere o código e consome a retirada uma única vez. |
| `GET /healthz` | Público local | `204` se o banco responde. |

O worker é parte do processo API e reprocessa eventos duráveis usando locks PostgreSQL; expira reservas pendentes em lotes limitados. Reiniciar API não descarta eventos confirmados. Tokens de operador e simulador são segredos didáticos do processo local, não identidade humana ou autenticação de produção.

## Máquinas de estado

- Pagamento: `pending → paid | expired | cancelled`; confirmação após `expired`/`cancelled` resulta em `payment_exception`.
- Fulfillment: `awaiting_payment → preparing → ready_for_pickup → picked_up`.
- Só `paid` pode preparar; só `ready_for_pickup` pode retirar. Pagamento tardio não repõe estoque outra vez nem libera retirada.

## Validação

`mise exec -- go test ./...` compila todos os testes; testes PostgreSQL são omitidos se `TEST_DATABASE_URL` estiver ausente. Para exercitá-los, siga o guia local e rode com `TEST_DATABASE_URL` apontando para o PostgreSQL exclusivo desta demo. O harness cria schemas de teste aleatórios e os remove ao terminar; não use URL de outro projeto.

O contrato HTTP versionado está em `openapi.yaml` e deve permanecer alinhado com os handlers.
