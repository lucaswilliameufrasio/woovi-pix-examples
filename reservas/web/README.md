# Web/BFF de Reservas

Aplicação SvelteKit server-rendered. O browser só recebe horário selecionado, resultado da reserva e estados públicos; capability fica em cookie `HttpOnly` com `Path` limitado à reserva. `DEMO_OPERATOR_TOKEN` e `DEMO_SIMULATOR_TOKEN` são lidos apenas pelo servidor BFF. A página `/api-reference` renderiza o contrato canônico `backend/openapi.yaml` com Scalar; `/openapi.yaml` entrega esse mesmo arquivo sem cópia divergente.

O backend deve estar em um URL explícito loopback (`API_BASE_URL`, padrão `http://127.0.0.1:8084`). Execute `npm ci`, `npm run dev`; validação: `npm run format:check`, `npm run lint`, `npm run check`, `npm run build`. O E2E Chromium usa `npm run test:browser`, iniciado pelo runner em `tooling/run_reservations_smoke.py`.

Não hospedar/publicar esta configuração de demonstração: cookies não Secure, tokens demo e backend loopback não formam uma implantação de produção.
