# Ofertas-relâmpago — app Flutter

App de demonstração local da oferta limitada. Usa Flutter 3.47.6 via FVM e consome a API `/v1`; não gera Pix nem realiza pagamentos. O package `woovi_pix_flutter` é resolvido do checkout local irmão `../woovi-pix-flutter-sdk/packages/woovi_pix_flutter` (repositório `woovi-pix-flutter-sdk` ao lado deste repo).

## Executar

Na raiz do repositório, Go é gerenciado pelo mise; dentro do app, Flutter é gerenciado pelo FVM:

```sh
cd ofertas-relampago/mobile
fvm flutter pub get
fvm flutter run --dart-define=API_BASE_URL=http://10.0.2.2:8080
```

`10.0.2.2` é o alias do Android Emulator para o loopback do host. Para Flutter desktop, use `http://127.0.0.1:8080`. O backend precisa estar rodando localmente conforme `../backend/README.md`; requer `DEMO_MODE=true` e token de operador local para rotas operacionais. O app público consulta ofertas, cria pedidos e mostra “Meus pedidos” com IDs locais consultados novamente ao backend. Histórico é por aparelho, limitado a 20 pedidos, e não substitui identidade/autenticação. No Android, HTTP sem TLS é liberado apenas no manifest de debug; builds release mantêm cleartext desabilitado.

## Verificação

```sh
fvm dart format --set-exit-if-changed lib test
fvm flutter analyze
fvm flutter test
fvm flutter build apk --debug
```

Os testes de widget usam HTTP simulado e preferências locais falsas; verificam catálogo, criação, persistência local do ID e leitura do estado autoritativo no histórico. Nenhuma chamada de pagamento real é feita.

## SDK Pix

O package Flutter local está incluído como dependência e sua API pública pode ser compilada pelo app. A UI de checkout ainda não é habilitada: o backend desta demo não fornece o endpoint merchant autenticado `POST /v1/merchant/checkout-sessions`, um `MerchantCheckoutAuthorizer` que prove ownership/preço do pedido, nem um `ChargeCreator` PSP. O sample endpoint genérico `POST /v1/checkout-sessions` do SDK não é usado porque não autoriza o pedido da demo nem representa cobrança vinculada. Não usar `DEMO_MODE` ou uma sessão inventada para contornar essa fronteira.
