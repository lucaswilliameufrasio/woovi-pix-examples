<script lang="ts">
  import { resolve } from "$app/paths";
  import type { ActionData, PageData } from "./$types";
  let { data, form }: { data: PageData; form: ActionData } = $props();
</script>

<svelte:head>
  <title>Agenda — Reserva</title>
  <meta
    name="description"
    content="Escolha um horário disponível na agenda demonstrativa."
  />
</svelte:head>

<main>
  <header class="topbar">
    <span class="brand">RESERVA <i>•</i> LOCAL</span>
    <div>
      <a href={resolve("/operator")}>Operação local</a><span class="tag"
        >DEMONSTRAÇÃO</span
      >
      <a href={resolve("/api-reference")}>API</a>
    </div>
  </header>
  <section class="hero">
    <p class="eyebrow">AGENDA ABERTA · SÃO PAULO</p>
    <h1>Um tempo<br />só seu.</h1>
    <p class="intro">
      Escolha seu horário. A reserva fica retida por cinco minutos enquanto você
      confirma a simulação local.
    </p>
  </section>
  <section class="booking" aria-labelledby="agenda-title">
    <div class="booking-heading">
      <div>
        <p class="eyebrow">SALA DE ATENDIMENTO</p>
        <h2 id="agenda-title">Escolha um horário</h2>
      </div>
      <strong>R$ 120,00</strong>
    </div>
    <form method="GET" class="date-form">
      <label for="date">Dia da visita</label>
      <div class="date-control">
        <input
          id="date"
          name="date"
          type="date"
          value={data.date}
          required
        /><button type="submit"
          >Ver horários <span aria-hidden="true">↗</span></button
        >
      </div>
    </form>
    {#if form?.message}<p class="error" role="alert">{form.message}</p>{/if}
    {#if data.unavailable}<p class="error" role="status">{data.message}</p>
    {:else if data.availability}
      <p class="timezone">
        {data.availability.duration_minutes} minutos · fuso {data.availability
          .time_zone}
      </p>
      {#if data.availability.slots.length === 0}<p class="empty">
          Não há horários disponíveis neste dia. Tente outra data.
        </p>
      {:else}
        <div class="slots">
          {#each data.availability.slots as slot (slot.starts_at)}
            <form method="POST" action="?/reserve">
              <input type="hidden" name="starts_at" value={slot.starts_at} />
              <button class="slot" type="submit"
                ><span>{slot.local_label}</span><span aria-hidden="true">→</span
                ></button
              >
            </form>
          {/each}
        </div>
      {/if}
    {/if}
  </section>
  <footer>DEMO LOCAL · NENHUM PAGAMENTO REAL É PROCESSADO</footer>
</main>

<style>
  :global(*) {
    box-sizing: border-box;
  }
  :global(body) {
    margin: 0;
    background: #f1efe8;
    color: #1f2c28;
    font-family: Arial, sans-serif;
  }
  main {
    max-width: 1100px;
    margin: auto;
    padding: 28px 6vw 24px;
  }
  .topbar {
    display: flex;
    justify-content: space-between;
    align-items: center;
    border-bottom: 1px solid #c8c8bd;
    padding-bottom: 18px;
    font-size: 12px;
    letter-spacing: 0.15em;
  }
  .topbar > div {
    display: flex;
    align-items: center;
    gap: 18px;
  }
  .topbar a {
    color: #24372e;
    text-decoration: none;
    font-size: 11px;
    letter-spacing: 0;
  }
  .brand {
    font-weight: 800;
  }
  .brand i {
    color: #bb6546;
    font-style: normal;
  }
  .tag,
  .eyebrow {
    font-size: 10px;
    letter-spacing: 0.18em;
    font-weight: 700;
  }
  .tag {
    border: 1px solid #9c9c8d;
    border-radius: 30px;
    padding: 8px 12px;
  }
  .hero {
    padding: 48px 0 38px;
    max-width: 560px;
  }
  .eyebrow {
    color: #9b5e43;
  }
  .hero h1 {
    font-family: Georgia, serif;
    font-size: clamp(52px, 8vw, 86px);
    line-height: 0.92;
    letter-spacing: -0.055em;
    font-weight: 400;
    margin: 14px 0;
  }
  .intro {
    font-size: 16px;
    line-height: 1.6;
    color: #5d645e;
    max-width: 390px;
  }
  .booking {
    background: #fffefa;
    border: 1px solid #d9d7cc;
    padding: 28px;
    max-width: 650px;
    box-shadow: 8px 8px 0 #dedbd1;
  }
  .booking-heading {
    display: flex;
    justify-content: space-between;
    align-items: end;
    gap: 16px;
  }
  .booking-heading h2 {
    font:
      400 28px Georgia,
      serif;
    margin: 8px 0;
  }
  .booking-heading strong {
    font:
      20px Georgia,
      serif;
  }
  .date-form {
    margin-top: 24px;
  }
  .date-form label {
    display: block;
    font-size: 12px;
    font-weight: 700;
    margin-bottom: 8px;
  }
  .date-control {
    display: flex;
    gap: 10px;
  }
  .date-control input {
    flex: 1;
    min-width: 0;
    padding: 12px;
    border: 1px solid #c9c7bc;
    background: white;
    font: inherit;
  }
  .date-control button,
  .slot {
    border: 0;
    background: #24372e;
    color: white;
    padding: 12px 16px;
    font: inherit;
    cursor: pointer;
  }
  .date-control button span {
    margin-left: 12px;
  }
  .timezone {
    font-size: 12px;
    color: #73776f;
    margin: 20px 0 12px;
  }
  .slots {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 8px;
  }
  .slot {
    width: 100%;
    display: flex;
    justify-content: space-between;
    text-align: left;
    background: #eff0e9;
    color: #24372e;
    border: 1px solid #d5d8ce;
  }
  .slot:hover {
    background: #dce4d9;
  }
  .empty,
  .error {
    padding: 12px;
    background: #f7e7df;
    color: #743b2b;
    font-size: 14px;
  }
  .error {
    margin-top: 16px;
  }
  .empty {
    background: #f1efe8;
    color: #5d645e;
  }
  footer {
    font-size: 9px;
    letter-spacing: 0.15em;
    color: #74796f;
    margin-top: 38px;
  }
  @media (max-width: 560px) {
    main {
      padding: 20px;
    }
    .booking {
      padding: 20px;
    }
    .slots {
      grid-template-columns: 1fr;
    }
    .hero {
      padding-top: 38px;
    }
    .booking-heading {
      align-items: start;
      flex-direction: column;
    }
  }
</style>
