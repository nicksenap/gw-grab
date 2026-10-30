// Pure side-panel view logic: render DispatchState + Options into a Document,
// pick form defaults, and build the Job from the form. No chrome.* here, so it
// is fully unit-testable with jsdom.
import type { Capture, DispatchState, Job, Options, Proposal } from "./types.js";

export interface PanelEls {
  empty: HTMLElement;
  items: HTMLOListElement;
  clear: HTMLButtonElement;
  addActive: HTMLButtonElement;
  form: HTMLFormElement;
  prompt: HTMLTextAreaElement;
  target: HTMLSelectElement;
  startIn: HTMLSelectElement;
  name: HTMLInputElement;
  agentCmd: HTMLInputElement;
  dispatch: HTMLButtonElement;
  status: HTMLElement;
}

export function els(doc: Document): PanelEls {
  const get = <T extends HTMLElement>(id: string): T => {
    const el = doc.getElementById(id);
    if (!el) throw new Error("missing #" + id);
    return el as T;
  };
  return {
    empty: get("empty"),
    items: get<HTMLOListElement>("items"),
    clear: get<HTMLButtonElement>("clear"),
    addActive: get<HTMLButtonElement>("addActive"),
    form: get<HTMLFormElement>("job"),
    prompt: get<HTMLTextAreaElement>("prompt"),
    target: get<HTMLSelectElement>("target"),
    startIn: get<HTMLSelectElement>("startIn"),
    name: get<HTMLInputElement>("name"),
    agentCmd: get<HTMLInputElement>("agentCmd"),
    dispatch: get<HTMLButtonElement>("dispatch"),
    status: get("status"),
  };
}

/** Target select values: "preset:<name>" or "repo:<name>". */
export type Target = { kind: "preset" | "repo"; name: string };

export function parseTarget(v: string): Target | null {
  const i = v.indexOf(":");
  if (i < 0) return null;
  const kind = v.slice(0, i);
  const name = v.slice(i + 1);
  if ((kind !== "preset" && kind !== "repo") || !name) return null;
  return { kind, name };
}

/** reposFor lists the repos a target puts in the workspace. */
export function reposFor(t: Target | null, o: Options): string[] {
  if (!t) return [];
  return t.kind === "preset" ? (o.presets[t.name] ?? []) : [t.name];
}

/**
 * defaultTarget: a resolved local repo from the capture (e.g. a GitHub PR) wins,
 * then the last target used, then the first preset, then the first repo.
 */
export function defaultTarget(o: Options, proposal: Proposal | null, last: string): string {
  if (proposal?.repo && o.repos.includes(proposal.repo)) return "repo:" + proposal.repo;
  const t = parseTarget(last);
  if (t && reposFor(t, o).length) return last;
  const presets = Object.keys(o.presets).sort();
  if (presets.length) return "preset:" + presets[0];
  return o.repos.length ? "repo:" + o.repos[0] : "";
}

function option(doc: Document, value: string, label: string, selected: boolean): HTMLOptionElement {
  const o = doc.createElement("option");
  o.value = value;
  o.textContent = label;
  o.selected = selected;
  return o;
}

export function renderTargets(e: PanelEls, o: Options, selected: string): void {
  const doc = e.target.ownerDocument;
  e.target.innerHTML = "";
  const presets = Object.keys(o.presets).sort();
  if (presets.length) {
    const g = doc.createElement("optgroup");
    g.label = "Presets";
    for (const p of presets) g.appendChild(option(doc, "preset:" + p, p, "preset:" + p === selected));
    e.target.appendChild(g);
  }
  if (o.repos.length) {
    const g = doc.createElement("optgroup");
    g.label = "Repos";
    for (const r of [...o.repos].sort()) g.appendChild(option(doc, "repo:" + r, r, "repo:" + r === selected));
    e.target.appendChild(g);
  }
}

/** renderStartIn offers the workspace root plus each repo of the target. */
export function renderStartIn(e: PanelEls, repos: string[], last: string): void {
  const doc = e.startIn.ownerDocument;
  e.startIn.innerHTML = "";
  const pick = repos.includes(last) ? last : repos.length === 1 ? repos[0] : "";
  e.startIn.appendChild(option(doc, "", "workspace root", pick === ""));
  for (const r of repos) e.startIn.appendChild(option(doc, r, r, r === pick));
}

function itemEl(doc: Document, c: Capture, index: number): HTMLLIElement {
  const li = doc.createElement("li");
  const t = doc.createElement("div");
  t.className = "t";
  t.textContent = c.title || c.url;
  t.title = c.title;
  const u = doc.createElement("div");
  u.className = "u";
  u.textContent = c.url;
  li.append(t, u);
  if (c.selection) {
    const s = doc.createElement("div");
    s.className = "s";
    s.textContent = "“" + c.selection + "”";
    li.appendChild(s);
  }
  const x = doc.createElement("button");
  x.type = "button";
  x.className = "x";
  x.textContent = "×";
  x.title = "Remove";
  x.dataset.index = String(index);
  li.appendChild(x);
  return li;
}

/** render reflects the list + status; it never touches the user's form inputs. */
export function render(e: PanelEls, s: DispatchState): void {
  const doc = e.items.ownerDocument;
  e.items.innerHTML = "";
  s.items.forEach((c, i) => e.items.appendChild(itemEl(doc, c, i)));
  e.empty.classList.toggle("hidden", s.items.length > 0);
  e.clear.disabled = s.items.length === 0;
  e.status.textContent = s.message;
  e.status.className = "status" + (s.status === "error" ? " error" : s.status === "done" ? " ok" : "");
  e.dispatch.disabled = s.status === "dispatching" || s.status === "capturing";
  e.dispatch.textContent = s.status === "dispatching" ? "Dispatching…" : "Dispatch";
}

/** slugName makes a valid workspace/branch name (matches the host's rule). */
export function slugName(s: string): string {
  return s
    .toLowerCase()
    .replace(/[^a-z0-9._-]+/g, "-")
    .replace(/^[-._]+|-+$/g, "")
    .slice(0, 50)
    .replace(/-+$/g, "");
}

/** suggestName: the resolved proposal's name, else a slug of the instruction. */
export function suggestName(proposal: Proposal | null, prompt: string): string {
  return slugName(proposal?.name || prompt.split(/\s+/).slice(0, 6).join(" "));
}

/** buildJob reads the form into a Job, or returns an error message. */
export function buildJob(e: PanelEls, s: DispatchState, o: Options): Job | string {
  const t = parseTarget(e.target.value);
  if (!t) return "Pick a preset or repo.";
  const name = slugName(e.name.value) || suggestName(s.proposal, e.prompt.value);
  if (!name) return "Give the workspace a name.";
  const prompt = e.prompt.value.trim();
  if (!prompt && !s.items.length) return "Add a capture or write an instruction.";
  const p = s.proposal;
  // Only check out the PR's existing branch when targeting that same repo.
  const track = !!p?.track && t.kind === "repo" && t.name === p.repo && p.branch !== "";
  return {
    name,
    branch: track ? p!.branch : name,
    track,
    preset: t.kind === "preset" ? t.name : "",
    repos: t.kind === "repo" ? [t.name] : [],
    startIn: e.startIn.value,
    agentCmd: e.agentCmd.value.trim() || o.agentCmd,
    prompt,
    items: s.items,
  };
}
