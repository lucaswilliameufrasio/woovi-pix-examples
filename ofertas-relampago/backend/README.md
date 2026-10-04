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
curl -s http://127.0.0.1:8080/offers
curl -i -X POST http://127.0.0.1:8080/orders \
  -H 'content-type: application/json' -d '{"offer_id":"demo-offer"}'
# use o ID retornado (ORDER_ID); valor é congelado no servidor
curl -i -X POST http://127.0.0.1:8081/charges \
  -H 'content-type: application/json' -d '{"order_id":"ORDER_ID"}'
curl -i -X POST http://127.0.0.1:8081/charges/ORDER_ID/pay
curl -s http://127.0.0.1:8080/orders/ORDER_ID
curl -s http://127.0.0.1:8080/operator/orders
```

Simulator `/charges` deriva o valor do pedido pelo backend e é idempotente por `order_id`. `pay` envia evento pago ao endpoint local; o backend persiste e deduplica o evento antes do HTTP 202 e um worker processa o evento. O worker reserva eventos com lease durável, recupera leases vencidos e agenda retry exponencial limitado para falhas transitórias. Há também um job periódico de expiração. Para exercitar webhook duplicado, repetir `POST /dev/webhooks/paid` com o mesmo `event_key`; reusar a chave em outro pedido conflita. `POST /dev/webhooks/{orderID}/late-paid` simula notificação tardia. Os eventos `/dev/scenarios` e `/dev/webhooks` só estão disponíveis com `DEMO_MODE=true`.

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

Ainda não implementados: autenticação de operador/retirada, assinatura/autenticidade de webhook PSP, retry/backoff de falhas do worker, timeout incerto reconciliável e integração com Woovi/SDK/MCP. O simulador em memória não persiste cobranças entre reinícios. Não é uma integração de pagamentos pronta para produção. O simulador inclui cenários básicos para timeout/rate limit/atraso, mas timeout pós-criação ainda não reconcilia por referência e permanece pendente.
