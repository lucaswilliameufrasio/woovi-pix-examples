# Woovi Pix Examples

Repositório público com demos independentes de Pix. Ferramentas são fixadas por projeto: Go/Node via mise, Flutter via FVM e dependências via gerenciador nativo de cada stack. Go 1.27.1 e Node 24.21.0 estão fixados em `mise.toml`; Flutter 3.47.6 está fixado via FVM nos projetos mobile já iniciados.

`ofertas-relampago/backend`, `ofertas-relampago/web` e `ofertas-relampago/mobile` compõem a primeira demo local. As interfaces listam ofertas e criam pedidos no backend da demo; não geram cobrança Pix nem movimentam dinheiro.

## Comece aqui

- [Executar e testar sem sandbox](docs/local.md): API, PostgreSQL, web/mobile, confirmação simulada e retirada.
- [Sandbox: requisitos e limites atuais](docs/sandbox.md): o que já pode ser testado no SDK e o que falta integrar nestas demos.
- [Simuladores e repositórios relacionados](docs/simulators.md): diferenças entre o simulador persistido de ofertas, o simulador HTTP Woovi e o checkout exemplo do SDK.
- [Contrato e estados de ofertas](docs/offers-contract.md): rotas, autoridade do backend, erros e estados.
- [Checklist de entrega](docs/status.md): implementado, validações realizadas e pendências.

| Demo              | Estado atual                                                                                                                                                 |
| ----------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Ofertas-relâmpago | Backend local, catálogo/reserva/histórico web e Flutter; pagamento simulado via HTTP e retirada autenticada. Checkout Pix nas interfaces ainda desabilitado. |
| Click & collect   | Não implementada; terá backend, web, mobile e banco próprios.                                                                                                |
| Reservas          | Não implementada; terá backend, web, mobile e banco próprios.                                                                                                |

**O projeto não está concluído nem pronto para produção.** Não há validação sandbox real, E2E de browser/dispositivo ou build iOS. Não use comprovante/callback da interface como autorização de entrega. Não pague códigos demonstrativos em banco real.
