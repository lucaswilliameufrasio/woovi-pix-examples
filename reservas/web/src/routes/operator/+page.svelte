<script lang="ts">
  import { resolve } from "$app/paths";
  import type { ActionData, PageData } from "./$types";
  let { data, form }: { data: PageData; form: ActionData } = $props();
</script>

<svelte:head><title>Operação local — Reserva</title></svelte:head>
<main>
  <header>
    <a href={resolve("/")}>← Agenda</a><span>OPERAÇÃO LOCAL</span>
  </header>
  <h1>Reservas confirmadas</h1>
  <p class="intro">
    Esta tela é uma operação local de demonstração, não um login de produção.
  </p>
  {#if data.unavailable}<p role="alert">
      {data.unavailable}
    </p>{:else if data.reservations.length === 0}<p>
      Nenhuma reserva paga foi confirmada ainda.
    </p>{:else}<div class="list">
      {#each data.reservations as reservation (reservation.id)}<article>
          <p>{reservation.local_label}</p>
          <small
            >{reservation.reservation_state} · pagamento {reservation.payment_state}</small
          >{#if reservation.reservation_state === "confirmed"}<form
              method="POST"
              action="?/complete"
            >
              <input
                type="hidden"
                name="reservation_id"
                value={reservation.id}
              /><button type="submit">Marcar atendimento concluído</button>
            </form>{/if}
        </article>{/each}
    </div>{/if}{#if form?.message}<p role="alert">
      {form.message}
    </p>{/if}{#if form?.completed}<p role="status">
      Atendimento marcado como concluído.
    </p>{/if}
</main>

<style>
  :global(body) {
    margin: 0;
    background: #f1efe8;
    color: #24372e;
    font-family: Arial, sans-serif;
  }
  main {
    max-width: 760px;
    margin: auto;
    padding: 32px 22px;
  }
  header {
    display: flex;
    justify-content: space-between;
    border-bottom: 1px solid #c8c8bd;
    padding-bottom: 16px;
    font-size: 12px;
    letter-spacing: 0.15em;
  }
  header a {
    color: #24372e;
    text-decoration: none;
  }
  h1 {
    font:
      400 clamp(36px, 7vw, 60px) Georgia,
      serif;
    margin: 38px 0 8px;
  }
  .intro {
    color: #626a62;
    line-height: 1.5;
  }
  .list {
    display: grid;
    gap: 12px;
    margin-top: 28px;
  }
  article {
    background: #fffefa;
    border: 1px solid #d9d7cc;
    padding: 20px;
    display: grid;
    gap: 8px;
  }
  article p {
    font:
      22px Georgia,
      serif;
    margin: 0;
  }
  article small {
    color: #73776f;
    text-transform: capitalize;
  }
  button {
    justify-self: start;
    background: #24372e;
    color: white;
    border: 0;
    padding: 12px 15px;
    cursor: pointer;
    margin-top: 8px;
  }
</style>
