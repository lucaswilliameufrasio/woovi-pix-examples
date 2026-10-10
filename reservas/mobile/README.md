# Flutter — Reservas

Aplicativo Flutter independente. O backend gera a agenda e permanece autoritativo. A capability privada fica em Flutter Secure Storage; histórico/ID local não autenticam. POST com timeout tem resultado incerto e não é repetido automaticamente. O simulador permanece server-side na web/operador.

Use FVM: `fvm install`, `fvm flutter pub get`, `fvm flutter analyze`, `fvm flutter test` e `fvm flutter build apk --debug`. A API aceita somente loopback. Para Android físico, configure `adb reverse tcp:8084 tcp:8084` e `--dart-define=API_BASE_URL=http://127.0.0.1:8084`; para o emulador Android use `http://10.0.2.2:8084` não é aceito pelo guard loopback atual — use `adb reverse` no emulador também. Testes unitários verificam parsing/loopback e timeout de criação como resultado incerto sem retry.

Teste de dispositivo: `fvm flutter drive --driver=test_driver/integration_test.dart --target=integration_test/app_test.dart -d <serial> --dart-define=API_BASE_URL=http://127.0.0.1:8084` com API e PostgreSQL locais ativos e `adb reverse` instalado. Sem chamada PSP/pagamento real.
