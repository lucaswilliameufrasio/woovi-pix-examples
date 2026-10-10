# Reservas — desenvolvimento e validação local

Esta demo usa processos e banco independentes. Execute os comandos a partir da raiz do repositório.

## Banco e API

1. Gere uma senha local, mantenha-a fora do repositório e inicie o PostgreSQL:

   ```sh
   export RESERVAS_DB_PASSWORD="$(openssl rand -hex 32)"
   docker compose -f reservas/backend/compose.yaml up -d --wait
   ```

2. Configure a URL do banco e tokens aleatórios locais (não os coloque no app):

   ```sh
   export DATABASE_URL="postgres://reservations:${RESERVAS_DB_PASSWORD}@127.0.0.1:55443/reservations?sslmode=disable"
   export DEMO_OPERATOR_TOKEN="$(openssl rand -hex 32)"
   export DEMO_SIMULATOR_TOKEN="$(openssl rand -hex 32)"
   ```

3. Inicie a API em outro terminal com os mesmos valores:

   ```sh
   cd reservas/backend
   mise exec -- go run ./cmd/server
   ```

Ela escuta apenas `127.0.0.1:8084`. A migration instala `btree_gist` no schema PostgreSQL `public`, portanto a role local precisa poder instalar a extensão.

## Web e Android

- Web: `cd reservas/web && mise exec -- npm ci && mise exec -- npm run dev`. Encaminhe `API_BASE_URL` e os dois tokens no ambiente do processo server-side.
- Android físico/emulador: `adb reverse tcp:8084 tcp:8084`; então `cd reservas/mobile` e `fvm flutter run --dart-define=API_BASE_URL=http://127.0.0.1:8084`.
- Remova o encaminhamento ao terminar: `adb reverse --remove tcp:8084`.

## Suítes reais e E2E

Configure `TEST_DATABASE_URL` apontando a um banco de teste isolado no loopback; testes Go criam schemas aleatórios. Execute:

```sh
cd reservas/backend && mise exec -- go test -race -count=3 ./... && mise exec -- go vet ./... && mise exec -- go build ./...
cd ../../reservas/web && mise exec -- npm ci && mise exec -- npm run format:check && mise exec -- npm run lint && mise exec -- npm run check && mise exec -- npm run build
cd ../mobile && fvm flutter pub get && fvm flutter analyze && fvm flutter test && fvm flutter build apk --debug
```

Com PostgreSQL de teste ativo, `psql` instalado e Chromium Playwright disponível, rode `python3 tooling/run_reservations_smoke.py`; ele cria e remove um schema isolado, inicia API/BFF loopback e executa uma jornada Chromium. O teste Android também pode ser executado com `flutter drive` conforme `reservas/mobile/README.md`.

## Limites

Todos os pagamentos são eventos sintéticos, sem payload Pix, rede PSP, sandbox ou ambiente remoto. A referência navegável Scalar fica em `/api-reference`, servindo o OpenAPI canônico em `/openapi.yaml`. Não reutilize os tokens/banco/servidor para outras demos. Ainda não há política de feriados, múltiplos recursos ou estorno real.
