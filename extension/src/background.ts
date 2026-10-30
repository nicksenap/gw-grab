// background.ts — service worker. Owns the triggers (context menu, keyboard
// command, panel buttons), collects captures into a dispatch list, brokers
// messages with the native host, and broadcasts state to the side panel.
import type { Capture, DispatchState, HostRequest, HostResponse, Job, Options } from "./types.js";

export const HOST = "io.grove.gw_grab";
const STORE_KEY = "dispatchState";

export function emptyState(): DispatchState {
  return { items: [], proposal: null, status: "idle", message: "" };
}

let state: DispatchState = emptyState();
let loaded: Promise<void> | null = null;

// --- native messaging (isolated so it can be unit-tested with a fake chrome) ---

/**
 * sendNative opens a port, posts one request and resolves with the first
 * non-progress reply. `{progress}` messages (streamed by long actions such as
 * dispatch) are passed to onProgress.
 */
export function sendNative(message: HostRequest, onProgress?: (msg: string) => void): Promise<HostResponse> {
  return new Promise((resolve) => {
    let port: chrome.runtime.Port;
    try {
      port = chrome.runtime.connectNative(HOST);
    } catch (e) {
      resolve({ error: "native host not reachable: " + (e as Error).message });
      return;
    }
    let done = false;
    port.onMessage.addListener((msg: HostResponse) => {
      if (msg?.progress && !msg.ok && !msg.error) {
        onProgress?.(msg.progress);
        return;
      }
      done = true;
      resolve(msg);
      port.disconnect();
    });
    port.onDisconnect.addListener(() => {
      if (!done) {
        const err = chrome.runtime.lastError;
        resolve({
          error: err?.message ?? "native host disconnected (run `gw grab install-host <id>`?)",
        });
      }
    });
    port.postMessage(message);
  });
}

// --- state (persisted to storage.session so a worker restart keeps the list) ---

async function load(): Promise<void> {
  loaded ??= (async () => {
    try {
      const got = await chrome.storage?.session?.get(STORE_KEY);
      const saved = got?.[STORE_KEY] as DispatchState | undefined;
      if (saved?.items) state = { ...saved, status: saved.status === "dispatching" ? "idle" : saved.status };
    } catch {
      // storage unavailable (tests) — keep in-memory state
    }
  })();
  return loaded;
}

export function getState(): DispatchState {
  return state;
}

function setState(next: Partial<DispatchState>): void {
  state = { ...state, ...next };
  void chrome.storage?.session?.set({ [STORE_KEY]: state }).catch(() => {});
  void chrome.runtime.sendMessage({ type: "grab:update", payload: state }).catch(() => {});
}

// --- capture ---

/** captureFromClick builds a Capture for a right-clicked link without scraping. */
export function captureFromLink(linkUrl: string, linkText = ""): Capture {
  return { url: linkUrl, title: linkText.trim() || linkUrl, body: "", branchHint: "", selection: "" };
}

async function scrapeTab(tabId: number): Promise<Capture> {
  const [{ result }] = await chrome.scripting.executeScript({ target: { tabId }, files: ["content.js"] });
  if (!result) throw new Error("empty capture");
  return result as Capture;
}

function openPanel(tab?: chrome.tabs.Tab): void {
  // Must run synchronously inside the user gesture, before any await.
  if (tab?.windowId !== undefined) {
    chrome.sidePanel?.open({ windowId: tab.windowId }).catch(() => {});
  }
}

/** addCapture appends a capture; the first one is resolved to prefill the form. */
export async function addCapture(c: Capture): Promise<void> {
  await load();
  const items = [...state.items, c];
  setState({ items, status: "idle", message: "", result: undefined });
  if (items.length === 1 || !state.proposal) {
    const resp = await sendNative({ action: "resolve", capture: c });
    if (resp?.proposal) setState({ proposal: resp.proposal });
  }
}

/** addTab scrapes a tab and adds it to the list. */
export async function addTab(tab: chrome.tabs.Tab): Promise<void> {
  if (!tab?.id) return;
  await load();
  setState({ status: "capturing", message: "Reading the page…" });
  let c: Capture;
  try {
    c = await scrapeTab(tab.id);
  } catch (e) {
    setState({ status: "error", message: "Could not read the page: " + (e as Error).message });
    return;
  }
  await addCapture(c);
}

export async function removeItem(index: number): Promise<void> {
  await load();
  const items = state.items.filter((_, i) => i !== index);
  setState({ items, proposal: items.length ? state.proposal : null });
}

export async function clear(): Promise<void> {
  await load();
  setState(emptyState());
}

// --- options + dispatch ---

let optionsCache: Options | null = null;

export async function options(): Promise<Options | { error: string }> {
  if (optionsCache) return optionsCache;
  const resp = await sendNative({ action: "options" });
  if (!resp?.ok) return { error: resp?.error ?? "no options from host" };
  optionsCache = {
    presets: resp.presets ?? {},
    repos: (resp.repos ?? []).map((r) => r.name),
    agentCmd: resp.agentCmd ?? "",
  };
  return optionsCache;
}

export async function dispatch(job: Job): Promise<HostResponse> {
  await load();
  setState({ status: "dispatching", message: "Sending to host…", result: undefined });
  const resp = await sendNative({ action: "dispatch", job }, (m) => setState({ message: m }));
  if (resp?.ok && resp.result) {
    const r = resp.result;
    setState({ ...emptyState(), status: "done", message: `Dispatched → ${r.workspace} (Herdr ${r.pane})`, result: r });
  } else {
    setState({ status: "error", message: resp?.error ?? "dispatch failed" });
  }
  return resp;
}

// --- wiring (skipped under test where `chrome` lacks these APIs) ---

export function registerTriggers(): void {
  chrome.runtime.onInstalled.addListener(() => {
    chrome.contextMenus.create({
      id: "add",
      title: "Add to Grove dispatch",
      contexts: ["page", "selection", "link"],
    });
    chrome.sidePanel.setPanelBehavior({ openPanelOnActionClick: true }).catch(() => {});
  });

  chrome.contextMenus.onClicked.addListener((info, tab) => {
    if (info.menuItemId !== "add") return;
    openPanel(tab);
    if (info.linkUrl) void addCapture(captureFromLink(info.linkUrl, info.selectionText));
    else if (tab) void addTab(tab);
  });

  chrome.commands.onCommand.addListener((cmd, tab) => {
    if (cmd !== "grab") return;
    openPanel(tab);
    if (tab) void addTab(tab);
    else chrome.tabs.query({ active: true, currentWindow: true }, ([t]) => t && void addTab(t));
  });

  chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
    const reply = (p: Promise<unknown>) => {
      p.then(sendResponse);
      return true; // async
    };
    switch (msg?.type) {
      case "grab:getState":
        return reply(load().then(getState));
      case "grab:addActive":
        return reply(
          new Promise<void>((res) =>
            chrome.tabs.query({ active: true, lastFocusedWindow: true }, ([t]) => res(t ? addTab(t) : undefined)),
          ),
        );
      case "grab:remove":
        return reply(removeItem(msg.index as number));
      case "grab:clear":
        return reply(clear());
      case "grab:options":
        return reply(options());
      case "grab:dispatch":
        return reply(dispatch(msg.job as Job));
    }
    return false;
  });
}

// Only auto-register when running as the actual service worker (chrome.runtime
// with an extension id present), not when imported by a unit test.
if (typeof chrome !== "undefined" && chrome.runtime?.id) {
  registerTriggers();
}
