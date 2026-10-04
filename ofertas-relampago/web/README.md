# Ofertas-relâmpago — web

Interface SvelteKit 3 com Svelte 5 e TypeScript. Consulta o backend próprio da demo por chamadas server-side; o browser não recebe URL ou segredo do operador. A reserva cria apenas um pedido local e **não inicia pagamento Pix**.

## Executar localmente

Requer Node 24.21.0 via mise. Inicie o backend conforme `../backend/README.md` e rode:

```sh
cd ofertas-relampago/web
npm ci
API_BASE_URL=http://127.0.0.1:8080 npm run dev -- --host 127.0.0.1
```

`API_BASE_URL` é lida somente pelo servidor. A API deve estar em loopback e em modo demo. O catálogo usa `GET /v1/offers`; o formulário envia `POST /v1/orders` com `offer_id` ao backend.

Após a reserva, “Acompanhar pedido” abre `/orders/{order_id}`. Essa página consulta `GET /v1/orders/{order_id}` e permite atualizar manualmente, sem criar cobrança ou alterar o pedido. Mostra pagamento e exceções conforme o backend; não autoriza entrega. O prazo original é exibido com timezone UTC explícito. Pedido removido pelo reset local retorna 404. A consulta tem limite de oito segundos e valida a resposta antes de mostrá-la; falhas não usam estado em cache como confirmação.

Esta é uma demo loopback sem identidade de cliente: conhecer o ID permite consultar o pedido. Não exponha a API ou o BFF em rede pública sem implementar autorização/ownership.

A web não apresenta QR/código Pix. Checkout permanece desabilitado até o backend integrar o adapter merchant autenticado que vincule pedido e valor autoritativo; não usar o endpoint genérico do sample SDK como substituto.

## Verificações

```sh
npm run format:check
npm run lint
npm run check
npm test
npm run build
```

O Prettier inclui arquivos `.svelte` com plugin fixado. Os testes Vitest verificam a lógica server-side com HTTP simulado; ainda não são testes Playwright E2E contra backend/PostgreSQL reais.

As convenções obrigatórias estão em `../../.agents/skills/typescript-code-standards/SKILL.md`. O ESLint rejeita `null` como estado de aplicação, casts, non-null assertions, `forEach`, `if` sem bloco, bindings prefixados com `_`, testes sem “Should”, testes em `src/`, navegação sem `resolve()` e listas Svelte sem chave. Os testes ficam em `tests/unit/`; `npm test` também verifica exemplos positivos e negativos dessas regras.

O build compila cliente e servidor. O aviso final `adapter_missing` é esperado enquanto o destino de execução não for escolhido; deploy não foi escolhido nem autorizado. Dependências diretas estão fixadas em `package.json` e transitivas em `package-lock.json`.
