import { test, expect, type Page } from "@playwright/test";
import { start, stop, baseUrl } from "./evcc";
import { expectModalHidden, expectModalVisible, openMoreMenu } from "./utils";

test.use({ baseURL: baseUrl() });
test.describe.configure({ mode: "parallel" });

test.beforeEach(async () => {
  await start("solar-control.evcc.yaml");
});
test.afterEach(async () => {
  await stop();
});

const mode = (page: Page, name: string) => page.getByRole("radio", { name: `Mode: ${name}` });
const strategy = (page: Page, name: string) =>
  page.getByRole("radio", { name: `Strategy: ${name}` });
const param = (page: Page, name: string) => page.getByRole("spinbutton", { name, exact: true });

test.describe("solar control", async () => {
  test("menu", async ({ page }) => {
    await page.goto("/");
    await openMoreMenu(page);
    await page.getByRole("link", { name: "Solar control" }).click();

    await expect(page.getByRole("heading", { name: "Solar control", exact: true })).toBeVisible();
    await expect(mode(page, "Off")).toBeChecked();
    await expect(strategy(page, "Smart")).toBeChecked();
    await expect(page.getByTestId("solar-control-reason")).toHaveText("Solar control is off.");
  });

  test("mode and strategy", async ({ page }) => {
    await page.goto("/#/solar");

    await mode(page, "Shadow").click();
    await expect(mode(page, "Shadow")).toBeChecked();
    await expect(page.getByText("Charging is not controlled.")).toBeVisible();

    await strategy(page, "Simple").click();
    await expect(strategy(page, "Simple")).toBeChecked();

    await page.reload();
    await expect(mode(page, "Shadow")).toBeChecked();
    await expect(strategy(page, "Simple")).toBeChecked();
  });

  test("activate", async ({ page }) => {
    await page.goto("/#/solar");
    const modal = page.getByTestId("solar-control-activate-modal");

    await mode(page, "Active").click();
    await expectModalVisible(modal);
    await expect(modal).toContainText("Home Assistant");
    await modal.getByRole("button", { name: "Cancel" }).click();
    await expectModalHidden(modal);
    await expect(mode(page, "Off")).toBeChecked();

    await mode(page, "Active").click();
    await expectModalVisible(modal);
    await modal.getByRole("button", { name: "Activate" }).click();
    await expectModalHidden(modal);
    await expect(mode(page, "Active")).toBeChecked();

    await page.reload();
    await expect(mode(page, "Active")).toBeChecked();
  });

  test("parameter", async ({ page }) => {
    await page.goto("/#/solar");
    const params = page.getByTestId("solar-control-params");
    const minPower = param(page, "Minimum charging power");

    await expect(minPower).toHaveValue("1400");
    await expect(params).toContainText("equals 700 W");
    await minPower.fill("1500");
    await minPower.press("Tab");
    await expect(params.getByText("Saved.", { exact: true })).toBeVisible();
    await expect(params).toContainText("equals 750 W");

    await page.reload();
    await expect(minPower).toHaveValue("1500");
  });

  test("validation", async ({ page }) => {
    await page.goto("/#/solar");
    const socHigh = param(page, "High charge level");
    const minPower = param(page, "Minimum charging power");

    await socHigh.fill("10");
    await socHigh.press("Tab");
    await expect(page.getByText("Must be greater than the low charge level.")).toBeVisible();

    await minPower.fill("100");
    await minPower.press("Tab");
    await expect(page.getByText("Allowed range: 500 to 11,000.")).toBeVisible();

    await page.reload();
    await expect(socHigh).toHaveValue("75");
    await expect(minPower).toHaveValue("1400");
  });
});
