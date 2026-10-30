import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// A controllable fake of chrome.runtime.connectNative ports.
interface FakePort {
  onMessage: { addListener: (cb: (m: unknown) => void) => void };
  onDisconnect: { addListener: (cb: () => void) => void };
  postMessage: (m: unknown) => void;
  disconnect: () => void;
  _emit: (m: unknown) => void;
  _disconnect: () => void;
}

function makePort(): FakePort {
  let onMsg: (m: unknown) => void = () => {};
  let onDisc: () => void = () => {};
  return {
    onMessage: { addListener: (cb) => (onMsg = cb) },
    onDisconnect: { addListener: (cb) => (onDisc = cb) },
    postMessage: vi.fn(),
    disconnect: vi.fn(),
    _emit: (m) => onMsg(m),
    _disconnect: () => onDisc(),
  };
}

let port: FakePort;
let lastError: { message: string } | undefined;

beforeEach(() => {
  port = makePort();
  lastError = undefined;
  (globalThis as Record<string, unknown>).chrome = {
    runtime: {
      // No id → registerTriggers() is skipped on import.
      get lastError() {
        return lastError;
      },
      connectNative: vi.fn(() => port),
    },
  };
});

afterEach(() => {
  vi.resetModules();
  delete (globalThis as Record<string, unknown>).chrome;
});

async function importBg() {
  return await import("../../src/background.js");
}

describe("sendNative", () => {
  it("resolves with the host message on success", async () => {
    const { sendNative } = await importBg();
    const p = sendNative({ action: "repos" });
    port._emit({ ok: true, repos: [{ name: "api", path: "/r", remote: "", display_name: "a/api" }] });
    const resp = await p;
    expect(resp.ok).toBe(true);
    expect(resp.repos?.[0].name).toBe("api");
    expect(port.postMessage).toHaveBeenCalledWith({ action: "repos" });
    expect(port.disconnect).toHaveBeenCalled();
  });

  it("resolves with an error when the port disconnects before replying", async () => {
    const { sendNative } = await importBg();
    lastError = { message: "host not found" };
    const p = sendNative({ action: "repos" });
    port._disconnect();
    const resp = await p;
    expect(resp.error).toContain("host not found");
  });

  it("gives a setup hint when disconnect has no lastError", async () => {
    const { sendNative } = await importBg();
    const p = sendNative({ action: "repos" });
    port._disconnect();
    const resp = await p;
    expect(resp.error).toContain("install-host");
  });

  it("handles connectNative throwing", async () => {
    (globalThis as Record<string, unknown>).chrome = {
      runtime: {
        connectNative: () => {
          throw new Error("no native messaging");
        },
      },
    };
    const { sendNative } = await importBg();
    const resp = await sendNative({ action: "repos" });
    expect(resp.error).toContain("not reachable");
  });
});
