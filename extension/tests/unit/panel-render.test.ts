import { beforeEach, describe, expect, it } from "vitest";
import { JSDOM } from "jsdom";
import { els, populateRepos, readProposal, render, setResult, type PanelEls } from "../../src/panel-render.js";
import type { Proposal } from "../../src/types.js";

// Minimal DOM matching public/sidepanel.html ids.
const HTML = `
  <div id="empty"></div>
  <div id="capturing" class="hidden"></div>
  <form id="plan" class="hidden">
    <span id="provider"></span><span id="sourceTitle"></span>
    <div id="sourceUrl"></div>
    <select id="repo"></select>
    <input id="branch" /><span id="trackTag" class="hidden"></span>
    <input id="name" />
    <ul id="notes"></ul>
    <button id="create"></button>
    <div id="result"></div>
  </form>`;

function proposal(over: Partial<Proposal> = {}): Proposal {
  return {
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
    ...over,
  };
}

let d: Document;
let e: PanelEls;
beforeEach(() => {
  d = new JSDOM(HTML).window.document;
  e = els(d);
});

describe("populateRepos", () => {
  it("lists candidates and selects the chosen one", () => {
    populateRepos(e.repo, ["a", "b", "c"], "b");
    expect(e.repo.value).toBe("b");
    expect(e.repo.options.length).toBe(3);
  });

  it("prepends a selected value not in candidates (clone URL)", () => {
    populateRepos(e.repo, ["a"], "https://github.com/x/y.git");
    expect(e.repo.options.length).toBe(2);
    expect(e.repo.value).toBe("https://github.com/x/y.git");
  });

  it("shows a placeholder when there are no repos", () => {
    populateRepos(e.repo, [], "");
    expect(e.repo.options.length).toBe(1);
    expect(e.repo.options[0].textContent).toContain("no repos");
  });
});

describe("render", () => {
  it("shows the plan form for a proposal and fills fields", () => {
    render(e, { state: "proposal", proposal: proposal(), capture: {} as never });
    expect(e.plan.classList.contains("hidden")).toBe(false);
    expect(e.branch.value).toBe("feat/cool-thing");
    expect(e.name.value).toBe("feat-cool-thing");
    expect(e.provider.textContent).toBe("github");
    expect(e.trackTag.classList.contains("hidden")).toBe(false); // track on
    expect(e.notes.querySelectorAll("li").length).toBe(1);
  });

  it("hides the track tag when not tracking", () => {
    render(e, { state: "proposal", proposal: proposal({ track: false }), capture: {} as never });
    expect(e.trackTag.classList.contains("hidden")).toBe(true);
  });

  it("shows capturing state", () => {
    render(e, { state: "capturing" });
    expect(e.capturing.classList.contains("hidden")).toBe(false);
    expect(e.plan.classList.contains("hidden")).toBe(true);
  });

  it("renders an error into the empty slot", () => {
    render(e, { state: "error", error: "host down" });
    expect(e.empty.textContent).toBe("host down");
    expect(e.empty.className).toContain("error");
  });

  it("idle shows the empty hint", () => {
    render(e, { state: "idle" });
    expect(e.empty.classList.contains("hidden")).toBe(false);
  });

  it("handles a null notes array", () => {
    render(e, { state: "proposal", proposal: proposal({ notes: null }), capture: {} as never });
    expect(e.notes.querySelectorAll("li").length).toBe(0);
  });
});

describe("readProposal", () => {
  it("reflects edited fields back into the proposal", () => {
    render(e, { state: "proposal", proposal: proposal(), capture: {} as never });
    e.repo.value = "acme/api";
    e.branch.value = "  fix/bug  ";
    e.name.value = "  fix-bug  ";
    const p = readProposal(e, proposal());
    expect(p.repo).toBe("acme/api");
    expect(p.branch).toBe("fix/bug");
    expect(p.name).toBe("fix-bug");
    expect(p.sourceUrl).toBe("https://github.com/acme/widget/pull/7"); // base preserved
  });
});

describe("setResult", () => {
  it("sets ok and error styling", () => {
    setResult(e, true, "done");
    expect(e.result.className).toContain("ok");
    setResult(e, false, "nope");
    expect(e.result.className).toContain("error");
    expect(e.result.textContent).toBe("nope");
  });
});

describe("els", () => {
  it("throws on a missing element", () => {
    const bare = new JSDOM("<div></div>").window.document;
    expect(() => els(bare)).toThrow(/missing #/);
  });
});
