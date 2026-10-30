// Wire contract between the extension and the gw-grab native host. These mirror
// the Go structs in internal/capture and internal/resolver (JSON field names
// must stay in sync).

/** What the content script scrapes from the page. */
export interface Capture {
  url: string;
  title: string;
  body: string;
  branchHint: string;
  selection: string;
}

/** The resolved plan the host returns and the panel edits. */
export interface Proposal {
  provider: string;
  repo: string; // local repo name, clone URL on miss, or "" (pick in panel)
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

/** Request sent to the native host (action-tagged). */
export type HostRequest =
  | { action: "resolve"; capture: Capture }
  | { action: "create"; plan: Proposal }
  | { action: "repos" };

/** Response from the native host. */
export interface HostResponse {
  ok?: boolean;
  error?: string;
  proposal?: Proposal;
  repos?: RepoInfo[];
}

/** State the background worker holds and broadcasts to the side panel. */
export type GrabState =
  | { state: "idle" }
  | { state: "capturing" }
  | { state: "proposal"; proposal: Proposal; capture: Capture }
  | { state: "error"; error: string; capture?: Capture };

/** Messages between side panel and background worker. */
export type PanelMessage =
  | { type: "grab:getState" }
  | { type: "grab:create"; plan: Proposal };

export interface PanelUpdate {
  type: "grab:update";
  payload: GrabState;
}
