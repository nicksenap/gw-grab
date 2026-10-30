import { beforeEach, describe, expect, it } from "vitest";
import { JSDOM } from "jsdom";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import {
  buildJob,
  defaultTarget,
  els,
  parseTarget,
  render,
  renderStartIn,
  renderTargets,
  reposFor,
  slugName,
  suggestName,
  type PanelEls,
} from "../../src/panel-render.js";
import type { DispatchState, Options, Proposal } from "../../src/types.js";

// Use the real panel HTML so id drift between html and code fails here.
const HTML = readFileSync(join(__dirname, "../../public/sidepanel.html"), "utf8");

const opts: Options = {
  presets: { "funnel-ai-full": ["saturn", "conversational-analytics"], "a-preset": ["api"] },
  repos: ["saturn", "conversational-analytics", "widget"],
  agentCmd: "pi --no-skills --skill-profile pocock",
};

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
    candidates: [],
    notes: [],
    ...over,
  };
}

function state(over: Partial<DispatchState> = {}): DispatchState {
  return { items: [], proposal: null, status: "idle", message: "", ...over };
}

const slack = { url: "https://x.slack.com/archives/C1/p1", title: "Data chat slow", body: "", branchHint: "", selection: "p99 12s" };

let e: PanelEls;
beforeEach(() => {
  e = els(new JSDOM(HTML).window.document);
});

describe("targets", () => {
  it("parses preset/repo values and rejects junk", () => {
    expect(parseTarget("preset:funnel-ai-full")).toEqual({ kind: "preset", name: "funnel-ai-full" });
    expect(parseTarget("repo:widget")).toEqual({ kind: "repo", name: "widget" });
    expect(parseTarget("x:y")).toBeNull();
    expect(parseTarget("repo:")).toBeNull();
    expect(parseTarget("")).toBeNull();
  });

  it("lists repos for a target", () => {
    expect(reposFor(parseTarget("preset:funnel-ai-full"), opts)).toEqual(["saturn", "conversational-analytics"]);
    expect(reposFor(parseTarget("repo:widget"), opts)).toEqual(["widget"]);
    expect(reposFor(parseTarget("preset:missing"), opts)).toEqual([]);
    expect(reposFor(null, opts)).toEqual([]);
  });

  it("defaults: resolved repo > last used > first preset > first repo", () => {
    expect(defaultTarget(opts, proposal(), "preset:funnel-ai-full")).toBe("repo:widget");
    expect(defaultTarget(opts, null, "preset:funnel-ai-full")).toBe("preset:funnel-ai-full");
    expect(defaultTarget(opts, null, "preset:gone")).toBe("preset:a-preset");
    expect(defaultTarget({ ...opts, presets: {} }, null, "")).toBe("repo:saturn");
    expect(defaultTarget({ presets: {}, repos: [], agentCmd: "" }, null, "")).toBe("");
  });

  it("renders grouped targets with the selection", () => {
    renderTargets(e, opts, "preset:funnel-ai-full");
    expect(e.target.querySelectorAll("optgroup").length).toBe(2);
    expect(e.target.value).toBe("preset:funnel-ai-full");
  });

  it("start-in offers root + repos, keeps last, picks the only repo", () => {
    renderStartIn(e, ["saturn", "conversational-analytics"], "conversational-analytics");
    expect([...e.startIn.options].map((o) => o.value)).toEqual(["", "saturn", "conversational-analytics"]);
    expect(e.startIn.value).toBe("conversational-analytics");
    renderStartIn(e, ["saturn", "conversational-analytics"], "gone");
    expect(e.startIn.value).toBe("");
    renderStartIn(e, ["widget"], "");
    expect(e.startIn.value).toBe("widget");
  });
});

describe("render", () => {
  it("shows empty hint with no items", () => {
    render(e, state());
    expect(e.empty.classList.contains("hidden")).toBe(false);
    expect(e.clear.disabled).toBe(true);
  });

  it("lists items with selection and remove buttons", () => {
    render(e, state({ items: [slack, { ...slack, url: "https://grafana/d/1", title: "", selection: "" }] }));
    const lis = e.items.querySelectorAll("li");
    expect(lis.length).toBe(2);
    expect(lis[0].querySelector(".t")?.textContent).toBe("Data chat slow");
    expect(lis[0].querySelector(".s")?.textContent).toContain("p99 12s");
    expect(lis[1].querySelector(".t")?.textContent).toBe("https://grafana/d/1");
    expect((lis[1].querySelector(".x") as HTMLElement).dataset.index).toBe("1");
    expect(e.empty.classList.contains("hidden")).toBe(true);
  });

  it("reflects dispatching / error / done", () => {
    render(e, state({ status: "dispatching", message: "Opening Herdr space…" }));
    expect(e.dispatch.disabled).toBe(true);
    expect(e.status.textContent).toBe("Opening Herdr space…");
    render(e, state({ status: "error", message: "boom" }));
    expect(e.status.className).toContain("error");
    expect(e.dispatch.disabled).toBe(false);
    render(e, state({ status: "done", message: "ok" }));
    expect(e.status.className).toContain("ok");
  });
});

describe("names", () => {
  it("slugs to a host-valid name", () => {
    expect(slugName("  Try this out: Grafana p99!! ")).toBe("try-this-out-grafana-p99");
    expect(slugName("feat/cool-thing")).toBe("feat-cool-thing");
    expect(slugName("--x--")).toBe("x");
    expect(slugName("a".repeat(80)).length).toBe(50);
  });

  it("suggests from proposal, else first words of the prompt", () => {
    expect(suggestName(proposal(), "whatever")).toBe("feat-cool-thing");
    expect(suggestName(null, "try this out on stage please now ok")).toBe("try-this-out-on-stage-please");
  });
});

describe("buildJob", () => {
  function fill(target: string, name: string, prompt: string, startIn = "") {
    renderTargets(e, opts, target);
    renderStartIn(e, reposFor(parseTarget(target), opts), startIn);
    e.name.value = name;
    e.prompt.value = prompt;
  }

  it("builds a preset job with captures", () => {
    fill("preset:funnel-ai-full", "Try It", "try this out", "conversational-analytics");
    const job = buildJob(e, state({ items: [slack] }), opts);
    expect(job).toEqual({
      name: "try-it",
      branch: "try-it",
      track: false,
      preset: "funnel-ai-full",
      repos: [],
      startIn: "conversational-analytics",
      agentCmd: opts.agentCmd,
      prompt: "try this out",
      items: [slack],
    });
  });

  it("tracks the PR branch only when targeting that repo", () => {
    fill("repo:widget", "review-it", "review");
    const s = state({ items: [slack], proposal: proposal() });
    const job = buildJob(e, s, opts);
    expect(job).toMatchObject({ track: true, branch: "feat/cool-thing", name: "review-it", repos: ["widget"] });
    fill("preset:funnel-ai-full", "review-it", "review");
    expect(buildJob(e, s, opts)).toMatchObject({ track: false, branch: "review-it" });
  });

  it("uses the custom agent command when set", () => {
    fill("repo:widget", "x", "go");
    e.agentCmd.value = " claude ";
    expect(buildJob(e, state(), opts)).toMatchObject({ agentCmd: "claude" });
  });

  it("falls back to a suggested name", () => {
    fill("repo:widget", "", "fix the flaky test");
    expect(buildJob(e, state(), opts)).toMatchObject({ name: "fix-the-flaky-test" });
  });

  it("returns errors for missing target, name, or content", () => {
    fill("repo:widget", "x", "");
    expect(buildJob(e, state(), opts)).toBe("Add a capture or write an instruction.");
    e.target.innerHTML = "";
    expect(buildJob(e, state(), opts)).toBe("Pick a preset or repo.");
    fill("repo:widget", "!!!", "");
    expect(buildJob(e, state({ items: [slack] }), opts)).toBe("Give the workspace a name.");
  });
});
