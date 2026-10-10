# Click & collect: execução local

Comandos Bash partem da raiz do checkout. Necessários: mise, Docker Compose, FVM e um Android Emulator para a interface mobile. O serviço Go só escuta `127.0.0.1`; a web também deve ficar em loopback. Nenhum passo usa conta Woovi, Pix pagável, PSP, sandbox ou deploy.

## Preparar segredos locais e PostgreSQL

Uma vez, crie segredos aleatórios só para o ambiente local. O arquivo fica ignorado pelo Git (`click-collect/backend/.gitignore`):

```bash
cd click-collect/backend
umask 077
DB_PASSWORD="$(openssl rand -hex 32)"
OPERATOR_TOKEN="$(openssl rand -hex 32)"
SIMULATOR_TOKEN="$(openssl rand -hex 32)"
cat > .env <<EOF
CLICK_COLLECT_DB_PASSWORD=$DB_PASSWORD
CLICK_COLLECT_DB_PORT=55441
DEMO_OPERATOR_TOKEN=$OPERATOR_TOKEN
DEMO_SIMULATOR_TOKEN=$SIMULATOR_TOKEN
EOF
unset DB_PASSWORD OPERATOR_TOKEN SIMULATOR_TOKEN
docker compose up -d --wait postgres
```

Se a porta `55441` estiver ocupada, edite `CLICK_COLLECT_DB_PORT` no arquivo `.env` para uma porta loopback livre. Não use nem copie dados de conexão dos outros projetos. O volume PostgreSQL fica preservado ao parar o serviço; não use `down -v` para “limpar” dados.

## Iniciar API e worker

Terminal 1:

```bash
cd click-collect/backend
set -a
. ./.env
set +a
export DATABASE_URL="postgres://click_collect:$CLICK_COLLECT_DB_PASSWORD@127.0.0.1:$CLICK_COLLECT_DB_PORT/click_collect?sslmode=disable"
export LISTEN_ADDR=127.0.0.1:8082
mise exec -- go run ./cmd/server
```

O startup aplica a migration/seed e inicia API, expirador e processador de eventos persistidos. Não imprima o conteúdo do `.env`. `Ctrl-C` encerra o listener e o worker de forma limitada.

## Web e operador local

Terminal 2:

```bash
cd click-collect/backend
set -a
. ./.env
set +a
cd ../web
mise exec -- npm ci
API_BASE_URL=http://127.0.0.1:8082 mise exec -- npm run dev -- --host 127.0.0.1 --port 5174 --strictPort
```

Abra `http://127.0.0.1:5174`. A reserva guarda capability e código em cookies HttpOnly/SameSite, isolados por pedido. O painel local `/loja` usa o token de operador apenas no servidor web; não é login humano e não deve ser exposto em rede.

Na página de pedido, **Simular confirmação local** aciona o simulador persistido no backend. Isso não cria QR/brCode nem cobrança pagável. O status é consultado no backend; o painel permite marcar pronto e conferir/consumir o código de retirada.

## Android

Com o serviço/API/web ligados e um emulador Android:

```bash
cd click-collect/mobile
fvm install
fvm flutter pub get
fvm flutter run --dart-define=API_BASE_URL=http://10.0.2.2:8082
```

O endpoint `10.0.2.2` é a ponte do Android Emulator ao loopback do host. Para outro simulador, informe uma URL local explícita; nunca use o IP público nem abra o backend em `0.0.0.0`. HTTP sem TLS só é habilitado no manifest **debug** Android. Capability e código ficam em Flutter Secure Storage e são reconsultados do backend; IDs ou histórico não autorizam acesso.

No app, reserve, use a ação de simulação local e atualize o estado. A tela mobile é de cliente; a fila da loja permanece no BFF web.

## Testes e limites

```bash
# backend, dentro de click-collect/backend
mise exec -- go test ./...
mise exec -- go vet ./...
mise exec -- go build ./...

# web, dentro de click-collect/web
mise exec -- npm run format:check
mise exec -- npm run check
mise exec -- npm run lint
mise exec -- npm test
mise exec -- npm run build
mise exec -- npm audit --audit-level=high

# mobile, dentro de click-collect/mobile
fvm dart format --output=none --set-exit-if-changed lib test
fvm flutter analyze
fvm flutter test
fvm flutter build apk --debug
```

Com Android Emulator conectado, o teste real do cliente também pode ser executado com API/DB na máquina host:

```bash
cd click-collect/mobile
fvm flutter drive --driver=test_driver/integration_test.dart \
  --target=integration_test/app_test.dart \
  -d emulator-5554 \
  --dart-define=API_BASE_URL=http://10.0.2.2:8082
```

Em aparelho Android físico conectado por ADB, encaminhe só a porta local da API e use o loopback do aparelho; remova o encaminhamento ao terminar:

```bash
adb -s <serial> reverse tcp:8082 tcp:8082
cd click-collect/mobile
fvm flutter drive --driver=test_driver/integration_test.dart \
  --target=integration_test/app_test.dart \
  -d <serial> \
  --dart-define=API_BASE_URL=http://127.0.0.1:8082
adb -s <serial> reverse --remove tcp:8082
```

### Browser E2E com backend e PostgreSQL reais

Com `psql` instalado, o container PostgreSQL desta demo healthy e `npm ci` executado, no terminal 1 (Bash), carregue `.env` sem imprimi-lo e exporte a URL do **banco click_collect**:

```bash
cd click-collect/backend
set -a
. ./.env
set +a
export TEST_DATABASE_URL="postgres://click_collect:$CLICK_COLLECT_DB_PASSWORD@127.0.0.1:$CLICK_COLLECT_DB_PORT/click_collect?sslmode=disable"
cd ../..
python3 tooling/run_click_collect_smoke.py --browser
```

O runner exige URL loopback para o banco exclusivo `click_collect`/`click_collect_test`, cria dois schemas aleatórios e isolados, inicia API + BFF local, roda Chromium e remove apenas esses schemas/processos próprios. O fluxo verifica cookie privado sem reaproveitar ID, simulador local, preparo, código e retirada única. Não registra trace, cookie ou token do browser. Instale o browser fixado, se necessário, de dentro de `click-collect/web` com `mise exec -- npx --no-install playwright install chromium`.

Os testes de integração Go também exigem `TEST_DATABASE_URL` para este banco; fixtures criam schemas aleatórios separados. O fluxo HTTP verifica catálogo, capability, simulador, worker, fila, pronto e retirada única. O `integration_test` Flutter percorre reserva privada e confirmação até preparo com backend real; a retirada é coberta no browser com o painel da loja. iOS/Keychain requer macOS e não foi validado. Autenticação humana, recuperação de credencial, rate limiting/abuso, observabilidade, backups, OpenAPI/MCP, E2E nativo iOS e qualquer integração PSP são trabalho posterior; não use como sistema de loja real.
