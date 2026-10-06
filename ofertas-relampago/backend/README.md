# Ofertas-relâmpago — backend local

Veja o [roteiro completo local](../../docs/local.md), o [contrato HTTP](../../docs/offers-contract.md) e os [limites sandbox](../../docs/sandbox.md). Para validar o fluxo em schemas isolados sem reset, execute `python3 tooling/run_offers_smoke.py` da raiz após subir o PostgreSQL.

O operador também pode emitir uma sessão local de checkout sem QR em `POST /v1/operator/orders/{order_id}/checkout`, com `Idempotency-Key` e sem corpo. Sua credencial curta permite somente `GET /v1/checkout-sessions/{checkout_id}`; não é checkout Pix SDK completo. Detalhes, estados e limites no contrato.

Novas reservas em `POST /v1/orders` recebem capability de pedido uma única vez (`order_access_token`, `order_token_expires_at`), hash persistido atomicamente com a reserva e TTL de vinte minutos. Esse Bearer consulta `/v1/customer/orders/{order_id}` e emite `/checkout`, sem ser operador. BFF web guarda capability em cookie HttpOnly. Não a imprima/coloque em URL/histórico; GET público legado não reemite token. Isso é posse local da reserva, não login de produção.

Demo educativa de checkout para uma oferta de estoque limitado. O banco e o simulador são locais; não há credencial nem chamada para Woovi. Não use para cobrar dinheiro real.

## Requisitos

- Go 1.27.1
- Docker Compose
- `curl`
- `jq` para projetar apenas campos públicos nos exemplos de terminal

## Executar

Inicie PostgreSQL e API no primeiro terminal. O worker roda em processo independente no segundo terminal.

```sh
docker compose -p woovi-offers up -d --wait postgres
export DATABASE_URL='postgres://offers:offers-local-only@127.0.0.1:55440/offers?sslmode=disable'
export TEST_DATABASE_URL="$DATABASE_URL"
export DEMO_MODE=true
export DEMO_OPERATOR_TOKEN="$(openssl rand -hex 32)" # secreto local; nunca commitar
mise exec -- go run ./cmd/offers
```

No segundo terminal, use o mesmo `DATABASE_URL` e `DEMO_MODE`; não é necessário compartilhar o token de operador:

```sh
cd ofertas-relampago/backend
export DATABASE_URL='postgres://offers:offers-local-only@127.0.0.1:55440/offers?sslmode=disable'
export DEMO_MODE=true
mise exec -- go run ./cmd/offers-worker
```

A API fica em `http://127.0.0.1:8080`; simulador em `http://127.0.0.1:8081`. Ajuste `API_ADDR`/`SIMULATOR_ADDR` se essas portas estiverem ocupadas. Use `:0` para obter portas efêmeras (os endereços escolhidos aparecem nos logs e o próprio processo configura o endereço interno da API para o simulador). A oferta seed `demo-offer` tem uma unidade. O modo demo habilita rotas `/dev`; não as habilite em deployments reais. API, simulador e worker são processos independentes; todos usam o mesmo banco local e permanecem isolados do restante do monorepo. O Dockerfile inclui `/offers` e `/offers-worker`, com a API como entrypoint padrão; para o worker, substitua o entrypoint por `/offers-worker`.
A API e o simulador exigem `DEMO_MODE=true` e só aceitam bind em IP loopback (`127.0.0.1` ou `::1`); esta versão não suporta deployment nem chamadas PSP reais.

## Fluxo HTTP

```sh
curl -s http://127.0.0.1:8080/v1/offers
curl -sS -X POST http://127.0.0.1:8080/v1/orders \
  -H 'content-type: application/json' -d '{"offer_id":"demo-offer"}' | jq '{id,amount_cents,state}'
# use o ID retornado (ORDER_ID); valor é congelado no servidor
curl -i -X POST http://127.0.0.1:8081/v1/charges \
  -H 'content-type: application/json' -d '{"order_id":"ORDER_ID"}'
curl -i -X POST http://127.0.0.1:8081/v1/charges/ORDER_ID/pay
curl -s http://127.0.0.1:8080/v1/operator/orders/ORDER_ID \
  -H "authorization: Bearer $DEMO_OPERATOR_TOKEN"
curl -s http://127.0.0.1:8080/v1/operator/orders \
  -H "authorization: Bearer $DEMO_OPERATOR_TOKEN"
# após confirmação local, emita token de retirada e valide-o uma única vez:
curl -i -X POST http://127.0.0.1:8080/v1/operator/orders/ORDER_ID/pickup-token \
  -H "authorization: Bearer $DEMO_OPERATOR_TOKEN"
curl -i -X POST http://127.0.0.1:8080/v1/operator/orders/ORDER_ID/pickup \
  -H "authorization: Bearer $DEMO_OPERATOR_TOKEN" -H 'content-type: application/json' \
  -d '{"pickup_token":"TOKEN_RETORNADO"}'
```

Todas as rotas HTTP usam prefixo `/v1`. Campos JSON e parâmetros usam `snake_case`. Erros seguem `{ "message", "error_code", "extra"? }`; lógica cliente usa `error_code` estável. `400 MALFORMED_REQUEST` é para request malformada; `413 PAYLOAD_TOO_LARGE` indica corpo acima do limite; `422 INVALID_PARAMS` inclui `extra.validation_errors[]` para payload semanticamente inválido; 404/409 usam códigos específicos (`409` cobre também transição em conflito com estado atual), 412 fica reservado a pré-condições de negócio não satisfeitas, 502 classifica falha local e 500 sempre `UNEXPECTED_ERROR` genérico. Cenários de webhook/pagamento só são registrados com `DEMO_MODE=true`; API e leitura/criação da cobrança sintética local seguem disponíveis no modo local.

Consulta por ID sem credencial foi encerrada: `GET /v1/orders/{order_id}` é agora alias de `/v1/customer/orders/{order_id}`, ambos exigem capability de pedido e retornam 401 para ausência/expiração/outro pedido. Operador usa `GET /v1/operator/orders/{order_id}`, inclusive para pedidos antigos/sem capability; essa consulta não emite token de cliente. Todas essas consultas são `no-store`. O exemplo de terminal projeta só campos públicos da reserva com `jq`; não imprima credenciais. Para possuir o pedido como cliente, prefira web/mobile ou tooling que mantém a capability em armazenamento apropriado.

Simulator `/v1/charges` deriva o valor do pedido pelo backend e persiste a cobrança no PostgreSQL; é idempotente por `order_id`, inclusive após reinício. `pay` atualiza o status e envia evento pago ao endpoint local; o backend persiste e deduplica o evento antes do HTTP 202 e um worker processa o evento. O worker reserva eventos com lease durável, recupera leases vencidos e agenda retry exponencial limitado para falhas transitórias. Há também um job periódico de expiração. Para exercitar webhook duplicado, repetir `POST /v1/dev/webhooks/paid` com o mesmo `event_key`; reusar a chave em outro pedido conflita. `POST /v1/dev/scenarios/duplicate_paid/ORDER_ID` simula reenvio do evento pago; `POST /v1/dev/webhooks/ORDER_ID/late_paid` simula notificação tardia. Cenários `/v1/dev` só funcionam com `DEMO_MODE=true`. Falta de estoque usa `412 OFFER_UNAVAILABLE`; repetição de transição terminal usa `409 ORDER_NOT_ELIGIBLE`.

Retirada local: rotas `/v1/operator/*` exigem `Authorization: Bearer $DEMO_OPERATOR_TOKEN`; configure segredo aleatório de pelo menos 32 caracteres antes de iniciar (`openssl rand -hex 32`). Não há valor padrão. O operador emite `POST /v1/operator/orders/{order_id}/pickup-token` somente após pagamento confirmado. O token aleatório é retornado uma vez, apenas seu hash fica armazenado e emitir outro substitui o anterior. `POST /v1/operator/orders/{order_id}/pickup` consome o token atomicamente e marca `picked_up_at`; token inválido, pedido não pago e retirada repetida são recusados. Rotas de operador são gated por `DEMO_MODE` e o serviço permanece loopback-only; ainda é um protótipo sem gestão de identidade/usuários, não use em produção.

Para simular timeout incerto após a cobrança ser criada, use `POST /v1/dev/scenarios/timeout?order_id=ORDER_ID` (retorna 504, mas persiste a cobrança). Reconcilie consultando `GET /v1/charges/ORDER_ID` ou repetindo `POST /v1/charges/ORDER_ID/retry`; ambos retornam a mesma cobrança por referência idempotente. Rotas que simulam pagamento, retry ou webhooks e cenários `/v1/dev` só são registradas quando `DEMO_MODE=true`; fora desse modo retornam 404. Esse fluxo é apenas do simulador local.

## Testes

Com PostgreSQL real disponível:

```sh
TEST_DATABASE_URL="$DATABASE_URL" go test -p=1 -count=3 -race ./...
go build ./...
go build ./cmd/offers-worker
gofmt -l .
golangci-lint run ./...
```

Testes de integração criam IDs únicos e limpam somente seus registros. O teste concorrente libera 24 clientes juntos contra a última unidade e verifica um pedido vencedor no PostgreSQL. Também testa expiração, corrida entre expiração e pagamento, worker e deduplicação HTTP/evento.

Use `-p=1` enquanto os pacotes de teste compartilham o mesmo banco: workers consultam a fila global e, com pacotes simultâneos, podem consumir eventos de outra fixture. Isso serializa pacotes, não os clientes/workers concorrentes exercitados dentro dos testes. `TEST_DATABASE_URL` deve estar definido; sem ele os testes PostgreSQL são ignorados. Isolar banco/schema por pacote continua pendente antes de habilitar execução paralela de suítes.

## Limites desta fatia

Autenticação de operador é um Bearer secret local, sem gestão de identidade/usuários. Assinatura/autenticidade de webhook PSP e integração com Woovi/SDK/MCP não estão implementadas. O simulador persiste cobranças sintéticas no banco local e não é um adapter PSP real. Não é uma integração de pagamentos pronta para produção. O simulador inclui cenários básicos para timeout/rate limit/atraso e timeout pós-criação reconciliável localmente por referência.

## Interfaces da demo

A interface web fica em `../web` (SvelteKit) e o app em `../mobile` (Flutter/FVM). Ambos consultam este backend local. As telas públicas apenas listam ofertas e criam pedidos; não iniciam pagamento. Inicie API e banco seguindo este README antes de executar qualquer interface.

O app mobile também referencia o package `woovi_pix_flutter` do checkout irmão `../woovi-pix-flutter-sdk`. A sessão local já associa pedido/valor e valida capability, mas checkout Pix não está habilitado: faltam payload legítimo e adapter PSP/`ChargeCreator`. Não usar o endpoint de sessão genérico do sample como substituto.
