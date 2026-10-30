import { test, expect, chromium, type BrowserContext, type Worker } from "@playwright/test";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";
import { readFileSync } from "node:fs";

// MV3 extensions load only in a persistent context launched with the extension
// path. Native messaging (the gw-grab host) cannot run here — there is no
// registered host and the extension id is ephemeral — so these tests verify the
// real built artifacts: the extension loads, the bundled content script scrapes
// a real DOM, and the bundled side panel renders a proposal. The native-host
// round-trip is covered by the Go host tests + --from-capture.

const dir = dirname(fileURLToPath(import.meta.url));
const distPath = join(dir, "..", "..", "dist");
const FIXTURES = "http://localhost:5173";

let context: BrowserContext;

test.beforeAll(async () => {
  context = await chromium.launchPersistentContext("", {
    headless: true,
    args: [
      `--headless=new`,
      `--disable-extensions-except=${distPath}`,
      `--load-extension=${distPath}`,
    ],
  });
});

test.afterAll(async () => {
  await context?.close();
});

async function serviceWorker(): Promise<Worker> {
  let [sw] = context.serviceWorkers();
  if (!sw) sw = await context.waitForEvent("serviceworker");
  return sw;
}

test("extension loads and registers a service worker", async () => {
  const sw = await serviceWorker();
  expect(sw.url()).toContain("background.js");
  // The id is the host of the worker URL.
  const id = new URL(sw.url()).host;
  expect(id).toMatch(/^[a-p]{32}$/);
});

test("bundled content script scrapes a GitHub PR DOM", async () => {
  const page = await context.newPage();
  await page.goto(`${FIXTURES}/github-pr.html`);

  // Inject the actual built content script; it stashes the result on a global.
  const code = readFileSync(join(distPath, "content.js"), "utf8");
  const capture = await page.evaluate((src) => {
    // eslint-disable-next-line no-eval
    (0, eval)(src);
    return (globalThis as Record<string, unknown>).__groveGrabResult;
  }, code);

  expect(capture).toMatchObject({
    title: "Add cool thing",
    branchHint: "feat/cool-thing",
  });
  expect((capture as { body: string }).body).toContain("cool thing");
  await page.close();
});

test("bundled content script scrapes a Notion page DOM", async () => {
  const page = await context.newPage();
  await page.goto(`${FIXTURES}/notion.html`);
  const code = readFileSync(join(distPath, "content.js"), "utf8");
  const capture = await page.evaluate((src) => {
    (0, eval)(src);
    return (globalThis as Record<string, unknown>).__groveGrabResult;
  }, code);
  expect((capture as { title: string }).title).toBe("Improve the data API");
  expect((capture as { body: string }).body).toContain("pagination");
  await page.close();
});

test("side panel renders a proposal and reflects edits", async () => {
  const sw = await serviceWorker();
  const id = new URL(sw.url()).host;
  const page = await context.newPage();

  // Stub chrome.runtime messaging BEFORE the panel script runs so it receives a
  // proposal without a live native host.
  await page.addInitScript(() => {
    const proposal = {
      provider: "github",
      repo: "widget",
      repoDisplay: "acme/widget",
      branch: "feat/cool-thing",
      name: "feat-cool-thing",
      track: true,
      sourceUrl: "https://github.com/acme/widget/pull/7",
      sourceRef: "7",
      sourceTitle: "Add cool thing",
      candidates: ["acme/widget", "acme/api"],
      notes: ["repo from memory of past grabs"],
    };
    // @ts-expect-error test shim
    window.chrome = {
      runtime: {
        sendMessage: (msg: { type: string }) => {
          if (msg.type === "grab:getState") {
            return Promise.resolve({ state: "proposal", proposal, capture: {} });
          }
          if (msg.type === "grab:create") return Promise.resolve({ ok: true });
          return Promise.resolve(undefined);
        },
        onMessage: { addListener: () => {} },
      },
    };
  });

  await page.goto(`chrome-extension://${id}/sidepanel.html`);

  await expect(page.locator("#sourceTitle")).toHaveText("Add cool thing");
  await expect(page.locator("#branch")).toHaveValue("feat/cool-thing");
  await expect(page.locator("#name")).toHaveValue("feat-cool-thing");
  await expect(page.locator("#trackTag")).toBeVisible();
  // Repo dropdown has both candidates.
  await expect(page.locator("#repo option")).toHaveCount(2);

  // Edit the branch, click create, see the success message (host stubbed ok).
  await page.fill("#branch", "fix/edited");
  await page.click("#create");
  await expect(page.locator("#result")).toContainText("Workspace created");
  await page.close();
});
