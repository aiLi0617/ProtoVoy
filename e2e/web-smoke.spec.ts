import { expect, test, type Page } from "@playwright/test";

const workbenchIds = ["http", "grpc", "websocket", "sse", "tcp", "udp", "mqtt", "amqp", "kafka", "redis", "sql", "mock", "gateway", "capture", "plugins", "ai"];

async function expectActiveWorkbench(page: Page, id: string) {
  await expect(page.getByTestId(`workbench-${id}`).first()).toHaveAttribute("aria-current", "page");
  const frame = page.locator(".workbench-panel:not([hidden]) .workbench-frame");
  await expect(frame).toBeVisible();
  await expect(frame.locator("h1")).toHaveCount(1);
}

test.beforeEach(async ({ page }) => {
  await page.route("http://127.0.0.1:39217/**", (route) => route.abort());
  await page.goto("/#workbench=http");
});

test("starts with zero tabs and the project overview", async ({ page }) => {
  await page.goto("/");
  await expect(page.locator(".workbench-tabs")).toHaveCount(0);
  await expect(page.locator(".workbench-tab")).toHaveCount(0);
  await expect(page.locator(".workbench-session-stack [role=tabpanel]")).toHaveCount(1);
  await expect(page.locator(".project-launcher")).toBeVisible();
  await expect(page.getByRole("heading", { name: "项目", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "新建项目" })).toBeVisible();
});

test("applies saved appearance before the app renders", async ({ page }) => {
  await page.addInitScript(() => {
    localStorage.setItem("protovoy:ui-state", JSON.stringify({ state: { themeMode: "light" }, version: 5 }));
    localStorage.setItem("protovoy:locale", "en-US");
    (window as Window & { __themeTransitions?: string[] }).__themeTransitions = [];
    new MutationObserver(() => {
      const theme = document.documentElement.dataset.theme;
      if (theme) (window as Window & { __themeTransitions?: string[] }).__themeTransitions?.push(theme);
    }).observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme"] });
  });
  await page.goto("/");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  await expect(page.locator("html")).toHaveAttribute("lang", "en-US");
  expect(await page.evaluate(() => (window as Window & { __themeTransitions?: string[] }).__themeTransitions)).not.toContain("dark");
});

test("turns Agent bootstrap failures into actionable guidance", async ({ page }) => {
  const explorerToggle = page.locator(".explorer-edge-toggle");
  if (await explorerToggle.isVisible()) await explorerToggle.click();
  const alert = page.getByRole("alert").filter({ hasText: "无法加载工作区" });
  await expect(alert).toContainText("无法连接 Local Agent");
  await expect(alert).not.toContainText("Failed to fetch");
  await expect(alert.getByRole("button", { name: "检查 Local Agent 设置" })).toBeVisible();
});

test("opens collection automation from the project rail", async ({ page }) => {
  await page.getByRole("button", { name: "自动化测试" }).click();
  await expect(page.locator(".collection-runner")).toBeVisible();
  await expect(page).toHaveURL(/view=automation/);
  await expect(page).toHaveURL(/workbench=runner/);
});
test("opens the grouped protocol workspace without horizontal overflow", async ({ page }) => {
  await expect(page).toHaveTitle("ProtoVoy");
  await expect(page.locator(".workbench-tabs")).toBeVisible();
  await expectActiveWorkbench(page, "http");
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  expect(overflow).toBeLessThanOrEqual(1);
  const separator = page.getByRole("separator", { name: /请求配置和响应检查器/ });
  await expect(separator).toBeVisible();
  const initialRatio = Number(await separator.getAttribute("aria-valuenow"));
  await separator.focus();
  await page.keyboard.press("ArrowRight");
  await expect(separator).toHaveAttribute("aria-valuenow", String(initialRatio + 5));
});

test("tab overflow tools expose every tab and allow closing the final tab", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 760 });
  const sparseGeometry = await page.locator(".workbench-tabs").evaluate((bar) => {
    const scroll = bar.querySelector(".workbench-tab-scroll")!.getBoundingClientRect();
    const tools = bar.querySelector(".workbench-tab-tools")!.getBoundingClientRect();
    const context = bar.querySelector(".workbench-context-actions")!.getBoundingClientRect();
    return { tabRight: scroll.right, toolsLeft: tools.left, toolsRight: tools.right, contextLeft: context.left };
  });
  expect(Math.abs(sparseGeometry.toolsLeft - sparseGeometry.tabRight)).toBeLessThanOrEqual(1);
  expect(sparseGeometry.contextLeft - sparseGeometry.toolsRight).toBeGreaterThan(1);

  await page.setViewportSize({ width: 720, height: 760 });
  for (const id of ["grpc", "redis", "mqtt", "websocket", "sql"]) {
    await page.evaluate((workbenchId) => window.dispatchEvent(new CustomEvent("protovoy-create-workbench", { detail: workbenchId })), id);
  }

  const tabs = page.locator(".workbench-tab-scroll");
  const tools = page.locator(".workbench-tab-tools");
  const context = page.locator(".workbench-context-actions");
  await expect(tabs.locator(".workbench-tab")).toHaveCount(6);
  const geometry = await page.locator(".workbench-tabs").evaluate((bar) => {
    const scroll = bar.querySelector(".workbench-tab-scroll")!;
    const tools = bar.querySelector(".workbench-tab-tools")!.getBoundingClientRect();
    return { scrollWidth: scroll.scrollWidth, clientWidth: scroll.clientWidth, toolsRight: tools.right };
  });
  expect(geometry.scrollWidth).toBeGreaterThan(geometry.clientWidth);
  expect(geometry.toolsRight).toBeLessThanOrEqual((await context.boundingBox())!.x + 1);
  await expect.poll(() => tabs.evaluate((element) => element.scrollLeft)).toBeGreaterThan(0);

  const more = page.getByRole("button", { name: "更多页签操作" });
  await more.dispatchEvent("mouseover");
  const menu = page.getByRole("menu", { name: "页签列表与操作" });
  await expect(menu).toBeVisible();
  await more.focus();
  await more.press("Escape");
  await expect(menu).toBeHidden();
  await more.evaluate((button) => (button as HTMLElement).blur());
  await more.dispatchEvent("mouseover");
  await expect(menu.getByRole("menuitem")).toHaveCount(9);
  const overflowGeometry = await more.evaluate((button) => {
    const wrapper = button.closest(".workbench-tabs-more")!;
    const content = button.closest(".workbench-content")!.getBoundingClientRect();
    const menuBounds = wrapper.querySelector(".workbench-tabs-menu")!.getBoundingClientRect();
    const trigger = wrapper.getBoundingClientRect();
    return { className: wrapper.className, content: { left: content.left, right: content.right }, trigger: { left: trigger.left, right: trigger.right }, menu: { left: menuBounds.left, right: menuBounds.right } };
  });
  expect(overflowGeometry.menu.left, JSON.stringify(overflowGeometry)).toBeGreaterThanOrEqual(overflowGeometry.content.left);
  expect(overflowGeometry.menu.right, JSON.stringify(overflowGeometry)).toBeLessThanOrEqual(overflowGeometry.content.right);
  await menu.getByRole("menuitem", { name: "关闭其他标签页" }).click();
  await expect(tabs.locator(".workbench-tab")).toHaveCount(1);

  await more.dispatchEvent("mouseover");
  await page.getByRole("menuitem", { name: "关闭当前标签页" }).click();
  await expect(tabs.locator(".workbench-tab")).toHaveCount(0);
  await expect(page.locator(".workbench-session-stack [role=tabpanel]")).toHaveCount(0);
  await expect(page.locator(".workbench-home")).toBeVisible();
  await expect(page.getByRole("button", { name: "新建", exact: true }).last()).toBeVisible();
  await more.dispatchEvent("mouseover");
  await expect(menu).toContainText("暂无打开的页签");
  const emptyMenuBox = await menu.boundingBox();
  expect(emptyMenuBox!.x).toBeGreaterThanOrEqual(0);
  expect(emptyMenuBox!.x + emptyMenuBox!.width).toBeLessThanOrEqual(720);
  await expect(menu.getByRole("menuitem", { name: "关闭全部标签页" })).toBeDisabled();

  await page.getByRole("button", { name: "新建", exact: true }).last().dispatchEvent("click");
  await expect(tabs.locator(".workbench-tab")).toHaveCount(1);
  await page.keyboard.press("Escape");
  await more.dispatchEvent("mouseover");
  await page.getByRole("menuitem", { name: "关闭全部标签页" }).dispatchEvent("click");
  await expect(tabs.locator(".workbench-tab")).toHaveCount(0);
  await expect(page.locator(".workbench-session-stack [role=tabpanel]")).toHaveCount(0);
  await expect(page.locator(".workbench-home")).toBeVisible();
});

test("gRPC response stays bottom-aligned and uses the HTTP split drag bounds", async ({ page }, testInfo) => {
  await page.evaluate(() => window.dispatchEvent(new CustomEvent("protovoy-select-workbench", { detail: "grpc" })));
  await expectActiveWorkbench(page, "grpc");

  const layout = page.locator(".grpc-workbench-layout");
  const response = page.locator(".grpc-response");
  const separator = page.getByRole("separator", { name: /gRPC 请求配置和响应检查器/ });
  const [layoutBox, responseBox, separatorBox] = await Promise.all([
    layout.boundingBox(),
    response.boundingBox(),
    separator.boundingBox(),
  ]);
  expect(layoutBox).not.toBeNull();
  expect(responseBox).not.toBeNull();
  expect(separatorBox).not.toBeNull();
  expect(Math.abs((layoutBox!.y + layoutBox!.height) - (responseBox!.y + responseBox!.height))).toBeLessThanOrEqual(1);

  const orientation = await separator.getAttribute("aria-orientation");
  if (testInfo.project.name === "desktop-chromium") {
    await page.mouse.move(separatorBox!.x + separatorBox!.width / 2, separatorBox!.y + separatorBox!.height / 2);
    await page.mouse.down();
    await page.mouse.move(orientation === "vertical" ? layoutBox!.x : separatorBox!.x + separatorBox!.width / 2, orientation === "horizontal" ? layoutBox!.y : separatorBox!.y + separatorBox!.height / 2, { steps: 4 });
    await page.mouse.up();
  } else {
    await separator.press("Home");
  }

  const requestSize = await page.locator('.grpc-workbench-split .split-pane > .split-panel[aria-label="gRPC 请求配置"]').evaluate((element, splitOrientation) => {
    const bounds = element.getBoundingClientRect();
    return splitOrientation === "vertical" ? bounds.width : bounds.height;
  }, orientation);
  expect(requestSize).toBeCloseTo(160, 0);
});

test("non-HTTP protocol response workbenches fill tall lifecycle panels", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 1200 });
  for (const id of ["grpc", "websocket", "sse", "tcp", "udp", "mqtt", "amqp", "kafka", "redis", "sql"]) {
    await page.evaluate((workbenchId) => window.dispatchEvent(new CustomEvent("protovoy-select-workbench", { detail: workbenchId })), id);
    await expectActiveWorkbench(page, id);
    const panel = page.locator(".workbench-panel:not([hidden])");
    const geometry = await panel.evaluate((element) => {
      const lifecycle = element.querySelector(".interface-lifecycle")?.getBoundingClientRect();
      const debug = element.querySelector(".interface-lifecycle-debug")?.getBoundingClientRect();
      return lifecycle && debug ? { lifecycleHeight: lifecycle.height, debugHeight: debug.height, lifecycleBottom: lifecycle.bottom, debugBottom: debug.bottom } : null;
    });
    expect(geometry, `${id} should render inside the lifecycle panel`).not.toBeNull();
    expect(Math.abs(geometry!.lifecycleBottom - geometry!.debugBottom), `${id} should reach the lifecycle bottom`).toBeLessThanOrEqual(1);
    expect(geometry!.debugHeight / geometry!.lifecycleHeight, `${id} should not collapse into the empty commandbar row`).toBeGreaterThan(.85);
  }
});

test("switches workbenches and preserves URL and stored state", async ({ page }) => {
  await page.evaluate(() => window.dispatchEvent(new CustomEvent("protovoy-select-workbench", { detail: "redis" })));
  await expectActiveWorkbench(page, "redis");
  await expect(page).toHaveURL(/#workbench=redis/);
  await expect.poll(() => page.evaluate(() => {
    const persisted = localStorage.getItem("protovoy:ui-state");
    if (!persisted) return null;
    try { return (JSON.parse(persisted) as { state?: { activeWorkbench?: string } }).state?.activeWorkbench ?? null; } catch { return null; }
  })).toBe("redis");
  await page.reload();
  await expectActiveWorkbench(page, "redis");
});

test("switches theme and keeps visible keyboard focus", async ({ page }) => {
  await page.getByTestId("theme-toggle").click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  await page.keyboard.press("Control+K");
  await expect(page.locator(".command-palette")).toBeVisible();
  await expect(page.locator(".command-input-wrap input")).toBeFocused();
});
test("shows shortcuts for the current platform", async ({ page }) => {
  const modifier = await page.evaluate(() => /mac|iphone|ipad|ipod/i.test(navigator.platform) ? "⌘" : "Ctrl");
  const commandTrigger = page.getByTestId("command-trigger");
  await expect(commandTrigger.locator("kbd")).toHaveText(`${modifier} K`);
  await expect(commandTrigger).toHaveAttribute("aria-label", new RegExp(`${modifier} K`));
});

test("uses one named lifecycle tablist", async ({ page }) => {
  await expect(page.getByRole("tablist", { name: "接口生命周期" })).toHaveCount(1);
});

test("moves project navigation below the workspace on narrow screens", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 760 });
  const navigation = page.locator(".project-module-nav");
  const main = page.locator("#protovoy-main");
  await expect(navigation).toBeVisible();
  const [navigationBox, mainBox] = await Promise.all([navigation.boundingBox(), main.boundingBox()]);
  expect(navigationBox).not.toBeNull();
  expect(mainBox).not.toBeNull();
  expect(navigationBox!.y).toBeGreaterThanOrEqual(mainBox!.y + mainBox!.height - 1);
  expect(mainBox!.width).toBeGreaterThanOrEqual(389);
});

test("keeps capture proxy instructions separated and copyable", async ({ page }) => {
  await page.evaluate(() => window.dispatchEvent(new CustomEvent("protovoy-select-workbench", { detail: "capture" })));
  const panel = page.locator(".capture-workbench:visible");
  await expect(panel.getByText("在目标应用中配置", { exact: true })).toBeVisible();
  await expect(panel.locator(".capture-proxy-instruction code")).toContainText("HTTP_PROXY=http://");
  await expect(panel.getByRole("button", { name: "复制代理地址" })).toBeVisible();
  await expect(panel.locator(".capture-control-actions .ui-status-badge")).toHaveText(/抓包中|已停止/);
});

test("keeps project settings separate from application settings", async ({ page }) => {
  await page.evaluate(() => window.dispatchEvent(new CustomEvent("protovoy-open-project-settings")));
  const projectSettings = page.locator(".project-settings-view");
  await expect(projectSettings).toContainText("通用设置");
  await expect(projectSettings).toContainText("项目资源");
  await projectSettings.getByRole("button", { name: "响应校验设置" }).click();
  await expect(projectSettings).toContainText("模块功能开关");
  await expect(projectSettings).toContainText("“接口运行”和“调试用例”里的校验响应");
  await expect(projectSettings).toContainText("Object 对象允许额外字段");
  const footerBox = await projectSettings.locator(".settings-dialog-footer").evaluate((element) => {
    const bounds = element.getBoundingClientRect();
    return { height: bounds.height, width: bounds.width };
  });
  expect(footerBox.height).toBeLessThan(80);
  expect(footerBox.width).toBeGreaterThan(footerBox.height * 4);

  await page.evaluate(() => window.dispatchEvent(new CustomEvent("protovoy-open-settings")));
  const applicationSettings = page.locator(".settings-dialog").filter({ hasText: /软件级|Application-wide/ });
  await expect(applicationSettings).toContainText(/软件级|Application-wide/);
  await expect(applicationSettings).toContainText(/不随项目切换|do not change with projects/);
  await expect(applicationSettings).not.toContainText("环境与变量");
});

test("HTTP target URL has an accessible label and neutral empty state", async ({ page }) => {
  const input = page.locator("#http-target-url");
  await expect(input).toHaveAttribute("aria-label");
  await expect(input).toHaveAttribute("placeholder");
  await expect(input).toHaveValue("");
  await expect(input).not.toHaveAttribute("aria-invalid", "true");
});

test("HTTP save action renders as one control with its dropdown arrow inside", async ({ page }) => {
  const control = page.locator(".workbench-panel:not([hidden]) .http-save-split");
  const save = control.locator(".http-save-button");
  const menuTrigger = control.locator(".http-save-menu-trigger");
  await expect(control).toBeVisible();
  await expect(save).toBeVisible();
  await expect(menuTrigger).toBeVisible();

  const geometry = await control.evaluate((element) => {
    const wrapper = element.getBoundingClientRect();
    const children = Array.from(element.querySelectorAll("button")).map((button) => {
      const bounds = button.getBoundingClientRect();
      const style = getComputedStyle(button);
      return { left: bounds.left, right: bounds.right, borderLeft: style.borderLeftWidth, borderRight: style.borderRightWidth };
    });
    return { left: wrapper.left, right: wrapper.right, border: getComputedStyle(element).borderLeftWidth, children };
  });

  expect(geometry.border).not.toBe("0px");
  expect(geometry.children).toHaveLength(2);
  expect(geometry.children.every((child) => child.left >= geometry.left && child.right <= geometry.right)).toBe(true);
  expect(geometry.children[0]).toMatchObject({ borderLeft: "0px", borderRight: "0px" });
  expect(geometry.children[1].borderLeft).toBe("1px");
  expect(geometry.children[1].borderRight).toBe("0px");
});

test("all protocol workbenches keep semantic frame and viewport bounds", async ({ page }) => {
  for (const id of workbenchIds) {
    await page.goto(`/#workbench=${id}`);
    await expectActiveWorkbench(page, id);
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    expect(overflow, `${id} horizontal overflow`).toBeLessThanOrEqual(1);
  }
});

test("all protocol workbenches keep light theme bounds", async ({ page }) => {
  await page.getByTestId("theme-toggle").click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  for (const id of workbenchIds) {
    await page.goto(`/#workbench=${id}`);
    await expectActiveWorkbench(page, id);
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    expect(overflow, `${id} light horizontal overflow`).toBeLessThanOrEqual(1);
  }
});

test("HTTP workbench exposes busy state semantics", async ({ page }) => {
  const frame = page.locator(".workbench-panel:not([hidden]) .workbench-frame");
  await expect(frame).not.toHaveAttribute("aria-busy", "true");
  await expect(frame).toHaveAttribute("aria-labelledby", /workbench-title-http/);
});

test("broker and database workbenches keep save action compact and request content visible", async ({ page }) => {
  for (const id of ["mqtt", "amqp", "kafka", "redis", "sql"]) {
    await page.evaluate((workbenchId) => window.dispatchEvent(new CustomEvent("protovoy-create-workbench", { detail: workbenchId })), id);
    await expectActiveWorkbench(page, id);
    const frame = page.locator(".workbench-panel:not([hidden]) .workbench-frame");
    const save = frame.getByRole("button", { name: "保存", exact: true });
    await expect(save).toBeVisible();
    expect((await save.boundingBox())?.height).toBeLessThanOrEqual(40);
    const content = frame.locator(".protocol-request-content");
    await expect(content).toBeVisible();
    expect((await content.boundingBox())?.height).toBeGreaterThan(80);
  }
});
