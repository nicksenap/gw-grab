import { afterEach, describe, expect, it, vi } from "vitest";
import type { Capture, DispatchState, HostResponse, Job, Proposal } from "../../src/types.js";

// Drives the background worker's list + dispatch flow with a fully faked chrome:
// each connectNative() call answers from `replies` by request action.

const page: Capture = { url: "https://x.slack.com/archives/C1/p1", title: "Data chat slow", body: "", branchHint: "", selection: "" };
const proposal = { name: "data-chat-slow", branch: "data-chat-slow", repo: "", track: false } as Proposal;

let updates: DispatchState[];
let sent: unknown[];
let stored: Record<string, unknown>;

function installChrome(replies: Record<string, HostResponse[]>, opts: { execThrows?: boolean; stored?: unknown } = {}) {
  updates = [];
  sent = [];
  stored = opts.stored ? { dispatchState: opts.stored } : {};
  (globalThis as Record<string, unknown>).chrome = {
    runtime: {
      connectNative: vi.fn(() => {
        let onMsg: (m: unknown) => void = () => {};
        return {
          onMessage: { addListener: (cb: (m: unknown) => void) => (onMsg = cb) },
          onDisconnect: { addListener: () => {} },
          postMessage: vi.fn((req: { action: string }) => {
            sent.push(req);
            for (const r of replies[req.action] ?? [{ error: "no reply" }]) queueMicrotask(() => onMsg(r));
          }),
          disconnect: vi.fn(),
        };
      }),
      sendMessage: vi.fn((msg: { type: string; payload: DispatchState }) => {
        if (msg.type === "grab:update") updates.push(msg.payload);
        return Promise.resolve();
      }),
    },
    storage: {
      session: {
        get: vi.fn(() => Promise.resolve(stored)),
        set: vi.fn((v: Record<string, unknown>) => {
          Object.assign(stored, v);
          return Promise.resolve();
        }),
      },
    },
    scripting: {
      executeScript: vi.fn(() =>
        opts.execThrows ? Promise.reject(new Error("no access")) : Promise.resolve([{ result: page }]),
      ),
    },
  };
}

const flush = () => new Promise((r) => setTimeout(r, 0));
const bg = () => import("../../src/background.js");

afterEach(() => {
  vi.resetModules();
  delete (globalThis as Record<string, unknown>).chrome;
});

describe("collecting captures", () => {
  it("adds a scraped tab and resolves the first item only", async () => {
    installChrome({ resolve: [{ ok: true, proposal }] });
    const { addTab, addCapture, captureFromLink, getState } = await bg();
    await addTab({ id: 1 } as chrome.tabs.Tab);
    await addCapture(captureFromLink("https://grafana/d/1", "Latency"));
    await flush();
    const s = getState();
    expect(s.items.map((i) => i.url)).toEqual([page.url, "https://grafana/d/1"]);
    expect(s.items[1].title).toBe("Latency");
    expect(s.proposal?.name).toBe("data-chat-slow");
    expect(sent.filter((r) => (r as { action: string }).action === "resolve").length).toBe(1);
    expect(updates.map((u) => u.status)).toContain("capturing");
    expect((stored.dispatchState as DispatchState).items.length).toBe(2);
  });

  it("uses the URL as title for a bare link", async () => {
    const { captureFromLink } = await (installChrome({}), bg());
    expect(captureFromLink("https://g/x").title).toBe("https://g/x");
  });

  it("reports unreadable pages", async () => {
    installChrome({}, { execThrows: true });
    const { addTab, getState } = await bg();
    await addTab({ id: 1 } as chrome.tabs.Tab);
    expect(getState()).toMatchObject({ status: "error", items: [] });
    expect(getState().message).toContain("Could not read the page");
  });

  it("ignores a tab with no id", async () => {
    installChrome({});
    const { addTab } = await bg();
    await addTab({} as chrome.tabs.Tab);
    expect(updates.length).toBe(0);
  });

  it("removes items and drops the proposal with the last one", async () => {
    installChrome({ resolve: [{ ok: true, proposal }] });
    const { addCapture, removeItem, clear, getState } = await bg();
    await addCapture(page);
    await addCapture({ ...page, url: "u2" });
    await removeItem(0);
    expect(getState().items.map((i) => i.url)).toEqual(["u2"]);
    expect(getState().proposal).not.toBeNull();
    await removeItem(0);
    expect(getState().proposal).toBeNull();
    await addCapture(page);
    await clear();
    expect(getState().items).toEqual([]);
  });

  it("restores the list from session storage after a worker restart", async () => {
    installChrome({}, { stored: { items: [page], proposal, status: "dispatching", message: "" } });
    const { removeItem, getState } = await bg();
    await removeItem(5); // any action triggers load()
    expect(getState().items).toEqual([page]);
    expect(getState().status).toBe("idle"); // an interrupted dispatch is not still running
  });
});

describe("options", () => {
  it("maps and caches host options", async () => {
    installChrome({
      options: [{ ok: true, presets: { p: ["a"] }, repos: [{ name: "a", path: "", remote: "", display_name: "" }], agentCmd: "pi" }],
    });
    const { options } = await bg();
    expect(await options()).toEqual({ presets: { p: ["a"] }, repos: ["a"], agentCmd: "pi" });
    await options();
    expect(sent.length).toBe(1);
  });

  it("surfaces host errors", async () => {
    installChrome({ options: [{ error: "gw not found" }] });
    const { options } = await bg();
    expect(await options()).toEqual({ error: "gw not found" });
  });
});

describe("dispatch", () => {
  const job = { name: "try-it", branch: "try-it", preset: "p", items: [page], prompt: "go" } as Job;

  it("streams progress, then clears the list on success", async () => {
    const result = { workspace: "try-it", path: "/w", herdr: "wX", pane: "wX:p1", brief: "/b" };
    installChrome({ resolve: [{ ok: true, proposal }], dispatch: [{ progress: "Creating workspace try-it…" }, { ok: true, result }] });
    const { addCapture, dispatch, getState } = await bg();
    await addCapture(page);
    const resp = await dispatch(job);
    expect(resp.ok).toBe(true);
    expect(updates.map((u) => u.message)).toContain("Creating workspace try-it…");
    expect(getState()).toMatchObject({ status: "done", items: [], proposal: null, result });
    expect(getState().message).toContain("wX:p1");
    expect(sent).toContainEqual({ action: "dispatch", job });
  });

  it("keeps the list on failure", async () => {
    installChrome({ resolve: [{ ok: true, proposal }], dispatch: [{ error: "gw create failed: exists" }] });
    const { addCapture, dispatch, getState } = await bg();
    await addCapture(page);
    await dispatch(job);
    expect(getState()).toMatchObject({ status: "error", message: "gw create failed: exists" });
    expect(getState().items.length).toBe(1);
  });
});
