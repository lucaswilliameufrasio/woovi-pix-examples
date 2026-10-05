# Sandbox: requisitos e estado real

**Sandbox não está habilitado nestas demos.** O backend de ofertas recusa execução sem `DEMO_MODE=true` e não implementa adapter Woovi. Definir AppID/URL sandbox aqui não habilita pagamentos. Não oferecemos um comando de teste das demos que ainda não funciona.

## Sem acesso ao sandbox

Use o [roteiro local](local.md) para o fluxo de ofertas, e o [simulador Woovi existente](simulators.md) para o contrato create/get. Não exige cadastro, CPF/CNPJ nem credenciais externas. Isso não equivale a validar o PSP real.

## Com acesso autorizado: validar o exemplo SDK separado

O repositório **woovi-pix-flutter-sdk**, não estas demos, já possui implementação e [passo a passo sandbox](https://github.com/lucaswilliameufrasio/woovi-pix-flutter-sdk/blob/main/docs/pt-br/sandbox.md). No checkout local, leia `docs/pt-br/sandbox.md`.

1. Use conta sandbox própria/autorizada, separada da produção. Não invente documentos nem reutilize conta, CNPJ ou keys de outro projeto/empresa.
2. Confirme os requisitos atuais de cadastro em [app.woovi-sandbox.com](https://app.woovi-sandbox.com/). Se não possui os dados autorizados exigidos, consulte suporte Woovi; o simulador permite continuar localmente.
3. Gere AppID com permissões mínimas no painel sandbox. Informe por prompt oculto/configuração protegida **somente ao backend**; nunca chat, config MCP, Flutter, URL/query ou arquivo público.
4. Execute o guia do SDK com banco isolado e portas livres. Não misture `ENABLE_DEMO_PSP` com sandbox. Preserve a chave de idempotência em erro/restart.
5. Crie a cobrança apenas após autorização explícita para escrita nessa conta. Confirme pelo mecanismo de pagamento de teste do painel, nunca pelo seu app bancário real.
6. Verifique valor, correlação, estado e eventual reconciliação; callback da UI não autoriza entrega.
7. Não distribua o APK do exemplo com token de sessão de teste embutido. Não exponha servidor ou webhook/túnel sem revisar autenticação e autorização separadamente.

O guia SDK documenta configuração; **sandbox real e iOS ainda não foram validados neste trabalho**. Consulte as instruções da versão efetivamente obtida, não presuma que documentos remotos acompanham seu checkout.

## Bloqueios para testar estas demos com sandbox

- Authorizer que comprove acesso ao pedido e use preço/recebedor definidos no servidor.
- Adapter de charge/status, restrição de host/ambiente e tentativa persistida antes do POST externo.
- Idempotência e recuperação de timeout por referência sem nova cobrança às cegas.
- Sessão curta/restrita compatível com SDK; UI Pix só após contrato/testes reais dessa integração.
- Já existe sessão **local sem QR**, emitida pelo operador ou pelo portador de capability de pedido, e restrita à consulta. Web usa cookie HttpOnly; mobile usa armazenamento seguro. Isso não resolve identidade de produção, E2E nativo nem o requisito de payload Pix do SDK.
- Webhook autenticado conforme contrato Woovi, persistência antes do ACK e worker/conciliação de negócio.
- Prova de pagamento tardio sem entregar estoque/horário já liberado.
- Roteiro MCP sem contornar regras de reserva.

Implementação e testes locais desses pontos precedem validação externa. Não confundir o smoke do simulador com sandbox aprovado.

## Fontes oficiais

- [Ambiente de teste](https://developers.woovi.com/en/docs/test-environment)
- [Configuração sandbox](https://developers.woovi.com/en/docs/sdk/node/how-to-configure-for-sandbox)
- [Conta bancária de teste](https://developers.woovi.com/en/docs/test-environment/test-account/flow-company-bank-test)
- [Pagar uma cobrança de teste](https://developers.woovi.com/en/docs/test-environment/test-account/test-pay-pix)

Fontes não autorizam operação financeira, cadastro com dados fictícios ou uso de credencial de terceiros.
