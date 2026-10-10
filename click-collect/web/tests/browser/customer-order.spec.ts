import { expect, test } from "@playwright/test";

test("Should reserve privately, simulate locally, and complete one pickup", async ({
  browser,
}) => {
  const context = await browser.newContext();
  const customer = await context.newPage();
  await customer.goto("/");
  await expect(
    customer.getByRole("heading", { name: "Hoje no balcão" }),
  ).toBeVisible();
  await expect(
    customer.getByText(
      "Demonstração local. Nenhum pagamento real é processado.",
    ),
  ).toBeVisible();
  await customer.getByRole("button", { name: "Reservar uma fatia" }).click();
  await expect(customer).toHaveURL(/\/orders\/[a-f0-9]{32}$/);
  await expect(
    customer.getByRole("heading", { name: /está com a gente/ }),
  ).toBeVisible();
  const orderPath = new URL(customer.url()).pathname;
  const orderID = orderPath.split("/").at(-1);
  expect(orderID).toMatch(/^[a-f0-9]{32}$/);
  const cookies = await context.cookies();
  const scopedCookies = cookies.filter((cookie) => cookie.path === orderPath);
  expect(scopedCookies).toHaveLength(2);
  expect(scopedCookies.every((cookie) => cookie.httpOnly)).toBe(true);
  expect(scopedCookies.every((cookie) => cookie.sameSite === "Strict")).toBe(
    true,
  );

  const anonymousContext = await browser.newContext();
  const anonymous = await anonymousContext.newPage();
  await anonymous.goto(orderPath);
  await expect(
    anonymous.getByRole("heading", { name: "Não encontramos este pedido." }),
  ).toBeVisible();
  await anonymousContext.close();

  await customer
    .getByRole("button", { name: "Simular confirmação local" })
    .click();
  await expect(customer.getByText("Pagamento")).toBeVisible();
  let paymentConfirmed = false;
  for (let attempt = 0; attempt < 20; attempt += 1) {
    await customer.getByRole("button", { name: "Atualizar status" }).click();
    if (await customer.getByText("A cozinha está preparando").isVisible()) {
      paymentConfirmed = true;
      break;
    }
    await customer.waitForTimeout(100);
  }
  expect(paymentConfirmed).toBe(true);

  const operator = await context.newPage();
  await operator.goto("/loja");
  await operator.getByRole("button", { name: "Marcar pronto" }).click();
  await expect(operator.getByText("Aguardando cliente")).toBeVisible();

  await customer.goto(orderPath);
  await customer.getByRole("button", { name: "Atualizar status" }).click();
  await expect(customer.getByText("Pronto para retirar")).toBeVisible();
  const pickupCode = (
    await customer.locator(".pickup-pass strong").innerText()
  ).replaceAll(" ", "");
  expect(pickupCode).toMatch(/^[A-F0-9]{8}$/);

  await operator.goto("/loja");
  const pickupInput = operator.getByLabel("Código do cliente");
  await pickupInput.fill(pickupCode);
  await expect(pickupInput).toHaveValue(pickupCode);
  const inputIsValid = await pickupInput.evaluate(
    (element) => element instanceof HTMLInputElement && element.checkValidity(),
  );
  expect(inputIsValid).toBe(true);
  await operator.getByRole("button", { name: "Confirmar retirada" }).click();
  await operator.goto("/loja");
  if (!(await operator.getByText("Entregue").isVisible())) {
    const alert = operator.getByRole("alert");
    const message = (await alert.isVisible())
      ? await alert.innerText()
      : (await operator.locator("body").innerText()).slice(-600);
    throw new Error(`A retirada não foi confirmada: ${message}`);
  }
  await expect(operator.getByText("Entregue")).toBeVisible();
  await operator.goto("/loja");
  await expect(operator.getByText("Entregue")).toBeVisible();
  await context.close();
});
