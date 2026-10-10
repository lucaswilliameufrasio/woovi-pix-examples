# Woovi Pix Examples — roadmap pendente

Este roadmap reúne o trabalho que ainda falta nas três demos. É uma sequência proposta, sem datas ou estimativas. O checklist de evidências concluídas fica em [status.md](status.md); contratos e limites atuais estão em [offers-contract.md](offers-contract.md), [sandbox.md](sandbox.md) e [simulators.md](simulators.md).

## Regras de execução

- Manter Ofertas, Click & collect e Reservas como aplicações independentes, inclusive backend e banco.
- Backend continua dono de preço, pagamento, reserva/agenda e autorização de retirada. IDs e histórico local nunca autenticam.
- Usar simuladores e fixtures locais até haver credenciais próprias e autorização explícita para chamadas/escritas no sandbox. Não inventar payload Pix nem testar pagamento real.
- Sandbox, deploy/produção, publicação e outras operações externas exigem autorização separada; CI continua apenas validação.
- Preservar idempotência e resultado incerto após timeout; não repetir criação às cegas. Pagamento tardio e fulfillment permanecem estados separados.

## 1. Ofertas — concluir integração merchant

**Estado:** fluxo local com PostgreSQL, simulador, worker independente, BFF web e app mobile existe. O checkout Pix do SDK ainda não está ligado ao pedido. A sessão local atual não contém Pix e deliberadamente não pode ser apresentada como sessão Pix do SDK.

- [ ] Definir contrato merchant/authorization: a reserva autoriza pedido, preço, destinatário e valor no servidor; nenhum preço enviado pelo cliente é confiável.
- [ ] Implementar adapter isolado para criar/consultar cobrança, com ambiente/host restrito, tentativa persistida antes do POST, correlação pedido-cobrança e idempotency key estável.
- [ ] Cobrir timeout pós-criação, retry/reconciliação pela mesma referência, rate limit, indisponibilidade, duplicatas, webhook inválido e pagamento tardio com fixtures do protocolo real; nunca confirmar entrega a partir do callback da UI.
- [ ] Implementar autenticação/autorização criptográfica do webhook conforme contrato vigente do PSP; persistir/deduplicar antes do ACK e reconciliar com worker.
- [ ] Integrar a UI pública do SDK somente quando houver sessão merchant legítima e compatível, sem QR/Pix fictício. Manter o simulador local sem payload pagável.
- [ ] Atualizar contrato, OpenAPI, threat review e roteiro sandbox sem incluir credenciais.

**Aceite:** testes do adapter com servidor de protocolo local e PostgreSQL real validam correlação, idempotência, rollback/retry e persistência; nenhum caminho de cliente pode escolher preço/recebedor ou autorizar retirada; a UI SDK só recebe payload originado pelo backend autorizado.

**Bloqueio externo:** validação contra sandbox exige conta e credenciais próprias/autorizadas, além de aprovação explícita antes de qualquer escrita. Isso não autoriza produção.

## 2. Contratos HTTP, OpenAPI e MCP

- [ ] Publicar OpenAPI completo e versionado para as rotas de cada demo, erros `{message,error_code,extra?}`, autenticação, idempotência, limites e estados terminais.
- [ ] Definir tools MCP do domínio começando por consulta de cobrança correlacionada; toda operação deve chamar o mesmo serviço de domínio e obedecer autorização, estoque/agenda e idempotência.
- [ ] Impedir criação direta de cobrança pelo MCP quando não houver pedido/reserva autorizados; não expor bearer, capability ou segredo operacional em outputs/logs.
- [ ] Testar o servidor MCP real contra fixtures locais para autorização, escopo, replay, falha e correlação. Separar testes de ferramentas/read-only dos que fazem mutação.

**Aceite:** contrato gerado/validado no CI, compatibilidade de rotas/DTOs testada e MCP não contorna invariantes do backend.

## 3. Matriz de testes, isolamento e CI

- [ ] Ampliar E2E de Ofertas para falha/timeout, reserva concorrente, expiração, late paid, webhook duplicado, retry/restart de backend e worker, perda/expiração de capability, reinício do app e retirada repetida.
- [ ] Investigar estabilidade sob execução paralela; isolar banco/schema por pacote antes de habilitar concorrência irrestrita nas suítes Go.
- [ ] Manter browser Chromium e Android nativo como fluxos distintos. A corrida de restore identificada em 06/10/2026 foi corrigida e passou 10 execuções locais; isso não prova iOS, aparelho físico ou CI hospedado.
- [ ] Adicionar Android emulator E2E ao CI quando runner/custo/tempo permitirem; validar iOS/Keychain em macOS. Não marcar plataforma não executada como aprovada.
- [ ] Criar workflow de validação por demo/path: lint/format, unit, integração com PostgreSQL real, builds suportados e E2E isolado; sem etapa de deploy.
- [ ] Revalidar no GitHub a atualização local das actions/cache, tratar warnings e registrar URL/SHA do run.

**Aceite:** suítes repetíveis com dados isolados e cleanup seguro; falhas não são escondidas por retry automático; CI dá evidência para cada aplicação tocada sem misturar bancos ou segredos.

## 4. Click & collect — demo vertical independente

### Base de domínio para o MVP

- **Cliente:** vê o catálogo, reserva exatamente uma unidade, acompanha pelo acesso privado retornado na reserva e apresenta código de retirada depois que o pedido estiver pronto. ID/histórico local não autorizam consulta.
- **Operador da loja:** usa autenticação local de demo para consultar a fila, iniciar preparo, marcar pronto e conferir/consumir a retirada uma única vez. Isto não representa login humano de produção.
- **Simulador/worker:** confirma somente pagamentos sintéticos via evento persistido; não cria Pix ou chama PSP.
- **Escopo inicial:** uma loja e um produto físico seedado; uma unidade por pedido; estoque e banco exclusivos desta demo. Catálogo permanente e preparo diferenciam este fluxo das ofertas-relâmpago.
- **Estados separados:** pagamento (`pending`, `paid`, `expired`, `cancelled`, `payment_exception`) não se mistura com fulfillment (`awaiting_payment`, `preparing`, `ready_for_pickup`, `picked_up`). Só pagamento confirmado permite iniciar preparo; só `ready_for_pickup` permite retirada. Pagamento tardio não reabre estoque nem autoriza entrega automaticamente.
- **Cancelamento MVP:** cliente cancela somente enquanto o pagamento estiver pendente; a transação libera estoque uma vez. Repetir cancelamento é idempotente. Pagamento após cancelamento/expiração vira `payment_exception`; não há cancelamento nem estorno automático de pedido já pago.
- **Módulos:** domínio/transições; PostgreSQL e reserva/expiração transacionais; API `/v1`; adapter de simulador local + processamento durável; web/BFF server-only; cliente Flutter com capability em armazenamento seguro; operações de loja protegidas.
- **Ordem vertical:** (1) migration/seed, reserva concorrente, capability e estados; (2) API/simulador, eventos e worker/retirada; (3) web/BFF cliente + fila de loja; (4) Flutter cliente e consulta privada; (5) E2E isolado, documentação e CI.
- **Critério de conclusão local:** em uma fixture real de PostgreSQL, cliente reserva → simulador confirma → operador prepara/marca pronto → cliente acompanha → retirada ocorre uma vez; pedidos, estoque e processos não usam Ofertas.

- [x] Confirmar linguagem de domínio, atores e estados: catálogo/produto, pedido, preparo, pronto e retirado; definir cancelamento, indisponibilidade e pagamento tardio.
- [x] Criar backend e schema PostgreSQL próprios, preço autoritativo, reserva de estoque concorrente e autorização server-side para transições.
- [x] Criar web/BFF e app Flutter próprios; manter secrets fora do browser e credenciais de pedido no armazenamento apropriado do mobile.
- [x] Implementar worker/outbox somente conforme necessidade comprovada; qualquer confirmação de pagamento deve ser persistida e idempotente.
- [ ] Criar simulador sem Pix pagável e E2E que cobre concorrência, transições fora de ordem, restart e retirada única.
- [x] Publicar contrato OpenAPI desta demo; a spec não oferece operação que ignore transições do domínio.
- [ ] Adicionar MCP desta demo sem permitir que ferramenta pule pedido, pagamento ou preparo.

**Progresso de implementação local (09/10/2026):** domínio, backend/API/PostgreSQL, worker durável, simulador local, BFF/web, app Flutter e guia local foram implementados. Testes backend com PostgreSQL real cobrem concorrência, cancelamento/expiração, late payment, restart do store/evento e retirada única; web tem validações unitárias e Flutter testes de API/widget. Ainda não concluir esta demo: falta smoke/E2E repetível integrando servidor + BFF/browser + app/dispositivo, pipeline CI dessa demo, contrato OpenAPI/MCP e revisão/validação dos limites operacionais. iOS/Keychain e PSP continuam bloqueados por plataforma/autorização; não integram o aceite local.

**Aceite:** uma fixture local percorre pedido → preparo → pronto → retirada exatamente uma vez em cada cliente; seu banco, processos e testes não dependem do schema de Ofertas.

## 5. Reservas — demo vertical independente

- **Base MVP didática registrada (09/10/2026; não é política comercial):** um recurso seedado, “Sala de atendimento”; sessão de 30 min, buffer de 15 min, grade de início a cada 15 min; seg–sex 09:00–17:00 em `America/Sao_Paulo`; disponibilidade até 14 dias; hold de 5 min; preço de seed R$ 120,00 controlado pelo backend. Sem feriados, múltiplos recursos ou refund automático.
- **Atores/jornadas:** cliente lista horários autoritativos → adquire hold atomicamente → usa capability privada para consultar, confirmar simulação local ou cancelar enquanto held; worker confirma evento, expira hold e marca pagamentos tardios como exceção; operador local pode consultar reservas pagas e marcar conclusão, sem representar identidade de produção.
- **Estados independentes:** reserva `held → confirmed → completed`, ou `held → expired/cancelled`; pagamento `pending → paid/expired/cancelled/payment_exception`. Pagamento depois da liberação não readquire horário nem troca a reserva de outro cliente.
- **Fuso/DST:** instantes wire em RFC3339/UTC; resposta inclui offset/local label e IANA zone. Enumerar instantes pela linha do tempo UTC para cada dia local, omitir horários inexistentes e distinguir ocorrências repetidas no fold. Cliente só pode submeter start retornado e backend sempre recalcula elegibilidade.
- **Concorrência:** Postgres é árbitro final com exclusion constraint para intervalo ocupado `[starts_at, ends_at + buffer)` em holds/confirmadas; slots adjacentes são válidos quando intervalo ocupado anterior termina no instante em que o seguinte começa. Hold e pagamento são gravados transacionalmente; expiração libera exatamente uma vez.
- **Módulos/rotas:** agenda/serviço e projeção de disponibilidade; store/transições; API `/v1`; simulador local + worker/outbox; BFF web com cookie HttpOnly; Flutter com credenciais em secure storage; operador local separado.
- **Sequência vertical:** (1) migrations/seed/timezone/gerador de slots/exclusion + reservas concorrentes; (2) capabilities, cancelamento, simulador, evento/worker/expiração/reconciliação; (3) HTTP/OpenAPI e web/BFF jornada cliente; (4) Flutter jornada cliente; (5) operador/retirada de manutenção, E2E Chromium + Android físico e CI/docs.
- [x] Validar base do domínio e implementar geração de disponibilidade, DST, holds, buffers e políticas acima.
- [x] Criar backend/schema próprios que previnam overlap/duplo booking no banco, inclusive chamadas concorrentes e slots adjacentes.
- [x] Construir disponibilidade e reserva em web/BFF e Flutter; backend calcula preço e janela disponível, clientes não decidem horário elegível.
- [x] Separar estados de reserva e pagamento; definir o efeito de pagamento tardio após liberação do slot sem reassociar silenciosamente a outra reserva.
- [x] Implementar expiração/reconciliação e simulador local; integrar PSP somente depois de contrato merchant autorizado.
- [x] Criar testes de mesma janela concorrente, adjacência, timezone/DST, timeout, expiração, late paid e restart; contrato OpenAPI restrito a horários elegíveis.
- [ ] Adicionar MCP restrito ao domínio; MCP ainda não existe neste repositório.

**Progresso local (10/10/2026):** backend Go/PostgreSQL, BFF/customer+operator web, Flutter, OpenAPI, Scalar, documentação e CI próprios foram implementados. PostgreSQL real `-race -count=3` cobre 20 hold concorrentes, intervalos adjacentes/conflitantes, capability/cancelamento, expiração, pagamento tardio, recuperação de evento após pool restart, migration idempotente e HTTP/auth/operator. Chromium E2E passou com Postgres/schema efêmero e processos API+BFF; também verifica Scalar renderizando o contrato OpenAPI canônico. Android físico Samsung SM-A146M passou em jornada local de hold/cancelamento com secure storage e adb reverse. Web format/lint/check/build/audit, Flutter analyze/test/APK e tooling Ruff passaram. Pendem revisão de limites/ameaças, MCP, CI hosted, validação iOS/Keychain em macOS e roteiro integrado das três demos. Não é autorização para PSP ou deploy.

**Aceite:** banco garante ausência de reservas sobrepostas sob concorrência real; horários e transições permanecem corretos ao reiniciar API/worker e atravessar mudanças de fuso.

## 6. Validação integrada e release candidate

- [ ] Automatizar um roteiro raiz que inicia e valida as três demos sem compartilhar DB/schema, portas secretas ou estado de teste.
- [ ] Fazer uma pessoa desenvolvedora nova executar setup, testes e E2E local usando apenas a documentação; corrigir passos ausentes e dependências implícitas.
- [ ] Revisar ameaças, autenticação humana, autorização, gestão/rotação de segredos, webhook, logs, backup, retenção, rate limits e configuração loopback/deploy de cada demo.
- [ ] Validar sandbox de cada integração apenas com conta própria/autorizada e autorização explícita; registrar limitações e evidências sem credenciais.
- [ ] Produzir checklist RC independente por demo e pelo SDK/MCP que tiverem sido realmente integrados.
- [ ] Considerar deploy/produção somente após revisão operacional e autorização separada; deployment não é parte do CI de validação.

**Saída do roadmap:** três demos reproduzíveis, isoladas, documentadas e validadas nas plataformas declaradas. O plano global só pode ser chamado concluído quando todos os critérios aplicáveis estiverem demonstrados; bloqueios externos devem continuar identificados como bloqueios, não como entregas.
