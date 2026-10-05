# Simuladores: o que pode ser reaproveitado

## Comparação

| Implementação                                              | Contrato                                                                     | Persistência                       | Confirma pagamento?                      | Liga ao pedido de ofertas?       |
| ---------------------------------------------------------- | ---------------------------------------------------------------------------- | ---------------------------------- | ---------------------------------------- | -------------------------------- |
| `ofertas-relampago/backend/internal/simulator`             | `/v1/charges` com `order_id`                                                 | PostgreSQL da própria demo         | Sim, evento HTTP → persistência → worker | Sim; valor consultado no backend |
| `woovi-pix-mcp/cmd/woovi-simulator` no checkout irmão      | `/api/v1/charge`, formato externo Woovi (`correlationID`, `value`, `brCode`) | Memória                            | Não no handler inspecionado              | Não                              |
| `woovi-pix-flutter-sdk/examples/backend` no checkout irmão | Sessões checkout e PSP de demonstração                                       | PostgreSQL/configuração do exemplo | Sim, rota local demo                     | Não; pedido fixo do exemplo SDK  |

Todos usam dados sintéticos. O simulador de ofertas não gera QR/código Pix. Os códigos dos exemplos irmãos são demonstrativos, não devem ser pagos nem apresentados como cobrança real. Os contratos externos camelCase pertencem à fronteira PSP; APIs destas demos continuam em snake_case.

## Testar o simulador HTTP Woovi existente

Não precisa de conta ou credencial real. No terminal 1, a partir do checkout irmão `woovi-pix-mcp`:

```sh
mise exec -- go run ./cmd/woovi-simulator
```

Escuta em `127.0.0.1:8081`. Se ofertas já está rodando na mesma porta, pare apenas o simulador/processo que você iniciou ou escolha outra porta para ofertas antes do startup; não mate processos desconhecidos.

No terminal 2, na raiz de **woovi-pix-examples**:

```sh
WOOVI_SIMULATOR_BASE_URL=http://127.0.0.1:8081 \
python3 tooling/smoke.py woovi-simulator --ack-local-write
```

O smoke cria referência única de R$ 12,50, consulta pelo identifier e verifica contrato/estado `ACTIVE`. O valor `simulator` no header Authorization é uma fixture explícita do servidor local, não um AppID Woovi. Não existe chamada externa.

**Limite:** sucesso comprova apenas a fronteira HTTP create/get. Não comprova integração MCP, SDK ou reserva/retirada das demos. Restart desse simulador perde cobranças em memória. Não substituir o simulador persistido de ofertas sem adapter de contrato, correlação de pedido, autorização e processamento de confirmação.

A implementação inspecionada aceita POST e GET; o README do checkout MCP ainda descreve consulta apenas. Confira a versão antes de executar: se POST retorna 404, esse checkout pode ser anterior ao suporte de criação. Este trabalho não altera o repositório irmão.

## Testar o checkout simulado do SDK separadamente

Siga o [guia local público do SDK](https://github.com/lucaswilliameufrasio/woovi-pix-flutter-sdk/blob/main/docs/pt-br/demo.md) no repositório **woovi-pix-flutter-sdk**, não neste backend. Também existe `docs/pt-br/demo.md` no checkout irmão.

Não execute os dois backends na mesma porta 8080. O pedido fixo/rota de criação demo do SDK não substitui autorização dos pedidos de ofertas. Nenhuma sessão desse exemplo será usada para “habilitar” o checkout aqui sem contrato merchant correto.

## Próxima integração

Reaproveitar o contrato HTTP externo reduz necessidade de sandbox para testes de adapter. Ainda faltam vincular pedido/valor, persistir tentativa/idempotência, emitir sessão restrita ao pedido e obter confirmação pelo caminho autoritativo. Para MCP, começar por consulta de cobrança correlacionada; criação direta no PSP não pode contornar reserva de estoque/agenda.
