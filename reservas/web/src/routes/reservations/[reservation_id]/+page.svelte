<script lang="ts">
  import { resolve } from "$app/paths";
  import type { ActionData, PageData } from "./$types";
  let { data, form }: { data: PageData; form: ActionData } = $props();
</script>

<svelte:head><title>Sua reserva — Reserva</title></svelte:head>
<main>
  <a href={resolve("/")} class="back">← Voltar à agenda</a>
  <article>
    <p class="eyebrow">SUA RESERVA</p>
    <h1>{data.reservation.local_label}</h1>
    <p class="zone">{data.reservation.time_zone}</p>
    <div class="states">
      <div>
        <small>RESERVA</small><strong
          >{form?.reservation?.reservation_state ??
            data.reservation.reservation_state}</strong
        >
      </div>
      <div>
        <small>PAGAMENTO</small><strong
          >{form?.reservation?.payment_state ??
            data.reservation.payment_state}</strong
        >
      </div>
    </div>
    <p class="price">
      R$ {(data.reservation.amount_cents / 100).toFixed(2).replace(".", ",")}
    </p>
    {#if form?.message}<p role="alert">
        {form.message}
      </p>{/if}{#if (form?.reservation?.reservation_state ?? data.reservation.reservation_state) === "held"}<p
        class="notice"
      >
        Este horário está retido até {new Date(
          data.reservation.expires_at,
        ).toLocaleTimeString("pt-BR", {
          timeZone: data.reservation.time_zone,
          hour: "2-digit",
          minute: "2-digit",
        })}. Pagamento é apenas uma simulação local.
      </p>
      <div class="actions">
        <form method="POST" action="?/simulate">
          <button class="simulate">Simular confirmação local</button>
        </form>
        <form method="POST" action="?/cancel">
          <button class="cancel">Cancelar retenção</button>
        </form>
      </div>{:else}<form method="POST" action="?/refresh">
        <button class="cancel">Atualizar estado</button>
      </form>{/if}
    <p class="fine">
      Pagamento e reserva são estados separados. Nenhum Pix ou pagamento real
      foi criado.
    </p>
  </article>
</main>

<style>
  :global(body) {
    margin: 0;
    background: #f1efe8;
    color: #1f2c28;
    font-family: Arial, sans-serif;
  }
  main {
    max-width: 720px;
    margin: 0 auto;
    padding: 40px 22px;
  }
  .back {
    color: #43564b;
    text-decoration: none;
    font-size: 13px;
  }
  article {
    margin-top: 40px;
    background: #fffefa;
    padding: clamp(24px, 6vw, 48px);
    border: 1px solid #d9d7cc;
    box-shadow: 8px 8px 0 #dedbd1;
  }
  .eyebrow {
    font-size: 10px;
    letter-spacing: 0.18em;
    color: #9b5e43;
    font-weight: bold;
  }
  h1 {
    font:
      400 clamp(34px, 7vw, 58px) Georgia,
      serif;
    line-height: 1.1;
    margin: 16px 0;
  }
  .zone {
    color: #70776f;
  }
  .states {
    display: flex;
    gap: 36px;
    padding: 20px 0;
    border-top: 1px solid #e0dfd6;
    border-bottom: 1px solid #e0dfd6;
  }
  .states div {
    display: grid;
    gap: 8px;
  }
  .states small {
    font-size: 9px;
    letter-spacing: 0.16em;
    color: #72786f;
  }
  .states strong {
    text-transform: capitalize;
  }
  .price {
    font:
      24px Georgia,
      serif;
  }
  .notice {
    line-height: 1.6;
    background: #eff0e9;
    padding: 14px;
  }
  .cancel {
    background: #24372e;
    color: white;
    border: 0;
    padding: 12px 16px;
    cursor: pointer;
  }
  .fine {
    font-size: 12px;
    color: #757a72;
    margin-top: 24px;
  }
</style>
