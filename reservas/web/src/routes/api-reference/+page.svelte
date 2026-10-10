<script lang="ts">
  import { resolve } from "$app/paths";
  import { onMount } from "svelte";
  import "@scalar/api-reference/style.css";

  onMount(() => {
    let active = true;

    async function mountReference(): Promise<void> {
      const { createApiReference } = await import("@scalar/api-reference");
      if (active) {
        createApiReference("#scalar-api-reference", {
          url: resolve("/openapi.yaml"),
          theme: "default",
          showSidebar: true,
        });
      }
    }

    void mountReference();
    return () => {
      active = false;
    };
  });
</script>

<svelte:head>
  <title>API de Reservas — Scalar</title>
  <meta name="description" content="Contrato OpenAPI da demo de Reservas." />
</svelte:head>

<main>
  <header>
    <a href={resolve("/")}>← Voltar à agenda</a>
    <span>CONTRATO OPENAPI · REFERÊNCIA SCALAR</span>
  </header>
  <div id="scalar-api-reference"></div>
</main>

<style>
  main {
    min-height: 100vh;
    background: #f1efe8;
  }

  header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 16px;
    padding: 18px clamp(18px, 5vw, 64px);
    color: #24372e;
    font:
      11px Arial,
      sans-serif;
    letter-spacing: 0.12em;
  }

  a {
    color: inherit;
    text-decoration: none;
    letter-spacing: 0;
  }

  #scalar-api-reference {
    min-height: calc(100vh - 58px);
  }

  @media (max-width: 560px) {
    header {
      align-items: flex-start;
      flex-direction: column;
    }
  }
</style>
