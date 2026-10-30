// Wire contract between the extension and the gw-grab native host. These mirror
// the Go structs in internal/capture, internal/resolver and internal/dispatch
// (JSON field names must stay in sync).

/** What the content script scrapes from the page (or a right-clicked link). */
export interface Capture {
  url: string;
  title: string;
  body: string;
  branchHint: string;
  selection: string;
}

/** The resolved plan the host returns for a capture (used to prefill the form). */
export interface Proposal {
  provider: string;
  repo: string; // local repo name, clone URL on miss, or ""
  repoDisplay: string;
  branch: string;
  name: string;
  track: boolean;
  sourceUrl: string;
  sourceRef: string;
  sourceTitle: string;
  candidates: string[];
  notes: string[] | null;
}

export interface RepoInfo {
  name: string;
  path: string;
  remote: string;
  display_name: string;
}

/** One dispatch: create workspace, open background Herdr space, start agent. */
export interface Job {
  name: string;
  branch: string;
  track: boolean;
  preset: string;
  repos: string[];
  startIn: string; // repo name, "" = workspace root
  agentCmd: string;
  prompt: string;
  items: Capture[];
}

export interface DispatchResult {
  workspace: string;
  path: string;
  herdr: string;
  pane: string;
  brief: string;
}

/** Request sent to the native host (action-tagged). */
export type HostRequest =
  | { action: "resolve"; capture: Capture }
  | { action: "create"; plan: Proposal }
  | { action: "repos" }
  | { action: "options" }
  | { action: "dispatch"; job: Job };

/** Response from the native host. A dispatch streams {progress} messages first. */
export interface HostResponse {
  ok?: boolean;
  error?: string;
  proposal?: Proposal;
  repos?: RepoInfo[];
  presets?: Record<string, string[]>;
  agentCmd?: string;
  progress?: string;
  result?: DispatchResult;
}

/** Targets/defaults the panel offers, from the host's "options" action. */
export interface Options {
  presets: Record<string, string[]>;
  repos: string[];
  agentCmd: string;
}

export type Status = "idle" | "capturing" | "dispatching" | "done" | "error";

/** State the background worker holds and broadcasts to the side panel. */
export interface DispatchState {
  items: Capture[];
  proposal: Proposal | null; // resolved from the first item
  status: Status;
  message: string; // progress, error, or done text
  result?: DispatchResult;
}

/** Messages from the side panel to the background worker. */
export type PanelMessage =
  | { type: "grab:getState" }
  | { type: "grab:addActive" }
  | { type: "grab:remove"; index: number }
  | { type: "grab:clear" }
  | { type: "grab:options" }
  | { type: "grab:dispatch"; job: Job };

export interface PanelUpdate {
  type: "grab:update";
  payload: DispatchState;
}
