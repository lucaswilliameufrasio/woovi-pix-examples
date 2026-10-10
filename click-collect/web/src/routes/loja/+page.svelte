<script lang="ts">
  import { resolve } from "$app/paths";
  import type { ActionData, PageData } from "./$types";

  let { data, form }: { data: PageData; form: ActionData } = $props();
  const fulfillmentLabel = (state: string) =>
    ({ preparing: "Em preparo", ready_for_pickup: "Aguardando cliente", picked_up: "Entregue" })[state] ?? "Estado indisponível";
</script>

<svelte:head>
  <title>Fila da cozinha — Balcão</title>
  <meta name="robots" content="noindex, nofollow" />
</svelte:head>

<div class="site-shell shop-shell">
  <header class="masthead">
    <a class="wordmark" href={resolve("/")} aria-label="Balcão, início"><span class="wordmark-dot"></span>balcão</a>
    <span class="operator-link">PAINEL LOCAL · LOJA</span>
  </header>
  <main class="shop-main">
    <p class="eyebrow"><span class="live-dot"></span> TURNO DE HOJE · COZINHA</p>
    <div class="section-heading"><div><p class="eyebrow">PEDIDOS CONFIRMADOS</p><h1>Fila da cozinha</h1></div><span class="menu-index">{data.orders.length.toString().padStart(2, "0")} NA FILA</span></div>
    {#if data.unavailable}
      <div class="notice notice-error" role="status">Não foi possível abrir a fila. Verifique o backend e a configuração local.</div>
    {:else if form?.message}
      <div class="notice notice-error" role="alert">{form.message}</div>
    {/if}
    {#if data.orders.length === 0 && !data.unavailable}
      <div class="notice">Nenhum pedido confirmado. A cozinha respira.</div>
    {/if}
    <div class="order-list">
      {#each data.orders as order (order.id)}
        <article class="kitchen-ticket">
          <div class="ticket-number"><span>COMANDA</span><strong>{order.id.slice(0, 8).toUpperCase()}</strong></div>
          <div class="ticket-content"><div><h2>{order.product_title}</h2><p>1 unidade · pedido {new Date(order.created_at).toLocaleTimeString("pt-BR", { hour: "2-digit", minute: "2-digit" })}</p></div><span class="ticket-state">{fulfillmentLabel(order.fulfillment_state)}</span>
            {#if order.fulfillment_state === "preparing"}
              <form method="POST" action="?/ready"><input type="hidden" name="order_id" value={order.id} /><button class="button button-primary" type="submit">Marcar pronto <span aria-hidden="true">→</span></button></form>
            {:else if order.fulfillment_state === "ready_for_pickup"}
              <form class="pickup-form" method="POST" action="?/pickup"><input type="hidden" name="order_id" value={order.id} /><label for={`pickup-${order.id}`}>Código do cliente</label><div><input id={`pickup-${order.id}`} name="pickup_code" autocomplete="off" inputmode="text" maxlength="8" placeholder="8 caracteres" required /><button class="button button-dark" type="submit">Confirmar retirada</button></div></form>
            {/if}
          </div>
        </article>
      {/each}
    </div>
    <p class="demo-note">Painel de demonstração local; autenticação da loja é configurada no servidor e não representa login de produção.</p>
  </main>
  <footer><span>feito com cuidado, no bairro</span><span>BALCÃO · DEMO LOCAL</span></footer>
</div>
