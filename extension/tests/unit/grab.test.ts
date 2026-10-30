import { afterEach, describe, expect, it, vi } from "vitest";
import type { Capture, Proposal } from "../../src/types.js";

// Drives background.grab() end-to-end with a fully faked chrome, capturing the
// state broadcasts so we can assert the capture→resolve→proposal flow.

interface FakePort {
  onMessage: { addListener: (cb: (m: unknown) => void) => void };
  onDisconnect: { addListener: (cb: () => void) => void };
  postMessage: (m: unknown) => void;
  disconnect: () => void;
  _emit: (m: unknown) => void;
}

function makePort(reply: unknown): FakePort {
  let onMsg: (m: unknown) => void = () => {};
  const p: FakePort = {
    onMessage: { addListener: (cb) => (onMsg = cb) },
    onDisconnect: { addListener: () => {} },
    postMessage: vi.fn(() => queueMicrotask(() => onMsg(reply))),
    disconnect: vi.fn(),
    _emit: (m) => onMsg(m),
  };
  return p;
}

const sampleCapture: Capture = {
  url: "https://github.com/acme/widget/pull/7",
  title: "Add cool thing",
  body: "",
  branchHint: "feat/cool-thing",
  selection: "",
};

const sampleProposal: Proposal = {
  provider: "github",
  repo: "widget",
  repoDisplay: "acme/widget",
  branch: "feat/cool-thing",
  name: "feat-cool-thing",
  track: true,
  sourceUrl: sampleCapture.url,
  sourceRef: "7",
  sourceTitle: "Add cool thing",
  candidates: ["acme/widget"],
  notes: [],
};

let updates: unknown[];

function installChrome(opts: { execResult?: unknown; resolveReply?: unknown; execThrows?: boolean }) {
  updates = [];
  (globalThis as Record<string, unknown>).chrome = {
    runtime: {
      connectNative: vi.fn(() => makePort(opts.resolveReply ?? { ok: true, proposal: sampleProposal })),
      sendMessage: vi.fn((msg: { type: string; payload: unknown }) => {
        if (msg.type === "grab:update") updates.push(msg.payload);
        return Promise.resolve();
      }),
    },
    sidePanel: { open: vi.fn(() => Promise.resolve()) },
    scripting: {
      executeScript: vi.fn(() => {
        if (opts.execThrows) return Promise.reject(new Error("no access"));
        return Promise.resolve([{ result: opts.execResult ?? sampleCapture }]);
      }),
    },
  };
}

afterEach(() => {
  vi.resetModules();
  delete (globalThis as Record<string, unknown>).chrome;
});

describe("grab", () => {
  it("captures the page and broadcasts a proposal", async () => {
    installChrome({});
    const { grab } = await import("../../src/background.js");
    await grab({ id: 1 } as chrome.tabs.Tab);
    // flush the microtask that delivers the native reply
    await new Promise((r) => setTimeout(r, 0));

    const states = updates.map((u) => (u as { state: string }).state);
    expect(states).toContain("capturing");
    const final = updates.at(-1) as { state: string; proposal?: Proposal };
    expect(final.state).toBe("proposal");
    expect(final.proposal?.repo).toBe("widget");
  });

  it("broadcasts an error when the page cannot be read", async () => {
    installChrome({ execThrows: true });
    const { grab } = await import("../../src/background.js");
    await grab({ id: 1 } as chrome.tabs.Tab);
    const final = updates.at(-1) as { state: string; error?: string };
    expect(final.state).toBe("error");
    expect(final.error).toContain("Could not read the page");
  });

  it("broadcasts an error when the host returns no proposal", async () => {
    installChrome({ resolveReply: { error: "gw not found" } });
    const { grab } = await import("../../src/background.js");
    await grab({ id: 1 } as chrome.tabs.Tab);
    await new Promise((r) => setTimeout(r, 0));
    const final = updates.at(-1) as { state: string; error?: string };
    expect(final.state).toBe("error");
    expect(final.error).toContain("gw not found");
  });

  it("ignores a tab with no id", async () => {
    installChrome({});
    const { grab } = await import("../../src/background.js");
    await grab({} as chrome.tabs.Tab);
    expect(updates.length).toBe(0);
  });
});
