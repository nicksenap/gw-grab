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
    channel: "chromium",
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

test("side panel lists captures and dispatches a job", async () => {
  const sw = await serviceWorker();
  const id = new URL(sw.url()).host;
  const page = await context.newPage();

  // Stub chrome.* BEFORE the panel script runs so it works without a live host.
  await page.addInitScript(() => {
    const state = {
      items: [
        { url: "https://x.slack.com/archives/C1/p1", title: "Data chat slow", body: "", branchHint: "", selection: "p99 12s" },
        { url: "https://grafana.example/d/abc", title: "Latency", body: "", branchHint: "", selection: "" },
      ],
      proposal: { name: "data-chat-slow", branch: "data-chat-slow", repo: "", track: false, sourceUrl: "https://x.slack.com/archives/C1/p1" },
      status: "idle",
      message: "",
    };
    const w = window as unknown as Record<string, unknown>;
    w.__sent = [];
    w.chrome = {
      storage: { local: { get: () => Promise.resolve({}), set: () => Promise.resolve() } },
      runtime: {
        sendMessage: (msg: { type: string }) => {
          (w.__sent as unknown[]).push(msg);
          if (msg.type === "grab:getState") return Promise.resolve(state);
          if (msg.type === "grab:options")
            return Promise.resolve({
              presets: { "funnel-ai-full": ["saturn", "conversational-analytics"] },
              repos: ["saturn", "conversational-analytics"],
              agentCmd: "pi --no-skills --skill-profile pocock",
            });
          return Promise.resolve({ ok: true });
        },
        onMessage: { addListener: () => {} },
      },
    };
  });

  await page.goto(`chrome-extension://${id}/sidepanel.html`);

  await expect(page.locator("#items li")).toHaveCount(2);
  await expect(page.locator("#items li").first()).toContainText("p99 12s");
  await expect(page.locator("#target")).toHaveValue("preset:funnel-ai-full");
  await expect(page.locator("#name")).toHaveValue("data-chat-slow");
  await expect(page.locator("#startIn option")).toHaveCount(3);
  await expect(page.locator("#agentCmd")).toHaveValue("pi --no-skills --skill-profile pocock");

  await page.selectOption("#startIn", "conversational-analytics");
  await page.fill("#prompt", "try this out");
  await page.press("#prompt", "Meta+Enter");

  const job = await page.waitForFunction(() =>
    ((window as unknown as { __sent: Array<{ type: string; job?: unknown }> }).__sent).find((m) => m.type === "grab:dispatch")?.job,
  );
  expect(await job.jsonValue()).toMatchObject({
    name: "data-chat-slow",
    preset: "funnel-ai-full",
    startIn: "conversational-analytics",
    prompt: "try this out",
    agentCmd: "pi --no-skills --skill-profile pocock",
  });
  await page.close();
});
