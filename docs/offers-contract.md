# Contrato e estados de ofertas

## Autoridade e invariantes

Servidor congela `amount_cents` ao reservar; clientes não enviam preço. PostgreSQL decrementa estoque atomicamente e libera uma vez ao expirar. Pedido e cobrança têm IDs distintos; neste simulador a cobrança é idempotente por `order_id`.

```text
pending_payment ── evento pago processado antes do prazo ──> paid
pending_payment ── expirer ──> expired
pending_payment/expired ── pagamento após prazo ──> payment_exception

paid + credencial válida + operador autenticado ──> picked_up_at preenchido
```

Pagamento e retirada não são o mesmo estado. `paid` permanece após retirada, com timestamp separado. `payment_exception` registra pagamento que não autoriza entrega; exige atendimento manual. Não há estorno automático. Credencial de retirada aleatória é retornada uma vez, armazenada como SHA-256, rotacionada e consumida atomicamente.

## API de ofertas — porta 8080

| Método/rota                                        | Sucesso              | Observação                                                            |
| -------------------------------------------------- | -------------------- | --------------------------------------------------------------------- |
| GET `/v1/offers`                                   | 200 array            | Catálogo pequeno seed                                                 |
| POST `/v1/orders`                                  | 201 pedido           | `{offer_id}`; 412 esgotado, 404 oferta ausente                        |
| GET `/v1/orders/{order_id}`                        | 200 pedido           | Alias autenticado customer; capability de pedido, nunca ID sozinho    |
| GET `/v1/operator/orders/{order_id}`               | 200 pedido           | Bearer operador; diagnóstico sem reemitir capability cliente          |
| GET `/v1/operator/orders`                          | 200 array            | Bearer local; sem paginação de produção                               |
| POST `/v1/operator/orders/{order_id}/pickup-token` | 201 `{pickup_token}` | Só pago e ainda não retirado                                          |
| POST `/v1/operator/orders/{order_id}/pickup`       | 200 pedido           | `{pickup_token}`; operação única                                      |
| POST `/v1/operator/orders/{order_id}/paid`         | 200 pedido           | Controle local operador; não substitui webhook de teste ponta a ponta |
| POST `/v1/dev/webhooks/paid`                       | 202 evento aceito    | `{order_id,event_key}`; persistência/dedup antes do ACK               |
| POST `/v1/operator/demo/reset`                     | 200                  | Destrutivo; só instância descartável e ação explícita                 |

Operador exige `DEMO_MODE=true` e Bearer de pelo menos 32 caracteres; não tem gestão de usuários/sessões. Consultas de pedido exigem capability ou operador na rota própria, com `no-store`. Conhecer ID não autoriza consulta. Ainda não há identidade humana de produção: não exponha API/BFF publicamente.

### Capability do pedido — acesso do cliente local

`POST /v1/orders` emite, na mesma transação da reserva, `order_access_token` aleatório de 256 bits e `order_token_expires_at` de vinte minutos. Token é retornado **somente na criação**, não na consulta. Banco guarda apenas SHA-256; erro de reserva reverte estoque e grant juntos. Pedidos antigos não recebem credencial por ID.

- `GET /v1/customer/orders/{order_id}`: Bearer de pedido; retorna pedido autoritativo sem credenciais. Ausência, token errado/de outro pedido ou expirado: 401 `ORDER_UNAUTHORIZED`.
- `POST /v1/customer/orders/{order_id}/checkout`: mesmo Bearer, `Idempotency-Key` e sem corpo, com os mesmos estados/idempotência da emissão do operador. Credencial é revalidada depois da espera nos locks PostgreSQL, antes de qualquer commit.
- Token de pedido não autoriza operador; token de checkout não consulta o pedido na rota de cliente nem emite outra sessão. Não há rotação/refresh por ID. Perda de resposta à criação pode também perder acesso: não repetir reserva às cegas; atendimento pelo operador local.
- Web mantém token de pedido em cookie HttpOnly restrito ao path, SameSite=Strict e prazo do banco. Consulta e emissão usam fetch nativo server-only, sem serializar resposta contendo credencial para hidratação. O segredo de operador não participa desse fluxo.

Capability autentica posse da reserva, **não uma pessoa**. `GET /v1/orders/{order_id}` existe somente como alias autenticado da rota customer, com os mesmos escopo/TTL/401; não há consulta pública nem reemissão por ID. Web/mobile usam rota customer. Pedido antigo/sem capability pode ser diagnosticado pelo operador em sua rota própria, sem gerar token cliente. Identidade completa continua pendente; manter loopback.

### Sessão local restrita — sem QR/Pix

- `POST /v1/operator/orders/{order_id}/checkout`, com Bearer do operador e **uma** `Idempotency-Key` ASCII de 16–128 caracteres, sem espaços. **Sem corpo**: preço/identidade enviados pelo cliente são recusados.
- Primeira emissão: HTTP 201. Replay: HTTP 200, mesmo `checkout_id`/`charge_id`, com nova credencial de leitura. Chave reutilizada para outro pedido: 409 `IDEMPOTENCY_CONFLICT`. Chaves diferentes para o mesmo pedido continuam apontando para uma única sessão/cobrança.
- Resposta: `checkout_id`, `order_id`, `charge_id`, `amount_cents`, `status`, `order_state`, `charge_status`, `expires_at`, `currency: "BRL"`, `mode: "local_simulation"`, `access_token`, `token_expires_at`. **Não contém** `pix_copy_paste`.
- `GET /v1/checkout-sessions/{checkout_id}`, com `Bearer access_token`: somente consulta, sem retornar/renovar token. Token tem 256 bits, hash SHA-256 no banco, escopo de uma sessão e TTL de dez minutos. Sessão ausente/token inválido/expirado retornam o mesmo 401 `CHECKOUT_UNAUTHORIZED`. Respostas são `Cache-Control: no-store`.
- Emissão grava cobrança simulada, sessão, chave (hash) e credencial numa única transação PostgreSQL. Não faz POST a PSP nem reserva estoque novamente. Erro antes do commit não deixa uma tentativa parcial. Depois de timeout de rede, preserve a mesma chave e consulte/reemita por ela — não crie outro pedido.
- Grants anteriores continuam válidos até seu prazo, para não invalidar consultas concorrentes. Replay não prolonga esses grants nem o prazo do pedido; só o operador pode emitir outro grant curto.
- Pedido `paid`/`expired` sem sessão anterior não inicia checkout. Sessão já existente pode ser recuperada com estado terminal. Cobrança financeira paga antes do worker não torna sessão `paid`. Prazo vencido retorna `status=expired` mesmo antes do tick de expiração.
- `payment_exception` retorna 412 `PAYMENT_EXCEPTION` com `extra.order_state`, nunca `paid`. O operador deve tratar a exceção; token de leitura não autoriza retirada nem operação de operador.

**Limite do SDK:** `HttpCheckoutTransport` usa a mesma rota de consulta, mas `CheckoutSession.fromJson` exige código Pix quando `pending`. Esta sessão local deliberadamente sem código **não é uma sessão Pix completa compatível**. Não contorne o parser com uma string fictícia. UI SDK e identidade de produção permanecem pendentes. Operador ou capability de pedido autorizam emissão local, não representam login de cliente.

## Simulador próprio — porta 8081

| Método/rota                                        | Comportamento                                                      |
| -------------------------------------------------- | ------------------------------------------------------------------ |
| POST `/v1/charges`                                 | `{order_id}` → 200 com id/order_id/amount_cents/status/order_state |
| GET `/v1/charges/{order_id}`                       | Consulta cobrança persistida                                       |
| POST `/v1/charges/{order_id}/retry`                | Recupera mesma cobrança/ref                                        |
| POST `/v1/charges/{order_id}/pay`                  | Registra financeiro e entrega evento HTTP ao backend               |
| POST `/v1/dev/scenarios/timeout?order_id=...`      | Persiste cobrança e retorna 504                                    |
| POST `/v1/dev/scenarios/rate-limit`                | 429 com Retry-After                                                |
| POST `/v1/dev/scenarios/delay`                     | Resposta atrasada sintética; não cria pagamento                    |
| POST `/v1/dev/scenarios/duplicate_paid/{order_id}` | Mesmo caminho de pay; terminal concluído pode retornar 409         |
| POST `/v1/dev/webhooks/{order_id}/late_paid`       | Envia evento tardio; testar após expiração                         |

Rotas de cenário/mutação exigem modo demo; não são API PSP de produção. `pay` 200 indica evento aceito, não worker finalizado. Estado financeiro `paid` pode coexistir com pedido `payment_exception`. Não há QR/código Pix gerado nesta implementação.

## Erros e recuperação

Erros JSON: `{message: string, error_code: string, extra?: object}`. `message` é humano; fluxo usa `error_code`. Campos internos/provider secrets nunca são repassados.

- 400 `MALFORMED_REQUEST`; 413 `PAYLOAD_TOO_LARGE`.
- 422 `INVALID_PARAMS`, com `extra.validation_errors`.
- 404 `OFFER_NOT_FOUND` / `ORDER_NOT_FOUND` / `CHARGE_NOT_FOUND`.
- 401 `OPERATOR_UNAUTHORIZED` / `CHECKOUT_UNAUTHORIZED` / `ORDER_UNAUTHORIZED`; 412 `OFFER_UNAVAILABLE` / `PICKUP_NOT_ELIGIBLE` / `CHECKOUT_NOT_ELIGIBLE` / `PAYMENT_EXCEPTION`.
- 409 `IDEMPOTENCY_CONFLICT`, `ORDER_NOT_ELIGIBLE`, `PICKUP_TOKEN_INVALID`, `PICKUP_ALREADY_DONE`.
- 502 `DEPENDENCY_REQUEST`; 500 `UNEXPECTED_ERROR` genérico.

Timeout após POST tem resultado incerto. Reconciliar pela referência existente; não repetir criação com outro pedido/chave. Worker usa lease/retry; clientes consultam backend. Não apagar tentativa/evento para esconder falha.

Esta referência descreve implementação local; OpenAPI completo, contrato merchant PSP com identidade cliente e assinaturas webhook PSP ainda estão pendentes.
