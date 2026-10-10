<script lang="ts">
  import { resolve } from "$app/paths";
  import type { ActionData, PageData } from "./$types";

  let { data, form }: { data: PageData; form: ActionData } = $props();

  const paymentLabel = (state: string) =>
    ({ pending: "Aguardando confirmação", paid: "Confirmado", expired: "Reserva vencida", cancelled: "Cancelado", payment_exception: "Confirmação em análise" })[state] ?? "Indisponível";
  const fulfillmentLabel = (state: string) =>
    ({ awaiting_payment: "Aguardando confirmação", preparing: "A cozinha está preparando", ready_for_pickup: "Pronto para retirar", picked_up: "Retirado" })[state] ?? "Indisponível";
</script>

<svelte:head>
  <title>{data.order ? `Pedido ${data.order.id} — Balcão` : "Pedido privado — Balcão"}</title>
  <meta name="robots" content="noindex, nofollow" />
</svelte:head>

<div class="site-shell order-shell">
  <header class="masthead">
    <a class="wordmark" href={resolve("/")} aria-label="Balcão, início"><span class="wordmark-dot"></span>balcão</a>
    <span class="operator-link">ACOMPANHAMENTO PRIVADO</span>
  </header>
  <main class="order-main">
    {#if data.unavailable || !data.order}
      <section class="status-card">
        <p class="eyebrow">ACESSO PRIVADO</p><h1>Não encontramos este pedido.</h1>
        <p>O acesso é salvo neste navegador e não pode ser recuperado apenas pelo número do pedido.</p>
        <a class="button button-secondary" href={resolve("/")}>Voltar à vitrine</a>
      </section>
    {:else}
      <p class="eyebrow"><span class="live-dot"></span> SEU PEDIDO · {data.order.id.slice(0, 8).toUpperCase()}</p>
      <h1 class="order-title">{data.order.product_title}<br /><em>está com a gente.</em></h1>
      <section class="status-card" aria-labelledby="status-title">
        <div class="status-head"><div><p class="eyebrow">ATUALIZAÇÃO DA COZINHA</p><h2 id="status-title">{fulfillmentLabel(data.order.fulfillment_state)}</h2></div><span class="status-mark" aria-hidden="true">✳</span></div>
        <div class="status-rule"></div>
        <dl class="order-details"><div><dt>Pedido</dt><dd>{data.order.id}</dd></div><div><dt>Pagamento</dt><dd>{paymentLabel(data.order.payment_state)}</dd></div><div><dt>Total</dt><dd>{new Intl.NumberFormat("pt-BR", { style: "currency", currency: "BRL" }).format(data.order.amount_cents / 100)}</dd></div></dl>
        {#if data.order.fulfillment_state === "ready_for_pickup" && data.pickupCode}
          <div class="pickup-pass"><p class="eyebrow">MOSTRE ESTE CÓDIGO NO BALCÃO</p><strong>{data.pickupCode.match(/.{1,4}/g)?.join(" ")}</strong><span>Válido somente para este pedido.</span></div>
        {/if}
        {#if data.order.payment_state === "pending"}
          <div class="action-row"><form method="POST" action="?/simulate"><button class="button button-primary" type="submit">Simular confirmação local</button></form><form method="POST" action="?/cancel"><button class="text-button" type="submit">Cancelar reserva</button></form></div>
          <p class="demo-note">Simulação local, sem Pix pagável e sem integração com instituição financeira.</p>
        {/if}
        {#if form?.message}<p class="form-error" role="alert">{form.message}</p>{/if}
        <form method="POST" action="?/refresh" class="refresh-form"><button class="text-button" type="submit">Atualizar status ↻</button></form>
      </section>
      <a class="back-link" href={resolve("/")}>← voltar à vitrine</a>
    {/if}
  </main>
  <footer><span>feito com cuidado, no bairro</span><span>BALCÃO · DEMO LOCAL</span></footer>
</div>
