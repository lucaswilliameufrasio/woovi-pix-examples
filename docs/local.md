# Executar e testar sem sandbox

Comandos em Bash, partindo da raiz de `woovi-pix-examples`, salvo indicação contrária. Não precisa de conta Woovi, AppID, CPF/CNPJ ou dinheiro. O simulador desta demo não chama a Woovi.

## 1. Preparar ferramentas e SDK

- Docker com Compose; mise; FVM; Python 3 para o smoke; `curl` e `jq` para os exemplos manuais.
- `mise install` na raiz instala Go/Node nas versões de `mise.toml`.
- Faça checkout de `woovi-pix-flutter-sdk` ao lado deste repositório, na versão compatível com o app. A dependência Flutter é por caminho relativo, não um pacote publicado.
- Dentro de `ofertas-relampago/mobile`, execute `fvm install` para a versão de `.fvmrc` e `fvm flutter pub get`.

Não coloque credenciais Woovi em nenhum destes comandos. Os dados de conexão abaixo são didáticos, exclusivamente do PostgreSQL local.

## 2. API, banco, seed e worker — terminal 1

```sh
mise install
cd ofertas-relampago/backend
docker compose -p woovi-offers up -d --wait postgres
export DATABASE_URL='postgres://offers:offers-local-only@127.0.0.1:55440/offers?sslmode=disable'
export DEMO_MODE=true
# Gere/guarde um token aleatório (≥32 caracteres) no gerenciador local.
# Informe o mesmo valor nos terminais de testes, sempre sem eco.
read -rsp 'Token local do operador: ' DEMO_OPERATOR_TOKEN
export DEMO_OPERATOR_TOKEN
mise exec -- go run ./cmd/offers
```

API `127.0.0.1:8080`; simulador persistido `127.0.0.1:8081`; banco `127.0.0.1:55440`. Migrações e seed são aplicadas no startup. API, worker de pagamento (250 ms) e expiração (1 s) rodam no mesmo processo nesta implementação. Ainda não há comando de worker independente.

A seed `demo-offer` tem **uma unidade, R$ 25,00 e TTL de 120 segundos**. Restart não repõe estoque. Para portas ocupadas, configure `API_ADDR` e `SIMULATOR_ADDR` com IP loopback literal e porta; `:0` escolhe portas efêmeras, mostradas nos logs. Atualize as URLs dos clientes conforme os endereços escolhidos.

## 3. Escolher uma interface — terminal 2

### Web

Da raiz:

```sh
cd ofertas-relampago/web
mise exec -- npm ci
API_BASE_URL=http://127.0.0.1:8080 mise exec -- npm run dev -- --host 127.0.0.1 --port 5173 --strictPort
```

Abra `http://127.0.0.1:5173`, reserve e abra **Acompanhar pedido** no mesmo navegador. Anote o ID. Cookie HttpOnly de pedido autoriza a página por vinte minutos; o link sozinho em outro navegador não concede acesso. **Abrir sessão local sem Pix** cria uma sessão idempotente ligada ao pedido, não código Pix/pagamento. Após a confirmação pelo simulador no próximo passo, clique **Atualizar estado**. A consulta é manual; não há polling web, QR Pix ou botão de pagamento implementado.

### Android

Da raiz, com emulador disponível:

```sh
cd ofertas-relampago/mobile
fvm install
fvm flutter pub get
fvm flutter run --dart-define=API_BASE_URL=http://10.0.2.2:8080
```

Crie um pedido e veja seu estado; **Meus pedidos** guarda até 20 IDs no aparelho e os reconsulta somente com capability válida no armazenamento seguro. ID sozinho não autoriza acesso; prazo de vinte minutos. O pedido ativo tem polling, pausa em background e backoff em falhas; 401 encerra polling e remove confirmação antiga. HTTP é permitido apenas no Android debug. SDK incluído, mas checkout Pix desabilitado.

Web e mobile disputam a mesma última unidade: não reserve nas duas interfaces esperando dois pedidos. Para comparar, use instâncias/bancos isolados ou complete um teste e prepare uma nova fixture.

Em dispositivo Android físico conectado por USB, uma alternativa local é `adb reverse tcp:8080 tcp:8080` e `API_BASE_URL=http://127.0.0.1:8080`. Não abra o backend em `0.0.0.0` para contornar isolamento. Para iOS no mesmo Mac, a URL é loopback; build/ATS e execução iOS continuam sem validação neste projeto.

## 4. Confirmar o pedido sem dinheiro — terminal 3

Para criar pelo HTTP em vez de uma interface, execute **uma vez** e capture o ID:

```sh
API=http://127.0.0.1:8080
SIM=http://127.0.0.1:8081
ORDER_ID="$(curl --fail-with-body -sS "$API/v1/orders" \
  -H 'Content-Type: application/json' -d '{"offer_id":"demo-offer"}' | jq -er '.id')"
```

Se criou na web/mobile, **não execute novamente o POST**: defina `ORDER_ID` com o ID mostrado na interface. Então:

```sh
curl --fail-with-body -sS "$SIM/v1/charges" \
  -H 'Content-Type: application/json' -d "{\"order_id\":\"$ORDER_ID\"}"
curl --fail-with-body -sS -X POST "$SIM/v1/charges/$ORDER_ID/pay"
# Bash: use o mesmo token de operador fornecido ao backend, sem eco.
read -rsp 'Token local do operador: ' DEMO_OPERATOR_TOKEN
export DEMO_OPERATOR_TOKEN
export API_BASE_URL="$API" ORDER_ID
python3 - <<'PY'
import os
from tooling.smoke import Client
api = Client(os.environ['API_BASE_URL'])
order = api.request('GET', '/v1/operator/orders/' + os.environ['ORDER_ID'],
                    token='Bearer ' + os.environ['DEMO_OPERATOR_TOKEN'])
print('Estado autoritativo:', order['state'])
PY
```

Criação da cobrança retorna HTTP 200 com valor derivado do pedido e referência idempotente. `pay` confirma somente a **aceitação do evento**: webhook persiste/deduplica, worker processa depois. Consulte até `state=paid`; não entregue apenas porque `pay` respondeu 200. Se a reserva expirou, espere `payment_exception`, nunca autorização de retirada.

Execute o helper Python na raiz. Repita somente a consulta, não a reserva/cobrança, para observar o worker. Consulta por ID sem credencial agora retorna 401, inclusive no alias legado `/v1/orders/{order_id}`. Web/mobile consultam com sua própria capability; operador não recupera/reemite credencial de cliente pelo ID.

Se executar o fluxo manual em outro terminal, defina `API`/`SIM` nele também. Não imprima/copiei o segredo do operador no chat, nem use `curl -v` com autenticação.

## 5. Retirada autenticada

Use o mesmo `DEMO_OPERATOR_TOKEN` fornecido ao processo da API, obtido no seu gerenciador local por entrada oculta; não coloque o valor literal em comandos/histórico. O token não fica no browser nem no app. A ferramenta de smoke isolado abaixo gera seu próprio token em memória e dispensa esse transporte manual.

```sh
# Bash: quando necessário, entrada local sem eco.
read -rsp 'Token local do operador: ' DEMO_OPERATOR_TOKEN
export DEMO_OPERATOR_TOKEN
export API_BASE_URL="$API" ORDER_ID
python3 - <<'PY'
import os
from tooling.smoke import Client
api = Client(os.environ['API_BASE_URL'])
path = '/v1/operator/orders/' + os.environ['ORDER_ID']
operator = 'Bearer ' + os.environ['DEMO_OPERATOR_TOKEN']
credential = api.request('POST', path + '/pickup-token', expected=201, token=operator)
picked = api.request('POST', path + '/pickup', body=credential, token=operator)
print('Retirada registrada em:', picked['picked_up_at'])
PY
```

Execute este bloco na raiz do repositório. O helper envia a credencial somente no header/corpo correto e não imprime seu valor. Repetir retirada com o mesmo token é recusado. O smoke abaixo também verifica rotação e reuso.

## 6. Smoke automatizado HTTP real

### Opção recomendada: fixture isolada automática

Com o PostgreSQL do passo 2 healthy, da raiz:

```sh
python3 tooling/run_offers_smoke.py
```

Com `npm ci` já executado na web, também teste o BFF real:

```sh
python3 tooling/run_offers_smoke.py --web
```

Essa opção sobe SvelteKit em loopback/porta efêmera, valida cookie HttpOnly/SameSite/path e ausência de tokens no HTML, recusa acesso apenas por link, recupera a mesma sessão e percorre worker/consulta/retirada. Não injeta segredo de operador no processo web. É teste HTTP/SSR, não Playwright nem execução de JavaScript no browser.

Com dependências Flutter via FVM já instaladas, valide também o cliente mobile:

```sh
python3 tooling/run_offers_smoke.py --mobile
```

Dois schemas exclusivos, cliente Flutter com HTTP/DB reais, reserva/consulta privada/restart e SQL; armazenamento seguro da plataforma substituído. Não é execução de UI/dispositivo nem teste nativo de Keystore/Keychain. `--web` e `--mobile` são modos separados. O mobile agora exige capability criada no mesmo app/origem: ID/histórico antigo não autoriza consulta; perda/expiração de acesso requer operador, não reserva repetida.

O runner compila o backend com mise, cria **schema exclusivo** de teste no PostgreSQL local, gera token em memória e inicia API/simulador em portas efêmeras. Não toca pedidos de outros schemas. Executa o smoke duas vezes em schemas separados e encerra seus próprios processos. Mantém os schemas de teste para inspeção; não reseta nem apaga dados. Isso funciona mesmo com a seed do banco principal já consumida.

Se seu PostgreSQL da demo usa outro projeto Compose/porta, informe `SMOKE_COMPOSE_PROJECT` e `SMOKE_DATABASE_PORT`. O runner usa somente `127.0.0.1` e as credenciais didáticas documentadas; não aceita URL de banco remoto.

### Opção manual: instância que você já iniciou

Use uma instância isolada com seed livre e o token já disponível no ambiente do shell. O smoke cria um pedido e consome uma unidade; **não reseta/apaga dados**. Não o execute depois de já ter reservado a seed no mesmo banco.

```sh
API_BASE_URL=http://127.0.0.1:8080 \
SIMULATOR_BASE_URL=http://127.0.0.1:8081 \
python3 tooling/smoke.py offers --ack-local-write
```

Resultado esperado: `PASS offers`, após testar ausência de token, estoque, preço, emissão/replay de sessão restrita sem QR, cobrança repetida, webhook/worker, pagamento terminal, rotação e retirada única. O token curto da sessão permite apenas sua consulta, não operação de operador. Veja o [contrato](offers-contract.md#sessão-local-restrita--sem-qrpix). Não é E2E de browser/dispositivo nem checkout SDK integrado. Só aceita IPs loopback literais, recusa redirects e não usa proxy de ambiente. Falhas não exibem tokens/corpos sensíveis nem repetem automaticamente POST.

## 7. Cenários de falha

- **Última unidade:** segundo `POST /v1/orders` recebe `412 OFFER_UNAVAILABLE` e não cria cobrança.
- **Timeout incerto:** `POST $SIM/v1/dev/scenarios/timeout?order_id=$ORDER_ID` retorna 504 após persistir cobrança. Consulte `GET $SIM/v1/charges/$ORDER_ID` e compare ID/valor; `POST .../retry` recupera a mesma referência. Não crie outro pedido para “resolver” timeout.
- **Duplicidade:** após worker concluir, repetir `pay` retorna `409 ORDER_NOT_ELIGIBLE`. O teste de dedup real da API usa a mesma `event_key` em `/v1/dev/webhooks/paid` e verifica um único efeito.
- **Expiração:** crie um pedido em fixture livre, espere mais de 120 s + tick do expirer, consulte `expired`; envio posterior de `pay` após criar cobrança deve resultar em `payment_exception`. Não autoriza retirada nem faz estorno.
- **Restart:** pare apenas o servidor e inicie com o mesmo banco/token/config. Consulte pedido e cobrança; seus IDs/status persistem. Não remova volumes.
- **Rede:** pare backend e consulte web/histórico mobile; devem informar falha, sem tratar cache como estado atual.

## 8. Testes e parada segura

```sh
python3 -m unittest discover -s tooling -p 'test_*.py'
```

Para lint/format do tooling, com `uv` instalado, use versões fixadas:

```sh
uvx ruff@0.14.10 check tooling
uvx ruff@0.14.10 format --check tooling
```

Backend, dentro de `ofertas-relampago/backend`:

```sh
TEST_DATABASE_URL="$DATABASE_URL" mise exec -- go test -race -p=1 -count=3 ./...
mise exec -- go build ./...
mise exec -- go vet ./...
gofmt -l .
golangci-lint run ./...
```

Rode suites PostgreSQL numa instância de teste sem API/worker externo simultâneo: eles consumiriam a mesma fila. Sem `TEST_DATABASE_URL`, integração é ignorada e não foi validada. `-p=1` evita competição entre pacotes, não remove testes concorrentes internos. Web/mobile: comandos completos nos respectivos READMEs.

Interrompa servidores/apps com Ctrl+C. Dentro de `ofertas-relampago/backend`, `docker compose -p woovi-offers stop postgres` para parar sem apagar dados. O reset de demo remove pedidos/eventos/cobranças: somente por decisão explícita do operador em instância descartável, nunca para contornar pagamento incerto. Este roteiro não o executa.

## Troubleshooting

| Sintoma                        | Verificação                                                                           |
| ------------------------------ | ------------------------------------------------------------------------------------- |
| API não inicia                 | Banco healthy, `DEMO_MODE=true`, token ≥32 caracteres, bind loopback e portas livres. |
| SDK não encontrado             | Checkout irmão na posição exigida pelo `pubspec.yaml`; rode FVM pub get.              |
| Oferta esgotada                | Seed só tem uma unidade e não é reposta no restart; não resete silenciosamente.       |
| `401 OPERATOR_UNAUTHORIZED`    | Token do shell deve ser o mesmo do processo backend; não imprima valores.             |
| `pending_payment` após `pay`   | Worker ainda processando ou falha persistida; consulte/logs sanitizados.              |
| `payment_exception`            | Prazo expirou; atendimento manual, sem retirada/estorno automático.                   |
| Mobile não acessa API          | Emulador Android usa `10.0.2.2`; dispositivo físico pode usar adb reverse.            |
| Porta 8081 ocupada             | Simuladores ofertas e MCP usam essa porta; não suba ambos nela.                       |
| `adapter_missing` no build web | Compilação passa, mas destino deploy ainda não foi definido.                          |
