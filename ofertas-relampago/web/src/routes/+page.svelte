<script lang="ts">
  import { resolve } from "$app/paths";
  import type { ActionData, PageData } from "./$types";

  let { data, form }: { data: PageData; form: ActionData } = $props();
  let submitting = $state(false);

  const money = (cents: number) =>
    new Intl.NumberFormat("pt-BR", {
      style: "currency",
      currency: "BRL",
    }).format(cents / 100);
  const stateLabel = (state: string) =>
    ({
      pending_payment: "Aguardando pagamento",
      paid: "Pago — aguardando retirada",
      expired: "Reserva expirada",
      payment_exception: "Pagamento em análise",
    })[state] ?? "Estado indisponível";
</script>

<svelte:head>
  <title>Última Chamada — boas sacolas, menos desperdício</title>
  <meta
    name="description"
    content="Reserve uma sacola-surpresa de uma loja da vizinhança. Demonstração local sem pagamento real."
  />
</svelte:head>

<div class="page-shell">
  <header class="topbar">
    <a class="brand" href={resolve("/")} aria-label="Última Chamada, início">
      <span class="brand-mark" aria-hidden="true">↗</span>
      <span>última chamada<span class="brand-period">.</span></span>
    </a>
    <div class="local-pill"><span></span> DEMO LOCAL</div>
  </header>

  <main>
    <section class="hero" aria-labelledby="hero-title">
      <div class="hero-copy">
        <p class="eyebrow"><span>✳</span> BOM DEMAIS PRA DESCARTAR</p>
        <h1 id="hero-title">O fim do dia<br />chega com <em>sabor.</em></h1>
        <p class="hero-description">
          Sacolas-surpresa de lugares que você já gosta.<br
            class="desktop-break"
          />
          Reserve agora, retire na loja.
        </p>
        <div class="hero-notes">
          <span>↳ Retirada presencial</span>
          <span>↳ Estoque de verdade</span>
        </div>
      </div>
      <div class="hero-art" aria-hidden="true">
        <div class="sun"></div>
        <div class="orbit orbit-one"></div>
        <div class="orbit orbit-two"></div>
        <div class="bag">
          <div class="bag-handle"></div>
          <div class="bag-stamp">BOM<br />DAQUI</div>
          <div class="bag-leaf">✳</div>
        </div>
        <div class="art-caption">feito perto.<br />aproveitado inteiro.</div>
      </div>
    </section>

    <section class="offers-section" aria-labelledby="offers-title">
      <div class="section-heading">
        <div>
          <p class="eyebrow dark">A SELEÇÃO DE HOJE</p>
          <h2 id="offers-title">
            Boas descobertas,<br class="mobile-break" /> a um passo.
          </h2>
        </div>
        <div class="pickup-label">
          <span aria-hidden="true">⌖</span> na sua vizinhança
        </div>
      </div>

      {#if form?.order}
        <aside class="order-result" aria-live="polite">
          <span class="result-icon" aria-hidden="true">✓</span>
          <div>
            <strong>Reserva criada</strong>
            <p>Pedido {form.order.id} · {money(form.order.amount_cents)}</p>
            <p class="state">{stateLabel(form.order.state)}</p>
            <p class="disclaimer">
              Esta demonstração não inicia pagamento Pix.
            </p>
            <a href={resolve("/orders/[order_id]", { order_id: form.order.id })}
              >Acompanhar pedido</a
            >
          </div>
        </aside>
      {/if}

      {#if form && "message" in form}
        <aside class="error-box" role="alert">{form.message}</aside>
      {/if}

      {#if data.loadError}
        <aside class="error-box" role="alert">
          <strong>A loja está quietinha.</strong>
          <p>{data.loadError.message}</p>
          <p class="error-code">Código: {data.loadError.error_code}</p>
        </aside>
      {:else if data.offers.length === 0}
        <div class="empty-state">
          <span aria-hidden="true">◌</span>
          <p>As sacolas de hoje já encontraram casa.</p>
        </div>
      {:else}
        <div class="offer-grid">
          {#each data.offers as offer (offer.id)}
            <article
              class:unavailable={offer.available_units < 1}
              class="offer-card"
            >
              <div class="offer-visual" aria-hidden="true">
                <div class="produce produce-a">✿</div>
                <div class="produce produce-b">◒</div>
                <div class="produce produce-c">✳</div>
                <div class="paper-bag"><span>da<br />feira</span></div>
                <span class="visual-tag">SURPRESA DO DIA</span>
              </div>
              <div class="offer-body">
                <div class="offer-meta">
                  <span
                    class:scarce={offer.available_units > 0 &&
                      offer.available_units <= 2}
                  >
                    {offer.available_units > 0
                      ? `${offer.available_units} ${offer.available_units === 1 ? "sacola restante" : "sacolas restantes"}`
                      : "esgotado por hoje"}
                  </span>
                  <span>RETIRADA NA LOJA</span>
                </div>
                <h3>{offer.title}</h3>
                <p class="offer-subtitle">
                  Uma seleção gostosa feita pra aproveitar.
                </p>
                <div class="offer-footer">
                  <strong class="price">{money(offer.price_cents)}</strong>
                  <form
                    method="POST"
                    action="?/reserve"
                    onsubmit={() => (submitting = true)}
                  >
                    <input type="hidden" name="offer_id" value={offer.id} />
                    <button
                      type="submit"
                      disabled={offer.available_units < 1 || submitting}
                    >
                      {submitting
                        ? "Reservando…"
                        : offer.available_units > 0
                          ? "Reservar sacola"
                          : "Esgotado"}
                    </button>
                  </form>
                </div>
              </div>
            </article>
          {/each}
        </div>
      {/if}
    </section>
  </main>

  <footer>
    <span>Última Chamada <span class="brand-period">✳</span></span>
    <span>DEMO LOCAL · SEM PAGAMENTO REAL</span>
  </footer>
</div>

<style>
  :global(*) {
    box-sizing: border-box;
  }
  :global(body) {
    margin: 0;
    background: #f4f1e7;
    color: #183a2b;
    font-family: "Avenir Next", Avenir, "Segoe UI", sans-serif;
  }
  :global(button),
  :global(input) {
    font: inherit;
  }
  .page-shell {
    max-width: 1440px;
    margin: 0 auto;
    padding: 0 6.1%;
  }
  .topbar {
    height: 82px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    border-bottom: 1px solid #d9ddcf;
  }
  .brand {
    display: inline-flex;
    align-items: center;
    gap: 10px;
    color: #183a2b;
    text-decoration: none;
    font-size: 19px;
    font-weight: 800;
    letter-spacing: -1px;
  }
  .brand-mark {
    display: grid;
    place-items: center;
    width: 31px;
    height: 31px;
    border-radius: 50%;
    color: #d9f36b;
    background: #183a2b;
    font-size: 18px;
    transform: rotate(-35deg);
  }
  .brand-period {
    color: #e98d43;
  }
  .local-pill {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 12px;
    border: 1px solid #d9ddcf;
    border-radius: 30px;
    color: #526456;
    font-size: 9px;
    font-weight: 800;
    letter-spacing: 1px;
  }
  .local-pill span {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: #e99047;
  }
  .hero {
    min-height: 430px;
    display: grid;
    grid-template-columns: 1.05fr 0.95fr;
    align-items: center;
    overflow: hidden;
    border-bottom: 1px solid #d9ddcf;
  }
  .hero-copy {
    padding: 55px 0 58px;
    position: relative;
    z-index: 1;
  }
  .eyebrow {
    margin: 0 0 22px;
    color: #d9f36b;
    font-size: 10px;
    font-weight: 800;
    letter-spacing: 1.5px;
  }
  .eyebrow span {
    margin-right: 6px;
    font-size: 16px;
    vertical-align: -1px;
  }
  h1 {
    margin: 0;
    color: #183a2b;
    font-family: Georgia, "Times New Roman", serif;
    font-size: clamp(48px, 6.5vw, 83px);
    font-weight: 500;
    line-height: 0.99;
    letter-spacing: -4.5px;
  }
  h1 em {
    color: #df8241;
    font-weight: 500;
  }
  .hero-description {
    margin: 23px 0 0;
    color: #647066;
    font-size: 15px;
    line-height: 1.65;
  }
  .hero-notes {
    display: flex;
    gap: 22px;
    margin-top: 28px;
    color: #385847;
    font-size: 11px;
    font-weight: 700;
  }
  .hero-art {
    height: 360px;
    position: relative;
    display: grid;
    place-items: center;
  }
  .sun {
    position: absolute;
    width: 245px;
    height: 245px;
    top: 49px;
    right: 18%;
    border-radius: 50%;
    background: #e5edc8;
  }
  .orbit {
    position: absolute;
    border: 1px solid #c3cfaa;
    border-radius: 50%;
    transform: rotate(-24deg);
  }
  .orbit-one {
    width: 370px;
    height: 150px;
  }
  .orbit-two {
    width: 300px;
    height: 112px;
    transform: rotate(37deg);
  }
  .bag {
    position: relative;
    z-index: 1;
    width: 174px;
    height: 194px;
    margin-top: 35px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: #db874a;
    clip-path: polygon(9% 13%, 91% 13%, 82% 100%, 17% 100%);
    box-shadow: inset -18px 0 #cb713a;
  }
  .bag:before {
    content: "";
    position: absolute;
    inset: 0 0 auto;
    height: 24px;
    background: #bd6839;
  }
  .bag-handle {
    position: absolute;
    width: 60px;
    height: 38px;
    top: -16px;
    border: 9px solid #bd6839;
    border-bottom: 0;
    border-radius: 40px 40px 0 0;
  }
  .bag-stamp {
    margin-top: 20px;
    color: #fff0d1;
    font-family: Georgia, serif;
    font-size: 24px;
    line-height: 0.84;
    font-weight: 700;
    text-align: center;
    transform: rotate(-6deg);
  }
  .bag-leaf {
    position: absolute;
    right: 31px;
    bottom: 24px;
    color: #d9f36b;
    font-size: 24px;
  }
  .art-caption {
    position: absolute;
    right: 2%;
    bottom: 22px;
    color: #526456;
    font-family: Georgia, serif;
    font-size: 13px;
    line-height: 1.4;
    font-style: italic;
  }
  .offers-section {
    padding: 57px 0 76px;
  }
  .section-heading {
    display: flex;
    justify-content: space-between;
    align-items: end;
    margin-bottom: 28px;
  }
  .eyebrow.dark {
    margin-bottom: 12px;
    color: #66804a;
    font-size: 9px;
  }
  h2 {
    margin: 0;
    color: #183a2b;
    font-family: Georgia, "Times New Roman", serif;
    font-size: 43px;
    line-height: 1.06;
    font-weight: 500;
    letter-spacing: -1.5px;
  }
  .mobile-break {
    display: none;
  }
  .pickup-label {
    padding-bottom: 7px;
    color: #647066;
    font-size: 11px;
  }
  .pickup-label span {
    color: #df8241;
    font-size: 17px;
    vertical-align: -2px;
  }
  .offer-grid {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 19px;
  }
  .offer-card {
    overflow: hidden;
    background: #fffdf7;
    border: 1px solid #e3e4d9;
    border-radius: 13px;
    transition:
      transform 0.2s,
      box-shadow 0.2s;
  }
  .offer-card:hover {
    transform: translateY(-3px);
    box-shadow: 0 14px 30px #29432b12;
  }
  .offer-card.unavailable {
    opacity: 0.72;
  }
  .offer-visual {
    position: relative;
    height: 177px;
    overflow: hidden;
    background: #e5edcf;
  }
  .offer-visual:before {
    content: "";
    position: absolute;
    width: 150px;
    height: 150px;
    left: 50%;
    top: 40px;
    transform: translateX(-50%);
    border: 1px solid #c4d3a8;
    border-radius: 50%;
  }
  .paper-bag {
    position: absolute;
    left: 50%;
    top: 27px;
    width: 94px;
    height: 122px;
    display: grid;
    place-items: center;
    transform: translateX(-50%) rotate(3deg);
    background: #d88448;
    clip-path: polygon(8% 8%, 92% 8%, 84% 100%, 16% 100%);
    box-shadow: inset -12px 0 #c7743c;
  }
  .paper-bag:before {
    content: "";
    position: absolute;
    top: 3px;
    width: 51px;
    height: 24px;
    border: 5px solid #b86636;
    border-bottom: 0;
    border-radius: 30px 30px 0 0;
  }
  .paper-bag span {
    margin-top: 17px;
    color: #fff1d8;
    font-family: Georgia, serif;
    font-size: 17px;
    font-weight: 700;
    line-height: 0.9;
    text-align: center;
  }
  .produce {
    position: absolute;
    z-index: 1;
    display: grid;
    place-items: center;
    width: 39px;
    height: 39px;
    border-radius: 50%;
    font-size: 21px;
  }
  .produce-a {
    top: 54px;
    left: calc(50% - 63px);
    color: #f3c75b;
    background: #e27648;
  }
  .produce-b {
    top: 85px;
    left: calc(50% + 43px);
    color: #f4e8bc;
    background: #779058;
  }
  .produce-c {
    top: 31px;
    left: calc(50% + 35px);
    color: #d9f36b;
    background: #4d7652;
  }
  .visual-tag {
    position: absolute;
    right: 12px;
    bottom: 11px;
    color: #61744c;
    font-size: 8px;
    font-weight: 800;
    letter-spacing: 1px;
  }
  .offer-body {
    padding: 17px 17px 15px;
  }
  .offer-meta {
    display: flex;
    justify-content: space-between;
    gap: 8px;
    color: #74806c;
    font-size: 8px;
    font-weight: 800;
    letter-spacing: 0.65px;
    text-transform: uppercase;
  }
  .offer-meta span:first-child {
    color: #39704b;
  }
  .offer-meta span.scarce {
    color: #c26736;
  }
  h3 {
    margin: 10px 0 4px;
    color: #203b2b;
    font-family: Georgia, serif;
    font-size: 19px;
    font-weight: 600;
    letter-spacing: -0.3px;
  }
  .offer-subtitle {
    margin: 0;
    color: #788075;
    font-size: 11px;
  }
  .offer-footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    margin-top: 18px;
    padding-top: 14px;
    border-top: 1px solid #e9e9df;
  }
  .price {
    color: #183a2b;
    font-size: 17px;
    font-weight: 800;
    letter-spacing: -0.5px;
  }
  button {
    min-height: 39px;
    padding: 0 15px;
    border: 0;
    border-radius: 6px;
    background: #183a2b;
    color: white;
    cursor: pointer;
    font-size: 11px;
    font-weight: 700;
    transition: background 0.15s;
  }
  button:hover:not(:disabled) {
    background: #2c6041;
  }
  button:focus-visible,
  a:focus-visible {
    outline: 3px solid #db8948;
    outline-offset: 3px;
  }
  button:disabled {
    background: #b7bfb3;
    cursor: not-allowed;
  }
  .order-result {
    display: flex;
    gap: 13px;
    margin: 0 0 22px;
    padding: 16px 18px;
    border: 1px solid #bfd19d;
    border-radius: 9px;
    background: #edf3dd;
  }
  .result-icon {
    display: grid;
    flex: 0 0 28px;
    height: 28px;
    place-items: center;
    border-radius: 50%;
    background: #315e3e;
    color: white;
    font-weight: 800;
  }
  .order-result strong {
    color: #183a2b;
    font-size: 14px;
  }
  .order-result p {
    margin: 4px 0 0;
    color: #526456;
    font-size: 11px;
  }
  .order-result p.state {
    color: #294d36;
    font-weight: 800;
  }
  .order-result p.disclaimer {
    color: #85836f;
    font-size: 10px;
  }
  .error-box {
    margin: 0 0 20px;
    padding: 16px;
    border: 1px solid #e7b99d;
    border-radius: 9px;
    background: #fff0e5;
    color: #753f26;
    font-size: 13px;
  }
  .error-box p {
    margin: 5px 0;
  }
  .error-code {
    font-size: 10px;
  }
  .empty-state {
    display: grid;
    min-height: 160px;
    place-content: center;
    justify-items: center;
    border: 1px dashed #bdc6b0;
    border-radius: 12px;
    color: #647066;
  }
  .empty-state span {
    color: #df8241;
    font-size: 30px;
  }
  .empty-state p {
    font-family: Georgia, serif;
    font-size: 17px;
  }
  footer {
    min-height: 66px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    border-top: 1px solid #d9ddcf;
    color: #647066;
    font-size: 10px;
    font-weight: 700;
  }
  footer span:last-child {
    font-size: 8px;
    letter-spacing: 1px;
  }
  @media (min-width: 1250px) {
    .page-shell {
      padding-right: 8%;
      padding-left: 8%;
    }
  }
  @media (max-width: 800px) {
    .page-shell {
      padding: 0 5%;
    }
    .topbar {
      height: 68px;
    }
    .hero {
      min-height: 380px;
      grid-template-columns: 1fr 0.72fr;
    }
    h1 {
      font-size: clamp(48px, 8vw, 66px);
      letter-spacing: -3px;
    }
    .hero-art {
      transform: scale(0.78);
      transform-origin: center right;
      margin-right: -45px;
    }
    .hero-description {
      font-size: 13px;
    }
    .hero-notes {
      flex-direction: column;
      gap: 8px;
    }
    .offer-grid {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }
  @media (max-width: 560px) {
    .topbar {
      height: 62px;
    }
    .brand {
      font-size: 17px;
    }
    .hero {
      min-height: 0;
      display: flex;
      flex-direction: column;
      align-items: stretch;
    }
    .hero-copy {
      padding: 39px 0 0;
    }
    .eyebrow {
      margin-bottom: 17px;
      font-size: 8px;
    }
    h1 {
      font-size: 57px;
      line-height: 0.98;
      letter-spacing: -3px;
    }
    .hero-description {
      margin-top: 17px;
      font-size: 12px;
    }
    .desktop-break {
      display: none;
    }
    .hero-notes {
      flex-direction: row;
      gap: 14px;
      margin-top: 18px;
      font-size: 9px;
    }
    .hero-art {
      width: 100%;
      height: 225px;
      margin: -1px 0 0;
      transform: scale(0.78);
      transform-origin: center;
    }
    .sun {
      top: 1px;
      right: auto;
      width: 210px;
      height: 210px;
    }
    .bag {
      width: 143px;
      height: 160px;
      margin-top: 10px;
    }
    .orbit-one {
      width: 310px;
      height: 115px;
    }
    .orbit-two {
      width: 250px;
      height: 100px;
    }
    .art-caption {
      right: 0;
      bottom: 3px;
      font-size: 11px;
    }
    .offers-section {
      padding: 39px 0 53px;
    }
    .section-heading {
      align-items: end;
      margin-bottom: 19px;
    }
    h2 {
      font-size: 34px;
      letter-spacing: -1px;
    }
    .mobile-break {
      display: initial;
    }
    .pickup-label {
      max-width: 100px;
      padding-bottom: 3px;
      text-align: right;
      font-size: 9px;
    }
    .offer-grid {
      grid-template-columns: 1fr;
      gap: 13px;
    }
    .offer-card {
      display: grid;
      grid-template-columns: 118px 1fr;
    }
    .offer-visual {
      height: 100%;
      min-height: 180px;
    }
    .paper-bag {
      top: 45px;
      width: 75px;
      height: 100px;
    }
    .paper-bag span {
      font-size: 14px;
    }
    .produce {
      width: 29px;
      height: 29px;
      font-size: 16px;
    }
    .produce-a {
      top: 57px;
      left: calc(50% - 48px);
    }
    .produce-b {
      top: 97px;
      left: calc(50% + 31px);
    }
    .produce-c {
      top: 38px;
      left: calc(50% + 26px);
    }
    .visual-tag {
      left: 5px;
      right: 5px;
      bottom: 7px;
      text-align: center;
      font-size: 6px;
    }
    .offer-body {
      min-width: 0;
      padding: 13px 11px 11px;
    }
    .offer-meta {
      font-size: 7px;
      letter-spacing: 0.2px;
    }
    h3 {
      margin-top: 9px;
      font-size: 16px;
      line-height: 1.1;
    }
    .offer-subtitle {
      font-size: 9px;
      line-height: 1.3;
    }
    .offer-footer {
      align-items: end;
      gap: 5px;
      margin-top: 12px;
      padding-top: 10px;
    }
    .price {
      font-size: 14px;
    }
    button {
      min-height: 35px;
      padding: 0 9px;
      font-size: 9px;
    }
    footer {
      align-items: flex-start;
      flex-direction: column;
      justify-content: center;
      gap: 5px;
      padding: 13px 0;
    }
  }
</style>
