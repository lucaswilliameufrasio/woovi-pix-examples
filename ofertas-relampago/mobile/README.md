# Ofertas-relâmpago — app Flutter

O [roteiro integrado](../../docs/local.md) explica reserva, confirmação HTTP simulada e retirada. [Sandbox ainda não está integrado](../../docs/sandbox.md).

App de demonstração local da oferta limitada. Usa Flutter 3.47.6 via FVM e consome a API `/v1`; não gera Pix nem realiza pagamentos. O package `woovi_pix_flutter` é resolvido do checkout local irmão `../woovi-pix-flutter-sdk/packages/woovi_pix_flutter` (repositório `woovi-pix-flutter-sdk` ao lado deste repo).

## Executar

Na raiz do repositório, Go é gerenciado pelo mise; dentro do app, Flutter é gerenciado pelo FVM:

```sh
cd ofertas-relampago/mobile
fvm flutter pub get
fvm flutter run --dart-define=API_BASE_URL=http://10.0.2.2:8080
```

`10.0.2.2` é o alias do Android Emulator para o loopback do host. Para Flutter desktop, use `http://127.0.0.1:8080`. Apenas HTTP local com porta explícita (`127.0.0.1`, `::1`, `10.0.2.2`) é aceito; URLs remotas, userinfo, caminhos extras e redirects são recusados. O backend precisa estar rodando conforme `../backend/README.md`, com `DEMO_MODE=true`. Segredo de operador **não entra no app**. No Android, HTTP sem TLS é liberado apenas no manifest de debug; builds release mantêm cleartext desabilitado.

### Acesso aos pedidos

Criar reserva retorna uma capability de vinte minutos, armazenada com `flutter_secure_storage` 11.2.0 e chave escopada por **origem API + pedido**. Consulta usa somente `GET /v1/customer/orders/{order_id}` com Bearer; não há fallback para GET público. `shared_preferences` guarda somente os até vinte IDs do histórico, nunca tokens ou estado autoritativo. IDs antigos sem credencial não autorizam consulta, nem recebem token por recuperação. O backend continua definindo preço/estado; posse local da capability não é identidade humana de produção.

Se escrita segura falhar, a reserva continua criada e o app avisa: acesso funciona em memória somente nesta execução. Se o prazo vencer/servidor responder 401, polling para, o estado antigo não é mostrado como confirmação atual e o usuário é direcionado ao operador. Sem renovar token por ID ou repetir POST automaticamente. Erro/timeout na criação pode significar reserva criada sem acesso recuperável: **não repita às cegas**. O histórico mantém IDs inacessíveis para diagnóstico, sem mostrar estado em cache como atual.

Android desabilita auto-backup e exclui shared preferences da transferência de dados; iOS usa Keychain `unlocked_this_device`. O primeiro E2E Android abaixo valida o plugin nativo e persistência em emulador; Keychain/iOS, aparelho físico e resistência a backup/comprometimento ainda não foram validados. Mocks/builds sozinhos não validam essas garantias. Trocar origem/porta não reaproveita credenciais.

## Verificação

```sh
fvm dart format --set-exit-if-changed lib test
fvm flutter analyze
fvm flutter test
fvm flutter build apk --debug
```

Testes de widget/unidade cobrem escopo/TTL, restart do cliente, falha de escrita/leitura, IDs sem acesso, revogação 401, payload inválido, deadline de criação sem retry e rejeição de redirects com servidor HTTP real. O teste de backend é omitido no `flutter test` padrão por exigir infraestrutura isolada. Execute da raiz, com PostgreSQL healthy:

```sh
python3 tooling/run_offers_smoke.py --mobile
```

O runner sobe Go em loopback/portas efêmeras em dois schemas exclusivos e executa o **cliente Flutter real contra HTTP/PostgreSQL**, validando reserva única, preço/estoque autoritativos, consulta privada e credencial após recriar cliente; confirma contagens SQL e mantém schemas para inspeção. Somente armazenamento da plataforma é substituído. Isso **não é E2E em dispositivo**, nem valida Keychain/Keystore, lifecycle nativo ou UI com backend real. Não recebe segredo do operador/DB no processo Flutter. Nenhuma chamada de pagamento real é feita.

### E2E Android nativo

Com o emulador **genérico `medium_phone`** iniciado, descubra seu serial por `adb devices` e execute da raiz:

```sh
python3 tooling/run_offers_smoke.py --android-device emulator-5586
```

Substitua o serial pelo desse perfil genérico. O runner não inicia/wipe/reconfigura emuladores nem aceita perfis privados ou aparelhos físicos. Requer FVM, SDK Android/adb e PostgreSQL healthy. Em host sem display, inicie o perfil genérico com `emulator -avd medium_phone -no-window -no-snapshot-save`; configure o PATH do SDK se necessário, sem instalar dependências gráficas do sistema às cegas.

São dois schemas exclusivos e **duas execuções de processo do app por schema**, nas fases `create` e `restore`. A primeira reserva pela UI, grava a capability usando o plugin Android real, verifica que preferências públicas guardam IDs e não o token, confirma só pelo simulador local e consulta o estado do worker. O app é encerrado; a segunda execução autentica usando a capability persistida e reabre o histórico com estado autoritativo. Sem storage mock ou credencial operacional compilada. O harness Python valida retirada única e SQL: um pedido/evento/cobrança/grant de pedido, zero sessões SDK (Pix permanece desabilitado).

`--no-uninstall` é indispensável: o `flutter test` padrão desinstala o app ao concluir, destruindo justamente os dados cuja persistência queremos testar. O runner **não apaga dados do app**; mantém fixture IDs/credenciais locais com seus prazos, em origem própria. `adb reverse` usa só portas efêmeras dos processos próprios, recusa sobrescrever mapeamento existente e remove apenas os que criou. Captura e omite output com possíveis credenciais; não captura tela/logcat/arquivos de tokens. Build/testes demoram mais que os testes de unidade.

Validado no emulador Android genérico com armazenamento nativo real e processo novo; **não** comprova aparelho físico, iOS/Keychain, todos os cenários de lifecycle/rede/expiração, resistência a backup/comprometimento ou checkout Pix/SDK.

## SDK Pix

O package Flutter local está incluído como dependência e sua API pública pode ser compilada pelo app. Backend já autoriza sessão local vinculada ao pedido por capability, mas essa sessão **não tem payload Pix**: o parser SDK exige `pix_copy_paste` para `pending`. UI SDK/QR permanece desabilitada até adapter PSP/payload legítimo; não invente string ou contorne o parser. O endpoint genérico do sample SDK não autoriza os pedidos desta demo. Checkout irmão não foi modificado.
