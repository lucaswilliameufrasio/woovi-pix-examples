<script lang="ts">
  import { resolve } from "$app/paths";
  import type { ActionData, PageData } from "./$types";

  let { data, form }: { data: PageData; form: ActionData } = $props();

  const money = (cents: number) =>
    new Intl.NumberFormat("pt-BR", { style: "currency", currency: "BRL" }).format(cents / 100);
</script>

<svelte:head>
  <title>Balcão — feito hoje, no seu tempo</title>
  <meta name="description" content="Peça direto da cozinha da vizinhança. Demonstração local, sem pagamento real." />
</svelte:head>

<div class="site-shell">
  <header class="masthead">
    <a class="wordmark" href={resolve("/")} aria-label="Balcão, início"><span class="wordmark-dot"></span>balcão</a>
    <a class="operator-link" href={resolve("/loja")}>visão da loja <span aria-hidden="true">↗</span></a>
  </header>

  <main>
    <section class="hero" aria-labelledby="hero-title">
      <p class="eyebrow"><span class="live-dot"></span> COZINHA ABERTA · HOJE</p>
      <h1 id="hero-title">Feito hoje.<br /><em>Retirado por você.</em></h1>
      <p class="hero-copy">Peça direto do balcão. A gente prepara com calma; você leva quentinho.</p>
      <div class="hero-stamp" aria-hidden="true"><span>FEITO<br />NA LOJA</span><b>✳</b></div>
    </section>

    <section class="menu-section" aria-labelledby="menu-title">
      <div class="section-heading">
        <div><p class="eyebrow">DA VITRINE PARA A SUA MESA</p><h2 id="menu-title">Hoje no balcão</h2></div>
        <span class="menu-index">01 / 01</span>
      </div>

      {#if data.unavailable}
        <div class="notice notice-error" role="status">O balcão está temporariamente offline. Tente de novo em instantes.</div>
      {:else if data.products.length === 0}
        <div class="notice" role="status">A vitrine está vazia por enquanto. Volte mais tarde.</div>
      {:else}
        {#each data.products as product (product.id)}
          <article class="product-card">
            <div class="product-art" aria-hidden="true">
              <div class="plate"><div class="cake"><span></span><i></i><b></b></div></div>
              <span class="art-label">DA CASA<br />DESDE CEDO</span>
            </div>
            <div class="product-copy">
              <p class="product-kicker">FEITO NA COZINHA · HOJE</p>
              <h3>{product.title}</h3>
              <p class="product-description">{product.description}</p>
              <div class="product-bottom">
                <div><strong>{money(product.price_cents)}</strong><span>por unidade</span></div>
                {#if product.available_units > 0}
                  <form method="POST" action="?/reserve">
                    <input type="hidden" name="product_id" value={product.id} />
                    <button class="button button-primary" type="submit">Reservar uma fatia <span aria-hidden="true">→</span></button>
                  </form>
                {:else}
                  <span class="sold-out">Acabou por hoje</span>
                {/if}
              </div>
              {#if form?.message}<p class="form-error" role="alert">{form.message}</p>{/if}
            </div>
          </article>
        {/each}
      {/if}
      <p class="demo-note"><span aria-hidden="true">✳</span> Demonstração local. Nenhum pagamento real é processado.</p>
    </section>
  </main>

  <footer><span>feito com cuidado, no bairro</span><span>BALCÃO · DEMO LOCAL</span></footer>
</div>
