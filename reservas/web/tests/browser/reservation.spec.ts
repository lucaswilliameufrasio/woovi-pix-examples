import { expect, test } from "@playwright/test";

function nextBusinessDate(today: string): string {
  const date = new Date(`${today}T00:00:00Z`);
  date.setUTCDate(date.getUTCDate() + 1);
  while (date.getUTCDay() === 0 || date.getUTCDay() === 6) {
    date.setUTCDate(date.getUTCDate() + 1);
  }
  return date.toISOString().slice(0, 10);
}

test("Should publish the canonical OpenAPI document through Scalar", async ({
  page,
}) => {
  const documentResponse = await page.request.get("/openapi.yaml");
  expect(documentResponse.ok()).toBe(true);
  expect(await documentResponse.text()).toContain("title: Reservas Demo API");

  await page.goto("/api-reference");
  await expect(
    page.getByText("Reservas Demo API", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Listar horários disponíveis" }),
  ).toBeVisible();
});

test("Should reserve privately and confirm only through the local payment event", async ({
  page,
}) => {
  await page.goto("/");
  const dateInput = page.getByLabel("Dia da visita");
  const selectedDate = nextBusinessDate(await dateInput.inputValue());
  await page.goto(`/?date=${selectedDate}`);
  await expect(page.getByLabel("Dia da visita")).toHaveValue(selectedDate);
  const slot = page.getByRole("button").filter({ hasText: /09:00/ }).first();
  await expect(slot).toBeVisible();
  await slot.click();

  await expect(page.getByText("SUA RESERVA")).toBeVisible();
  await expect(page.getByText("held")).toBeVisible();
  await expect(page.getByText("pending")).toBeVisible();
  await expect(page.getByText(/Nenhum Pix ou pagamento real/)).toBeVisible();

  await page.getByRole("button", { name: "Simular confirmação local" }).click();
  await page.goto("/operator");
  const reservation = page.locator("article").first();
  await expect
    .poll(async () => {
      const count = await page.locator("article").count();
      if (count === 0) {
        await page.reload();
      }
      return count;
    })
    .toBeGreaterThan(0);
  await expect
    .poll(async () => {
      await page.reload();
      return (await reservation.innerText()).toLowerCase();
    })
    .toContain("confirmed · pagamento paid");
});
