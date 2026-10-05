# Ofertas-relâmpago — web

O [roteiro integrado](../../docs/local.md) explica reserva, confirmação HTTP simulada e atualização da página. [Sandbox ainda não está integrado](../../docs/sandbox.md).

Interface SvelteKit 3 com Svelte 5 e TypeScript. Consulta o backend próprio da demo por chamadas server-side; o browser não recebe URL ou segredo do operador. A reserva cria apenas um pedido local e **não inicia pagamento Pix**.

## Executar localmente

Requer Node 24.21.0 via mise. Inicie o backend conforme `../backend/README.md` e rode:

```sh
cd ofertas-relampago/web
npm ci
API_BASE_URL=http://127.0.0.1:8080 npm run dev -- --host 127.0.0.1
```

`API_BASE_URL` é lida somente pelo servidor. A API deve estar em loopback e em modo demo. O catálogo usa `GET /v1/offers`; o formulário envia `POST /v1/orders` com `offer_id` ao backend.

Após a reserva, o BFF guarda a capability emitida pelo backend em cookie **HttpOnly, SameSite=Strict, com path do pedido e validade de vinte minutos**. “Acompanhar pedido” abre `/orders/{order_id}` e consulta `GET /v1/customer/orders/{order_id}` autenticado. Token não aparece no HTML, action/load data, JavaScript ou URL. Sem cookie, após seu prazo ou após reset: 401; copiar o link ou restaurar apenas um ID não concede acesso. Não há recuperação da credencial por ID nem refresh automático; não crie outra reserva para contornar perda de acesso.

A página permite refresh autoritativo e **Abrir sessão local sem Pix** por POST autenticado pelo BFF, com chave estável por pedido. Repetir a ação recupera a mesma referência, não duplica cobrança/estoque. Apenas ID/mode da sessão é retornado à UI; grant de checkout é descartado pelo BFF, sem permissão operacional. Prazo UTC explícito; erros não exibem cache como confirmação. Chamadas com credenciais usam fetch nativo server-only, prazo de oito segundos, base loopback validada e redirects recusados. Respostas HTML/actions usam `no-store`/`no-referrer`.

Capability prova posse da reserva local, não identidade humana/login de produção. Mobile também usa consulta privada. O caminho legado `/v1/orders/{order_id}` exige a mesma capability; não há consulta anônima por ID. Operador tem rota própria de diagnóstico, sem reemitir acesso do cliente. Não exponha API/BFF em rede pública; autenticação completa e webhook PSP permanecem pendentes.

A web não apresenta QR/código Pix. A sessão local já vincula pedido/cobrança/valor, mas checkout Pix/SDK permanece desabilitado até integrar payload legítimo e adapter PSP; não usar o endpoint genérico do sample SDK como substituto.

## Verificações

```sh
npm run format:check
npm run lint
npm run check
npm test
npm run build
```

O Prettier inclui arquivos `.svelte` com plugin fixado. Os testes Vitest verificam a lógica server-side com HTTP simulado; ainda não são testes Playwright E2E contra backend/PostgreSQL reais.

Da raiz, com PostgreSQL healthy e dependências web instaladas, `python3 tooling/run_offers_smoke.py --web` sobe backend e BFF reais em portas efêmeras, valida cookie privado/HTML sem credenciais e percorre reserva → sessão → webhook/worker → consulta → retirada. Usa schemas exclusivos e mantém dados para inspeção. É teste HTTP/SSR, **não** execução de browser JavaScript/dispositivo.

As convenções obrigatórias estão em `../../.agents/skills/typescript-code-standards/SKILL.md`. O ESLint rejeita `null` como estado de aplicação, casts, non-null assertions, `forEach`, `if` sem bloco, bindings prefixados com `_`, testes sem “Should”, testes em `src/`, navegação sem `resolve()` e listas Svelte sem chave. Os testes ficam em `tests/unit/`; `npm test` também verifica exemplos positivos e negativos dessas regras.

O build compila cliente e servidor. O aviso final `adapter_missing` é esperado enquanto o destino de execução não for escolhido; deploy não foi escolhido nem autorizado. Dependências diretas estão fixadas em `package.json` e transitivas em `package-lock.json`.
