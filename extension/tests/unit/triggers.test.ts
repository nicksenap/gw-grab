import { afterEach, describe, expect, it, vi } from "vitest";
import type { Proposal } from "../../src/types.js";

// Captures the listeners registerTriggers() installs so we can fire them and
// cover the context-menu / action / command / message-broker wiring.
interface Listeners {
  installed: Array<() => void>;
  ctxClicked: Array<(info: { menuItemId: string }, tab?: chrome.tabs.Tab) => void>;
  actionClicked: Array<(tab: chrome.tabs.Tab) => void>;
  command: Array<(cmd: string) => void>;
  message: Array<(msg: unknown, sender: unknown, send: (r: unknown) => void) => boolean | void>;
}

function fakeChrome() {
  const L: Listeners = { installed: [], ctxClicked: [], actionClicked: [], command: [], message: [] };
  const port = {
    onMessage: { addListener: (cb: (m: unknown) => void) => queueMicrotask(() => cb({ ok: true })) },
    onDisconnect: { addListener: () => {} },
    postMessage: vi.fn(),
    disconnect: vi.fn(),
  };
  const chrome = {
    runtime: {
      id: undefined as string | undefined, // keep import-time auto-register off
      connectNative: vi.fn(() => port),
      sendMessage: vi.fn(() => Promise.resolve()),
      onInstalled: { addListener: (cb: () => void) => L.installed.push(cb) },
      onMessage: { addListener: (cb: never) => L.message.push(cb) },
      lastError: undefined,
    },
    contextMenus: {
      create: vi.fn(),
      onClicked: { addListener: (cb: never) => L.ctxClicked.push(cb) },
    },
    action: { onClicked: { addListener: (cb: never) => L.actionClicked.push(cb) } },
    commands: { onCommand: { addListener: (cb: never) => L.command.push(cb) } },
    sidePanel: {
      open: vi.fn(() => Promise.resolve()),
      setPanelBehavior: vi.fn(() => Promise.resolve()),
    },
    scripting: {
      executeScript: vi.fn(() =>
        Promise.resolve([{ result: { url: "https://e.com", title: "t", body: "", branchHint: "", selection: "" } }]),
      ),
    },
    tabs: { query: vi.fn((_q: unknown, cb: (t: chrome.tabs.Tab[]) => void) => cb([{ id: 9 } as chrome.tabs.Tab])) },
  };
  return { chrome, L };
}

afterEach(() => {
  vi.resetModules();
  delete (globalThis as Record<string, unknown>).chrome;
});

describe("registerTriggers", () => {
  it("registers all listeners and wires the context menu on install", async () => {
    const { chrome, L } = fakeChrome();
    (globalThis as Record<string, unknown>).chrome = chrome;
    const { registerTriggers } = await import("../../src/background.js");
    registerTriggers();

    expect(L.installed.length).toBe(1);
    L.installed[0]();
    expect(chrome.contextMenus.create).toHaveBeenCalledWith(
      expect.objectContaining({ id: "grab", title: expect.stringContaining("Grove") }),
    );
    expect(chrome.sidePanel.setPanelBehavior).toHaveBeenCalled();
  });

  it("context-menu click on our item triggers a grab", async () => {
    const { chrome, L } = fakeChrome();
    (globalThis as Record<string, unknown>).chrome = chrome;
    const { registerTriggers } = await import("../../src/background.js");
    registerTriggers();

    L.ctxClicked[0]({ menuItemId: "grab" }, { id: 5 } as chrome.tabs.Tab);
    await new Promise((r) => setTimeout(r, 0));
    expect(chrome.scripting.executeScript).toHaveBeenCalled();
  });

  it("ignores context-menu clicks on other items", async () => {
    const { chrome, L } = fakeChrome();
    (globalThis as Record<string, unknown>).chrome = chrome;
    const { registerTriggers } = await import("../../src/background.js");
    registerTriggers();
    L.ctxClicked[0]({ menuItemId: "something-else" }, { id: 5 } as chrome.tabs.Tab);
    await new Promise((r) => setTimeout(r, 0));
    expect(chrome.scripting.executeScript).not.toHaveBeenCalled();
  });

  it("keyboard command queries the active tab and grabs it", async () => {
    const { chrome, L } = fakeChrome();
    (globalThis as Record<string, unknown>).chrome = chrome;
    const { registerTriggers } = await import("../../src/background.js");
    registerTriggers();
    L.command[0]("grab");
    await new Promise((r) => setTimeout(r, 0));
    expect(chrome.tabs.query).toHaveBeenCalled();
    expect(chrome.scripting.executeScript).toHaveBeenCalled();
  });

  it("getState message responds synchronously", async () => {
    const { chrome, L } = fakeChrome();
    (globalThis as Record<string, unknown>).chrome = chrome;
    const { registerTriggers } = await import("../../src/background.js");
    registerTriggers();
    const send = vi.fn();
    const keepOpen = L.message[0]({ type: "grab:getState" }, null, send);
    expect(send).toHaveBeenCalled();
    expect(keepOpen).toBe(false);
  });

  it("create message brokers to the native host and replies async", async () => {
    const { chrome, L } = fakeChrome();
    (globalThis as Record<string, unknown>).chrome = chrome;
    const { registerTriggers } = await import("../../src/background.js");
    registerTriggers();
    const send = vi.fn();
    const plan = { repo: "api", branch: "x" } as Proposal;
    const keepOpen = L.message[0]({ type: "grab:create", plan }, null, send);
    expect(keepOpen).toBe(true); // async channel held open
    await new Promise((r) => setTimeout(r, 0));
    expect(send).toHaveBeenCalledWith({ ok: true });
  });

  it("unknown message type is ignored", async () => {
    const { chrome, L } = fakeChrome();
    (globalThis as Record<string, unknown>).chrome = chrome;
    const { registerTriggers } = await import("../../src/background.js");
    registerTriggers();
    const send = vi.fn();
    const keepOpen = L.message[0]({ type: "whatever" }, null, send);
    expect(keepOpen).toBe(false);
  });
});
