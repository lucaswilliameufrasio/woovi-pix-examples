<script lang="ts">
  import { resolve } from "$app/paths";
  import type { PageProps } from "./$types";
  let { data, params, form }: PageProps = $props();
  const money = (cents: number) =>
    new Intl.NumberFormat("pt-BR", {
      style: "currency",
      currency: "BRL",
    }).format(cents / 100);
  const labels = {
    pending_payment: "Aguardando pagamento",
    paid: "Pagamento registrado",
    expired: "Reserva expirada",
    payment_exception: "Pagamento em análise",
  };
  const descriptions = {
    pending_payment:
      "Sua sacola está reservada temporariamente. O checkout Pix ainda não está habilitado nesta demo.",
    paid: "O backend registrou o pagamento local. A entrega depende da validação do operador na loja.",
    expired:
      "O prazo da reserva terminou. Consulte as ofertas atuais antes de fazer outro pedido.",
    payment_exception:
      "O pagamento exige atendimento do operador. Não há confirmação de disponibilidade nem devolução automática.",
  };
  const expiry = (date: string) =>
    new Intl.DateTimeFormat("pt-BR", {
      dateStyle: "medium",
      timeStyle: "short",
      timeZone: "UTC",
    }).format(new Date(date));
</script>

<svelte:head>
  <title>Seu pedido — Última Chamada</title>
  <meta name="robots" content="noindex, nofollow" />
</svelte:head>

<main>
  <a class="back" href={resolve("/")}>Voltar às ofertas</a>
  <h1>Seu pedido</h1>
  {#if data.lookup_error}
    <section class="error" role="alert">
      <h2>Consulta indisponível</h2>
      <p>{data.lookup_error.message}</p>
    </section>
  {:else if data.order}
    {@const order = data.order}
    <section aria-labelledby="order-state">
      <p class="reference">Pedido {order.id}</p>
      <h2 id="order-state">{labels[order.state]}</h2>
      <p>{descriptions[order.state]}</p>
      <dl>
        <dt>Valor do pedido</dt>
        <dd>{money(order.amount_cents)}</dd>
        <dt>Prazo original da reserva (UTC)</dt>
        <dd>{expiry(order.expires_at)}</dd>
      </dl>
    </section>
  {/if}
  {#if data.order?.state === "pending_payment"}
    <form method="POST" action="?/checkout">
      <button type="submit">Abrir sessão local sem Pix</button>
    </form>
  {/if}
  {#if form?.checkout}
    <p role="status">
      Sessão local {form.checkout.checkout_id}. Sem QR, código Pix ou pagamento
      real.
    </p>
  {:else if form && "message" in form}
    <p role="alert">{form.message}</p>
  {/if}
  <a
    class="refresh"
    href={resolve("/orders/[order_id]", { order_id: params.order_id })}
    data-sveltekit-reload>Atualizar estado</a
  >
  <p class="notice">
    Demonstração local, sem pagamento real. O estado vem do backend a cada
    consulta.
  </p>
</main>

<style>
  :global(body) {
    margin: 0;
    background: #f4f1e7;
    color: #183a2b;
    font-family: "Avenir Next", Avenir, "Segoe UI", sans-serif;
  }
  main {
    max-width: 640px;
    margin: 0 auto;
    padding: 40px 22px;
  }
  h1 {
    font-family: Georgia, serif;
    font-size: 42px;
    font-weight: 500;
  }
  h2 {
    font-size: 24px;
    margin: 14px 0;
  }
  section {
    border: 1px solid #cbd5c0;
    border-radius: 16px;
    padding: 24px;
    background: #fffdf7;
  }
  .error {
    border-color: #c27d54;
  }
  p {
    line-height: 1.6;
  }
  .reference {
    font-size: 12px;
    overflow-wrap: anywhere;
  }
  dl {
    display: grid;
    gap: 8px;
  }
  dt {
    font-size: 12px;
    margin-top: 12px;
  }
  dd {
    margin: 0;
    font-weight: 700;
  }
  a {
    color: #183a2b;
  }
  a:focus-visible,
  button:focus-visible {
    outline: 3px solid #c27d54;
    outline-offset: 4px;
  }
  .refresh,
  button {
    display: inline-block;
    padding: 14px 18px;
    margin-top: 22px;
    border-radius: 8px;
    background: #183a2b;
    color: white;
    text-decoration: none;
  }
  .notice {
    color: #526456;
    font-size: 12px;
    margin-top: 24px;
  }
</style>
