// sidepanel.ts — thin chrome wiring around the pure panel-render logic. Listens
// for state broadcasts from the background worker and sends the create request.
import type { GrabState, HostResponse, Proposal, PanelUpdate } from "./types.js";
import { els, readProposal, render, setResult } from "./panel-render.js";

const e = els(document);
let current: Proposal | null = null;

function apply(state: GrabState): void {
  render(e, state);
  current = state.state === "proposal" ? state.proposal : null;
}

// Initial state + live updates.
chrome.runtime.sendMessage({ type: "grab:getState" }).then((s: GrabState) => {
  if (s) apply(s);
});

chrome.runtime.onMessage.addListener((msg: PanelUpdate) => {
  if (msg?.type === "grab:update") apply(msg.payload);
});

e.create.addEventListener("click", (ev) => {
  ev.preventDefault();
  if (!current) return;
  const plan = readProposal(e, current);
  if (!plan.repo) {
    setResult(e, false, "Pick a repository first.");
    return;
  }
  e.create.disabled = true;
  setResult(e, true, "Creating…");
  chrome.runtime.sendMessage({ type: "grab:create", plan }).then((resp: HostResponse) => {
    if (resp?.ok) {
      setResult(e, true, "✓ Workspace created: " + plan.name);
    } else {
      setResult(e, false, resp?.error ?? "Failed to create workspace");
      e.create.disabled = false;
    }
  });
});
