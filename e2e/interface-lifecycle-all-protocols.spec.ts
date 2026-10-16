import { expect, test } from "@playwright/test";
import { mkdir } from "node:fs/promises";

test("uses the lifecycle header name field for every protocol workbench", async ({ page }) => {
  await page.addInitScript(() => localStorage.clear());
  await page.route("http://127.0.0.1:39217/**", (route) => route.abort());

  for (const protocol of ["grpc", "websocket", "sse", "tcp", "udp", "mqtt", "amqp", "kafka", "redis", "sql"]) {
    await page.goto(`/#workbench=${protocol}`);
    const activePanel = page.locator(".workbench-panel:not([hidden])");
    await expect(page.getByTestId(`workbench-${protocol}`).first()).toHaveAttribute("aria-current", "page");
    await expect(activePanel).toBeVisible();
    const lifecycleTabs = activePanel.locator(".interface-lifecycle-tabs");
    const nameInput = activePanel.locator(".interface-lifecycle-name-slot input[aria-label='接口名称']");

    await expect(lifecycleTabs).toBeVisible();
    await expect(nameInput, `${protocol} should render one interface-name input in the lifecycle header`).toHaveCount(1);
    await expect(lifecycleTabs.locator("button svg")).toHaveCount(0);

    const lastTabBox = await lifecycleTabs.getByRole("tab").last().boundingBox();
    const nameBox = await nameInput.boundingBox();
    expect(lastTabBox, `${protocol} should render lifecycle tabs`).not.toBeNull();
    expect(nameBox, `${protocol} should render the interface-name input`).not.toBeNull();
    expect(nameBox!.x, `${protocol} name should follow lifecycle tabs`).toBeGreaterThanOrEqual(lastTabBox!.x + lastTabBox!.width);

    await nameInput.fill(`${protocol} 接口`);
    await expect(nameInput).toHaveValue(`${protocol} 接口`);
  }

  await mkdir("output/playwright", { recursive: true });
  await page.screenshot({ path: "output/playwright/all-protocol-interface-name-header.png", fullPage: true });
});
