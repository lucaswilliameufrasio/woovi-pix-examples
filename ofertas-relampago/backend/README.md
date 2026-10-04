# Ofertas-relâmpago — backend local

Demo educativa de checkout para uma oferta de estoque limitado. O banco e o simulador são locais; não há credencial nem chamada para Woovi. Não use para cobrar dinheiro real.

## Requisitos

- Go 1.27.1
- Docker Compose
- `curl`

## Executar

```sh
docker compose -p woovi-offers up -d --wait postgres
export DATABASE_URL='postgres://offers:offers-local-only@127.0.0.1:55440/offers?sslmode=disable'
export TEST_DATABASE_URL="$DATABASE_URL"
export DEMO_MODE=true
go run ./cmd/offers
```

A API fica em `http://127.0.0.1:8080`; simulador em `http://127.0.0.1:8081`. Ajuste `API_ADDR`/`SIMULATOR_ADDR` se essas portas estiverem ocupadas. Use `:0` para obter portas efêmeras (os endereços escolhidos aparecem nos logs e o próprio processo configura o endereço interno da API para o simulador). A oferta seed `demo-offer` tem uma unidade. O modo demo habilita rotas `/dev`; não as habilite em deployments reais. API e simulador são independentes do restante do monorepo.
A API e o simulador exigem `DEMO_MODE=true` e só aceitam bind em IP loopback (`127.0.0.1` ou `::1`); esta versão não suporta deployment nem chamadas PSP reais.

## Fluxo HTTP

```sh
curl -s http://127.0.0.1:8080/v1/offers
curl -i -X POST http://127.0.0.1:8080/v1/orders \
  -H 'content-type: application/json' -d '{"offer_id":"demo-offer"}'
# use o ID retornado (ORDER_ID); valor é congelado no servidor
curl -i -X POST http://127.0.0.1:8081/v1/charges \
  -H 'content-type: application/json' -d '{"order_id":"ORDER_ID"}'
curl -i -X POST http://127.0.0.1:8081/v1/charges/ORDER_ID/pay
curl -s http://127.0.0.1:8080/v1/orders/ORDER_ID
curl -s http://127.0.0.1:8080/v1/operator/orders
```

Todas as rotas HTTP usam prefixo `/v1`. Campos JSON e parâmetros usam `snake_case`. Erros seguem `{ "message", "error_code", "extra"? }`; lógica cliente usa `error_code` estável. `400 MALFORMED_REQUEST` é para request malformada; `413 PAYLOAD_TOO_LARGE` indica corpo acima do limite; `422 INVALID_PARAMS` inclui `extra.validation_errors[]` para payload semanticamente inválido; 404/409 usam códigos específicos, 502 classifica falha local e 500 sempre `UNEXPECTED_ERROR` genérico. Ambos servidores registram suas rotas versionadas somente em `DEMO_MODE=true`; APIs externas/produção não estão suportadas neste tracer bullet.

Simulator `/v1/charges` deriva o valor do pedido pelo backend e persiste a cobrança no PostgreSQL; é idempotente por `order_id`, inclusive após reinício. `pay` atualiza o status e envia evento pago ao endpoint local; o backend persiste e deduplica o evento antes do HTTP 202 e um worker processa o evento. O worker reserva eventos com lease durável, recupera leases vencidos e agenda retry exponencial limitado para falhas transitórias. Há também um job periódico de expiração. Para exercitar webhook duplicado, repetir `POST /v1/dev/webhooks/paid` com o mesmo `event_key`; reusar a chave em outro pedido conflita. `POST /v1/dev/scenarios/duplicate_paid/ORDER_ID` simula reenvio do evento pago; `POST /v1/dev/webhooks/ORDER_ID/late_paid` simula notificação tardia. Cenários `/v1/dev` só funcionam com `DEMO_MODE=true`.

Para simular timeout incerto após a cobrança ser criada, use `POST /v1/dev/scenarios/timeout?order_id=ORDER_ID` (retorna 504, mas persiste a cobrança). Reconcilie consultando `GET /v1/charges/ORDER_ID` ou repetindo `POST /v1/charges/ORDER_ID/retry`; ambos retornam a mesma cobrança por referência idempotente. Rotas que simulam pagamento, retry ou webhooks e cenários `/v1/dev` só são registradas quando `DEMO_MODE=true`; fora desse modo retornam 404. Esse fluxo é apenas do simulador local.

## Testes

Com PostgreSQL real disponível:

```sh
TEST_DATABASE_URL="$DATABASE_URL" go test -count=1 -race ./...
go build ./...
gofmt -l .
golangci-lint run ./...
```

Testes de integração criam IDs únicos e limpam somente seus registros. O teste concorrente libera 24 clientes juntos contra a última unidade e verifica um pedido vencedor no PostgreSQL. Também testa expiração, corrida entre expiração e pagamento, worker e deduplicação HTTP/evento.

## Limites desta fatia

Ainda não implementados: autenticação de operador/retirada, assinatura/autenticidade de webhook PSP, timeout incerto reconciliável e integração com Woovi/SDK/MCP. O simulador persiste cobranças sintéticas no banco local e não é um adapter PSP real. Não é uma integração de pagamentos pronta para produção. O simulador inclui cenários básicos para timeout/rate limit/atraso, mas timeout pós-criação ainda não reconcilia por referência e permanece pendente.
