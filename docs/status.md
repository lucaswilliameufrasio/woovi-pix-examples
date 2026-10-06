# Checklist de entrega

Este arquivo distingue implementação, testes automatizados e validação externa. Commit/push não significam plano concluído.

## Ofertas

- [x] Banco próprio, seed, reserva atômica e preço autoritativo.
- [x] Simulador próprio persistido, cobrança idempotente e timeout pós-criação reconciliável.
- [x] Evento HTTP persistido/deduplicado antes do ACK; worker lease/retry e expiração.
- [x] Pagamento tardio como exceção, sem retirada indevida.
- [x] Operador Bearer local; rotação/resgate único da credencial de retirada.
- [x] Sessão local sem QR, emissão pelo operador, idempotência transacional e credencial curta de consulta por sessão.
- [x] Capability de pedido emitida atomicamente na reserva, escopo/TTL próprios e BFF web com cookie HttpOnly e sessão local autorizada pelo cliente.
- [x] Cliente mobile migrado para consulta privada, capability no armazenamento seguro e IDs sem autorização; testes de cliente com backend/PostgreSQL reais.
- [x] Consulta anônima por ID encerrada: alias legado requer capability; operador possui consulta separada para diagnóstico de pedidos antigos.
- [x] Primeiro E2E Chromium de UI/BFF/Go/PostgreSQL/simulador em schemas isolados, com sessão/replay e acesso por cookie.
- [x] Primeiro E2E Android nativo no emulador genérico: armazenamento seguro real, persistência entre processos, UI/worker/histórico e retirada/SQL em duas fixtures.
- [x] Web catálogo/reserva/acompanhamento e Flutter catálogo/pedido/histórico/polling.
- [x] Skills locais e enforcement TypeScript por lint/testes.
- [ ] Checkout merchant seguro e UI SDK efetivamente conectada ao pedido.
- [ ] Adapter PSP/simulador HTTP Woovi conectado à demo, não apenas smoke de fronteira.
- [ ] MCP com cobrança correlacionada e autorização do domínio.
- [x] Primeiros E2E Playwright e Flutter Android contra backend/DB em schemas isolados (não cobrem a matriz completa).
- [ ] Matriz ampliada de E2E: falhas, timeout, expiração, concorrência e restart de backend/app em plataformas suportadas.
- [ ] Isolamento de DB/schema por pacote para executar suítes Go em paralelo.
- [ ] Worker independente, OpenAPI completo e CI reproduzível por demo.

## Outras demos

- [ ] Click & collect: backend/banco/web/mobile independentes, preparo e retirada.
- [ ] Reservas: backend/banco/web/mobile independentes, agenda/slots/timezone.

## Documentação e validação

- [x] Roteiro local com requisitos, portas, comandos, estados esperados e troubleshooting.
- [x] Comparação dos simuladores e limites do reaproveitamento.
- [x] Guia sandbox que distingue exemplo SDK separado de integração ainda ausente nas demos.
- [ ] Execução e testes ponta a ponta de todas as três demos por um novo desenvolvedor.
- [ ] Sandbox real autorizado e validado; cadastro sem documentos/credenciais de terceiros.
- [ ] iOS validado em macOS.
- [ ] Deploy/produção, identidade completa, webhook criptográfico e revisão de ameaças.

As suites existentes de Go/PostgreSQL, web e Flutter são executadas antes de registrar cada entrega. Smoke HTTP complementa, não substitui testes de UI. Confira o resultado mais recente no relatório da execução; não interprete checkboxes como evidência de sandbox ou dispositivo.

### Validação deste ajuste documental — 04/10/2026

- Smoke de ofertas executado duas vezes por rodada em schemas exclusivos, incluindo leitura SQL de pedido retirado, evento processado e cobrança paga.
- Simulador Woovi do checkout MCP: criação/consulta HTTP reais passaram. Como as portas fixas estavam ocupadas, foi compilado com overlay temporário apenas para bind efêmero, sem editar o checkout irmão.
- Go/PostgreSQL real: `go test -race -p=1 -count=3 ./...`, build, vet, gofmt e golangci-lint passaram.
- Tooling Python: cinco testes de segurança/startup, Ruff 0.14.10 lint/format e compilação dos módulos.
- Documentação: formatação e links locais verificados.
- Nenhum código web/Flutter alterado; suas suites não foram reexecutadas neste ajuste documental. E2E UI, sandbox e iOS continuam pendentes.
- A instância Docker antiga apresentou falha de runtime no startup; não foi removida/recriada. A validação usou um novo projeto PostgreSQL de teste e manteve seus dados.

### Slice seguinte — sessão local sem QR

Implementado contrato autenticado de emissão/consulta, sem Pix inventado e sem modificar SDK. Testes PostgreSQL/HTTP em schemas exclusivos cobrem concorrência (mesma chave, chaves diferentes e mesma chave em pedidos distintos), hashes/TTL, isolamento de credenciais, replay após reabertura/migração, preço autoritativo, worker, prazo e pagamento tardio. Smoke com processo real agora inclui emissão/replay/consulta da sessão e verifica sessão/chave únicas no SQL. Isso não conclui checkout Pix, ownership de cliente ou integração de UI.

Gates do slice: seis testes novos de contrato (quatro com PostgreSQL isolado), suíte Go completa com `-race -p=1 -count=3`, build/vet/gofmt/golangci-lint, smoke de processo completo em dois schemas, cinco testes do tooling Python, Ruff lint/format e Prettier passaram. Reset/revogação foi testado apenas em schema descartável exclusivo do teste, sem tocar pedidos externos. Web/Flutter não tiveram alteração de código nem validação de UI neste slice.

### Slice seguinte — cliente local e BFF privado

Capability de vinte minutos emitida uma vez com a reserva, hash no banco, consulta privada e emissão de checkout com revalidação após locks. BFF mantém token em cookie HttpOnly/SameSite/path; resposta de reserva e action/load não o serializam. Fluxo HTTP/SSR com processos SvelteKit/Go e PostgreSQL reais passou em dois schemas, incluindo cookie privado, ausência de token no HTML, acesso só por ID negado, sessão idempotente, worker/status e retirada única. Não é Playwright/JavaScript E2E. API pública legada/mobile ainda loopback-only; SDK Pix, identidade completa e demais demos continuam pendentes.

Gates: quatro testes novos de autorização/rollback (três PostgreSQL isolado), suíte Go completa `-race -p=1 -count=3`, build/vet/gofmt/golangci-lint; web 59 testes, lint/check/format/build, autofixer Svelte sem problemas; tooling cinco testes e Ruff; smoke HTTP direto e BFF em dois schemas cada. Gitleaks nos diretórios de código/testes tocados sem leaks. Build web mantém aviso `adapter_missing`; nenhum deploy, browser/dispositivo ou sandbox real validado.

### Mobile por capability — 05/10/2026

Mobile usa GET privado com capability escopada por origem/pedido e TTL do backend; `flutter_secure_storage` 11.2.0, nunca token em shared_preferences. Sem acesso ou com prazo vencido: nenhuma consulta pública; 401 encerra polling/remove estado anterior como confirmação. Falha de escrita mantém reserva e acesso em memória com aviso de perda após restart. Parsing/URLs/redirects/deadlines defensivos e POST sem retry. Android sem backup/transferência de shared preferences; iOS acessibilidade Keychain restrita ao aparelho.

Runner `--mobile`: cliente Flutter real → backend Go → PostgreSQL em dois schemas, reserva única/estoque/preço/consulta privada/restart de cliente comprovados; armazenamento nativo é substituído. Build APK debug passou com aviso de SDK XML incompatível entre ferramentas locais. **E2E UI em dispositivo e Keystore/Keychain nativos ainda pendentes**, assim como Pix/SDK, outras demos e sandbox. Consulta pública legada da API permanece por compatibilidade de tooling, não usada por web/mobile novos.

Formatação Dart/Go/Prettier/Ruff aplicada aos códigos alterados e checada, conforme pedido. Suíte mobile cobre também 401 encerrando polling/retirando confirmação antiga; teste com backend real é executado separadamente pelo runner, não contado como aprovado quando omitido no comando padrão.

### Fechamento do alias legado — 05/10/2026

Snapshots anteriores acima descrevem o estado de seus slices. Estado atual: GET `/v1/orders/{order_id}` exige a mesma capability da rota customer, com TTL/401/no-store, e não é público. GET `/v1/operator/orders/{order_id}` permite diagnóstico pelo operador sem reemitir token cliente. Smoke/roteiros migrados. Pedido sem capability não ganha acesso por ID; autenticação humana completa e produção ainda pendentes.

Regressão demonstrou a falha original (200 anônimo) antes da correção; depois, suíte Go/PostgreSQL completa passou `-race -p=1 -count=3`, mais build/vet/gofmt/golangci-lint. Smokes API/BFF/mobile passaram duas vezes por modo em schemas independentes. Tooling: cinco testes, Ruff/format; Gitleaks backend/docs sem leaks. Nenhum sandbox/pagamento real/deploy/publicação.

### Primeiro E2E Chromium — após publicação cf03359

`@playwright/test` 1.63.0 fixado, modo `--browser` com duas fixtures isoladas, sem retries/reset. Teste de interface cobre reserva, HttpOnly/SameSite/TTL, ausência de credenciais no DOM/JS, contexto anônimo negado, restart de contexto com estado somente em memória, sessão idempotente e confirmação local/worker/retirada. PostgreSQL confirma pedido/evento/cobrança/sessão/chave únicos e dois grants. Sem segredo operacional no processo web/Chromium/página; traces/screenshots/vídeos/storageState em arquivo desabilitados. Primeiro fluxo E2E de navegador, **não toda a matriz** de concorrência/timeout/expiração/restart de backend. Keystore/Keychain e E2E mobile nativo pendentes; nenhum dispositivo conectado na inspeção local, emulador Android genérico disponível. Playwright usa fallback Ubuntu no host atual. Estas mudanças são posteriores ao commit publicado e não foram commitadas/pushadas novamente.

Gates finais: 59 testes web, lint/check/format/build/audit; cinco testes Python/Ruff; smokes BFF/mobile 2x e Chromium 2x após esperar navegação explícita no replay. Houve uma falha intermitente de browser em execução paralela e um timeout inicial de ESLint sob carga alta do host; cinco execuções sequenciais de browser e a execução final passaram, assim como repetição unitária sem aumentar timeout. Causa do incidente paralelo não comprovada; estabilidade sob carga continua a investigar, não ocultada por retry automático.

### Android nativo — após publicação e421466

Runner `--android-device` exige serial explícito do perfil genérico `medium_phone`, portas loopback efêmeras via adb reverse e schema PostgreSQL próprio. Integração SDK Flutter com fases `create`/`restore` executadas em processos distintos, armazenamento seguro Android real, sem mock/operador no app. Reserva por UI, confirmação no simulador/worker, estado/histórico depois de restart; retirada única feita pelo harness, SQL de pedido/evento/cobrança/grant únicos conferido, zero sessões SDK. Passou em duas fixtures independentes. Não valida iOS/Keychain, dispositivo físico, checkout Pix nem toda matriz de falhas.

Startup gráfico do emulador falhou por Qt/xcb sem display; mesmo perfil iniciou com `-no-window`, sem modificar/wipe do AVD. Restore inicial falhou porque `flutter test` desinstala app por padrão; `--no-uninstall` preserva instalação/dados e a regressão nativa passou. Testes Python verificam esse argumento, ausência de tokens/DB em dart-defines e recusa de serial/perfil inválido; tooling agora tem sete testes. Dados do app/AVD e schemas preservados. Novas alterações nativas ainda não publicadas; commit mais recente enviado é e421466.
