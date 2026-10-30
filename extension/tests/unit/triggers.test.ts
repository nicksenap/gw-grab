import { afterEach, describe, expect, it, vi } from "vitest";

// Captures the listeners registerTriggers() installs so we can fire them and
// cover the context-menu / command / message-broker wiring.
type Msg = (msg: unknown, sender: unknown, send: (r: unknown) => void) => boolean | void;

function fakeChrome() {
  const L = {
    installed: [] as Array<() => void>,
    ctx: [] as Array<(info: Record<string, unknown>, tab?: chrome.tabs.Tab) => void>,
    command: [] as Array<(cmd: string, tab?: chrome.tabs.Tab) => void>,
    message: [] as Msg[],
  };
  const sent: Array<{ action: string; capture?: { url: string } }> = [];
  const chrome = {
    runtime: {
      id: undefined as string | undefined,
      connectNative: vi.fn(() => {
        let cb: (m: unknown) => void = () => {};
        return {
          onMessage: { addListener: (f: (m: unknown) => void) => (cb = f) },
          onDisconnect: { addListener: () => {} },
          postMessage: vi.fn((req: { action: string }) => {
            sent.push(req);
            queueMicrotask(() => cb({ ok: true, presets: {}, repos: [] }));
          }),
          disconnect: vi.fn(),
        };
      }),
      sendMessage: vi.fn(() => Promise.resolve()),
      onInstalled: { addListener: (cb: () => void) => L.installed.push(cb) },
      onMessage: { addListener: (cb: Msg) => L.message.push(cb) },
      lastError: undefined,
    },
    contextMenus: { create: vi.fn(), onClicked: { addListener: (cb: never) => L.ctx.push(cb) } },
    commands: { onCommand: { addListener: (cb: never) => L.command.push(cb) } },
    sidePanel: { open: vi.fn(() => Promise.resolve()), setPanelBehavior: vi.fn(() => Promise.resolve()) },
    tabs: { query: vi.fn((_q: unknown, cb: (t: unknown[]) => void) => cb([{ id: 9, windowId: 2 }])) },
    scripting: {
      executeScript: vi.fn(() =>
        Promise.resolve([{ result: { url: "https://page", title: "P", body: "", branchHint: "", selection: "" } }]),
      ),
    },
  };
  (globalThis as Record<string, unknown>).chrome = chrome;
  return { L, chrome, sent };
}

const flush = () => new Promise((r) => setTimeout(r, 0));

afterEach(() => {
  vi.resetModules();
  delete (globalThis as Record<string, unknown>).chrome;
});

async function setup() {
  const f = fakeChrome();
  const bg = await import("../../src/background.js");
  bg.registerTriggers();
  return { ...f, bg };
}

describe("registerTriggers", () => {
  it("creates the context menu and panel behavior on install", async () => {
    const { L, chrome } = await setup();
    L.installed[0]();
    expect(chrome.contextMenus.create).toHaveBeenCalledWith(
      expect.objectContaining({ id: "add", contexts: ["page", "selection", "link"] }),
    );
    expect(chrome.sidePanel.setPanelBehavior).toHaveBeenCalledWith({ openPanelOnActionClick: true });
  });

  it("context menu on a link adds the link without scraping", async () => {
    const { L, chrome, bg } = await setup();
    L.ctx[0]({ menuItemId: "add", linkUrl: "https://grafana/d/1", selectionText: "Latency" }, { id: 1, windowId: 2 } as chrome.tabs.Tab);
    await flush();
    expect(chrome.sidePanel.open).toHaveBeenCalledWith({ windowId: 2 });
    expect(chrome.scripting.executeScript).not.toHaveBeenCalled();
    expect(bg.getState().items[0]).toMatchObject({ url: "https://grafana/d/1", title: "Latency" });
  });

  it("context menu on a page scrapes it; other menu ids are ignored", async () => {
    const { L, chrome, bg } = await setup();
    L.ctx[0]({ menuItemId: "other" }, { id: 1 } as chrome.tabs.Tab);
    expect(chrome.scripting.executeScript).not.toHaveBeenCalled();
    L.ctx[0]({ menuItemId: "add" }, { id: 1, windowId: 2 } as chrome.tabs.Tab);
    await flush();
    await flush();
    expect(bg.getState().items[0].url).toBe("https://page");
  });

  it("keyboard command adds the given or active tab", async () => {
    const { L, chrome, bg } = await setup();
    L.command[0]("nope");
    L.command[0]("grab");
    await flush();
    await flush();
    expect(chrome.tabs.query).toHaveBeenCalled();
    expect(bg.getState().items.length).toBe(1);
  });

  it("brokers panel messages", async () => {
    const { L, sent } = await setup();
    const call = (msg: unknown) =>
      new Promise((res) => {
        expect(L.message[0](msg, {}, res)).toBe(true);
      });
    expect(L.message[0]({ type: "unknown" }, {}, () => {})).toBe(false);

    await call({ type: "grab:addActive" });
    expect(((await call({ type: "grab:getState" })) as { items: unknown[] }).items.length).toBe(1);
    await call({ type: "grab:remove", index: 0 });
    await call({ type: "grab:clear" });
    expect(await call({ type: "grab:options" })).toMatchObject({ presets: {}, repos: [] });
    await call({ type: "grab:dispatch", job: { name: "x" } });
    expect(sent.map((s) => s.action)).toContain("dispatch");
  });
});
