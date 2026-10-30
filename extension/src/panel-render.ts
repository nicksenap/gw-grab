// Pure side-panel view logic: render a GrabState into a given Document and read
// the (possibly edited) Proposal back out of the form. No chrome.* here, so it
// is fully unit-testable with jsdom.
import type { GrabState, Proposal } from "./types.js";

export interface PanelEls {
  empty: HTMLElement;
  capturing: HTMLElement;
  plan: HTMLElement;
  provider: HTMLElement;
  sourceTitle: HTMLElement;
  sourceUrl: HTMLElement;
  repo: HTMLSelectElement;
  branch: HTMLInputElement;
  name: HTMLInputElement;
  trackTag: HTMLElement;
  notes: HTMLElement;
  result: HTMLElement;
  create: HTMLButtonElement;
}

export function els(doc: Document): PanelEls {
  const get = <T extends HTMLElement>(id: string): T => {
    const el = doc.getElementById(id);
    if (!el) throw new Error("missing #" + id);
    return el as T;
  };
  return {
    empty: get("empty"),
    capturing: get("capturing"),
    plan: get("plan"),
    provider: get("provider"),
    sourceTitle: get("sourceTitle"),
    sourceUrl: get("sourceUrl"),
    repo: get<HTMLSelectElement>("repo"),
    branch: get<HTMLInputElement>("branch"),
    name: get<HTMLInputElement>("name"),
    trackTag: get("trackTag"),
    notes: get("notes"),
    result: get("result"),
    create: get<HTMLButtonElement>("create"),
  };
}

function show(el: HTMLElement, visible: boolean): void {
  el.classList.toggle("hidden", !visible);
}

/** populateRepos fills the repo <select> with candidates, selecting `selected`. */
export function populateRepos(sel: HTMLSelectElement, candidates: string[], selected: string): void {
  sel.innerHTML = "";
  const opts = [...candidates];
  // Ensure the selected value is present (e.g. a clone URL not in candidates).
  if (selected && !opts.includes(selected)) opts.unshift(selected);
  if (!opts.length) {
    const o = sel.ownerDocument.createElement("option");
    o.value = "";
    o.textContent = "(no repos discovered)";
    sel.appendChild(o);
  }
  for (const c of opts) {
    const o = sel.ownerDocument.createElement("option");
    o.value = c;
    o.textContent = c;
    if (c === selected) o.selected = true;
    sel.appendChild(o);
  }
}

/** render reflects a GrabState into the panel DOM. */
export function render(e: PanelEls, state: GrabState): void {
  show(e.empty, state.state === "idle");
  show(e.capturing, state.state === "capturing");
  show(e.plan, state.state === "proposal");
  e.result.textContent = "";
  e.result.className = "status";

  if (state.state === "error") {
    show(e.empty, true);
    e.empty.textContent = state.error;
    e.empty.className = "error";
    return;
  }
  if (state.state !== "proposal") return;

  const p = state.proposal;
  e.provider.textContent = p.provider || "task";
  e.sourceTitle.textContent = p.sourceTitle;
  e.sourceUrl.textContent = p.sourceUrl;
  populateRepos(e.repo, p.candidates ?? [], p.repo);
  e.branch.value = p.branch;
  e.name.value = p.name;
  show(e.trackTag, p.track);

  e.notes.innerHTML = "";
  for (const note of p.notes ?? []) {
    const li = e.notes.ownerDocument.createElement("li");
    li.textContent = note;
    e.notes.appendChild(li);
  }
  e.create.disabled = false;
}

/** readProposal reconstructs a Proposal from the (edited) form fields. */
export function readProposal(e: PanelEls, base: Proposal): Proposal {
  return {
    ...base,
    repo: e.repo.value,
    repoDisplay: e.repo.value,
    branch: e.branch.value.trim(),
    name: e.name.value.trim(),
  };
}

export function setResult(e: PanelEls, ok: boolean, msg: string): void {
  e.result.textContent = msg;
  e.result.className = "status " + (ok ? "ok" : "error");
}
