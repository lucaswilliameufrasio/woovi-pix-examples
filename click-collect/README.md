# Balcão — Click & collect local

Demonstração independente de pedido e retirada em loja. API Go e banco PostgreSQL próprios, BFF/web SvelteKit e app Flutter. O backend é autoridade de preço, estoque, pagamento e retirada.

- Uma loja de demonstração, produto seedado e uma unidade por pedido.
- Pagamento simulado e persistido; **não** gera Pix, não chama Woovi/PSP e não movimenta dinheiro.
- Pagamento e preparo/retirada são estados separados. Reserva concorrente não vende além do estoque; pagamento após expiração/cancelamento fica em exceção e não autoriza retirada.
- API e apps permanecem em loopback/local; não exponha esses tokens ou o serviço à rede.

Comece pelo [guia de execução local](../docs/click-collect-local.md) e consulte o [contrato da API](backend/README.md). O estado restante do projeto está em [`docs/roadmap.md`](../docs/roadmap.md); esta demo ainda não inclui integração PSP, autenticação humana de loja ou OpenAPI/MCP.
