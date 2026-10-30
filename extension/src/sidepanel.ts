// sidepanel.ts — thin chrome wiring around the pure panel-render logic.
import type { DispatchState, Options, PanelUpdate } from "./types.js";
import {
  buildJob,
  defaultTarget,
  els,
  parseTarget,
  render,
  renderStartIn,
  renderTargets,
  reposFor,
  suggestName,
} from "./panel-render.js";

const e = els(document);
let state: DispatchState = { items: [], proposal: null, status: "idle", message: "" };
let opts: Options | null = null;
let nameTouched = false;
let lastProposalUrl = "";
let prefs = { lastTarget: "", lastStartIn: "", agentCmd: "" };

const send = <T>(msg: unknown) => chrome.runtime.sendMessage(msg) as Promise<T>;

function refreshStartIn(): void {
  if (!opts) return;
  renderStartIn(e, reposFor(parseTarget(e.target.value), opts), prefs.lastStartIn);
}

function prefill(): void {
  if (!opts) return;
  const url = state.proposal?.sourceUrl ?? "";
  if (url !== lastProposalUrl) {
    lastProposalUrl = url;
    // A new first capture may imply a repo (e.g. GitHub PR).
    renderTargets(e, opts, defaultTarget(opts, state.proposal, prefs.lastTarget));
    refreshStartIn();
    if (!nameTouched) e.name.value = suggestName(state.proposal, e.prompt.value);
  }
}

function apply(s: DispatchState): void {
  state = s;
  render(e, s);
  if (s.status === "done") {
    e.prompt.value = "";
    nameTouched = false;
  }
  prefill();
}

async function init(): Promise<void> {
  prefs = { ...prefs, ...(await chrome.storage.local.get(["lastTarget", "lastStartIn", "agentCmd"])) };
  const o = await send<Options | { error: string }>({ type: "grab:options" });
  if ("error" in o) {
    e.status.textContent = o.error;
    e.status.className = "status error";
    return;
  }
  opts = o;
  e.agentCmd.value = prefs.agentCmd || o.agentCmd;
  e.agentCmd.placeholder = o.agentCmd;
  renderTargets(e, o, defaultTarget(o, null, prefs.lastTarget));
  refreshStartIn();
  apply(await send<DispatchState>({ type: "grab:getState" }));
}

chrome.runtime.onMessage.addListener((msg: PanelUpdate) => {
  if (msg?.type === "grab:update") apply(msg.payload);
});

e.target.addEventListener("change", () => {
  prefs.lastTarget = e.target.value;
  void chrome.storage.local.set({ lastTarget: e.target.value });
  refreshStartIn();
});
e.startIn.addEventListener("change", () => {
  prefs.lastStartIn = e.startIn.value;
  void chrome.storage.local.set({ lastStartIn: e.startIn.value });
});
e.agentCmd.addEventListener("change", () => {
  void chrome.storage.local.set({ agentCmd: e.agentCmd.value.trim() });
});
e.name.addEventListener("input", () => (nameTouched = e.name.value !== ""));
e.prompt.addEventListener("input", () => {
  if (!nameTouched && !state.proposal) e.name.value = suggestName(null, e.prompt.value);
});
e.prompt.addEventListener("keydown", (ev) => {
  if (ev.key === "Enter" && (ev.metaKey || ev.ctrlKey)) {
    ev.preventDefault();
    e.form.requestSubmit();
  }
});

e.items.addEventListener("click", (ev) => {
  const i = (ev.target as HTMLElement).dataset?.index;
  if (i !== undefined) void send({ type: "grab:remove", index: Number(i) });
});
e.clear.addEventListener("click", () => void send({ type: "grab:clear" }));
e.addActive.addEventListener("click", () => void send({ type: "grab:addActive" }));

e.form.addEventListener("submit", (ev) => {
  ev.preventDefault();
  if (!opts) return;
  const job = buildJob(e, state, opts);
  if (typeof job === "string") {
    e.status.textContent = job;
    e.status.className = "status error";
    return;
  }
  e.name.value = job.name;
  void send({ type: "grab:dispatch", job });
});

void init();
