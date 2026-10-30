// background.ts — service worker. Owns the triggers (context menu, toolbar icon,
// keyboard command), scrapes the active tab, brokers messages with the native
// host, and broadcasts state to the side panel.
import type { Capture, GrabState, HostRequest, HostResponse, Proposal } from "./types.js";

export const HOST = "io.grove.gw_grab";

let lastState: GrabState = { state: "idle" };

// --- native messaging (isolated so it can be unit-tested with a fake chrome) ---

export function sendNative(message: HostRequest): Promise<HostResponse> {
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

function broadcast(): void {
  void chrome.runtime.sendMessage({ type: "grab:update", payload: lastState }).catch(() => {});
}

function setState(s: GrabState): void {
  lastState = s;
  broadcast();
}

// --- capture + resolve ---

export async function grab(tab: chrome.tabs.Tab): Promise<void> {
  if (!tab?.id) return;
  const tabId = tab.id;
  try {
    await chrome.sidePanel.open({ tabId });
  } catch {
    // Programmatic open can be disallowed; the user can open the panel manually.
  }
  setState({ state: "capturing" });

  let capture: Capture;
  try {
    const [{ result }] = await chrome.scripting.executeScript({
      target: { tabId },
      files: ["content.js"],
    });
    if (!result) throw new Error("empty capture");
    capture = result as Capture;
  } catch (e) {
    setState({ state: "error", error: "Could not read the page: " + (e as Error).message });
    return;
  }

  const resp = await sendNative({ action: "resolve", capture });
  if (!resp || resp.error || !resp.proposal) {
    setState({ state: "error", error: resp?.error ?? "no proposal from host", capture });
    return;
  }
  setState({ state: "proposal", proposal: resp.proposal, capture });
}

// --- wiring (skipped under test where `chrome` lacks these APIs) ---

export function registerTriggers(): void {
  chrome.runtime.onInstalled.addListener(() => {
    chrome.contextMenus.create({
      id: "grab",
      title: "Grab into Grove workspace",
      contexts: ["page", "selection", "link"],
    });
    chrome.sidePanel.setPanelBehavior({ openPanelOnActionClick: true }).catch(() => {});
  });

  chrome.contextMenus.onClicked.addListener((info, tab) => {
    if (info.menuItemId === "grab" && tab) void grab(tab);
  });

  chrome.action.onClicked.addListener((tab) => void grab(tab));

  chrome.commands.onCommand.addListener((cmd) => {
    if (cmd === "grab") {
      chrome.tabs.query({ active: true, currentWindow: true }, ([tab]) => {
        if (tab) void grab(tab);
      });
    }
  });

  chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
    if (msg?.type === "grab:getState") {
      sendResponse(lastState);
      return false;
    }
    if (msg?.type === "grab:create") {
      sendNative({ action: "create", plan: msg.plan as Proposal }).then(sendResponse);
      return true; // async
    }
    return false;
  });
}

// Only auto-register when running as the actual service worker (chrome.runtime
// with an extension id present), not when imported by a unit test.
if (typeof chrome !== "undefined" && chrome.runtime?.id) {
  registerTriggers();
}
