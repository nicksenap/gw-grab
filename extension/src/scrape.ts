// Pure DOM scraping: turn a rendered page into a Capture. Kept free of any
// chrome.* or global `document` references so it can be unit-tested by passing a
// jsdom Document + a location-like object.
import type { Capture } from "./types.js";

export interface Loc {
  href: string;
  hostname: string;
}

const MAX_BODY = 4000;

function text(el: Element | null): string {
  return el ? (el.textContent ?? "").trim().replace(/\s+/g, " ") : "";
}

function firstText(doc: Document, selectors: string[]): string {
  for (const sel of selectors) {
    const t = text(doc.querySelector(sel));
    if (t) return t;
  }
  return "";
}

interface SitePart {
  title?: string;
  body?: string;
  branchHint?: string;
}

function github(doc: Document): SitePart {
  return {
    title: firstText(doc, [".js-issue-title", "bdi.js-issue-title", "h1 .markdown-title"]),
    branchHint: firstText(doc, [
      ".head-ref .css-truncate-target",
      ".commit-ref.head-ref",
      "span.head-ref",
    ]),
    body: firstText(doc, [".comment-body", ".markdown-body"]),
  };
}

function gitlab(doc: Document): SitePart {
  return {
    title: firstText(doc, [".title.qa-title", "h1.title", ".issuable-details .title"]),
    branchHint: firstText(doc, [".js-source-branch", ".ref-container .gl-link", "span.source-branch"]),
    body: firstText(doc, [".description .md", ".issuable-details .md"]),
  };
}

function notion(doc: Document): SitePart {
  const title =
    firstText(doc, [".notion-page-block .notranslate", 'div[placeholder="Untitled"]', "h1.notion-page-title"]) ||
    text(doc.querySelector("title"));
  const main = doc.querySelector(".notion-page-content") ?? doc.body;
  return { title, body: text(main).slice(0, MAX_BODY) };
}

function slack(doc: Document): SitePart {
  const msg = firstText(doc, [".c-message_kit__blocks", ".p-rich_text_section", ".c-message__body"]);
  const channel = firstText(doc, [".p-view_header__channel_title", ".p-channel_sidebar__name--selected"]);
  return { title: msg || (channel ? "Slack: " + channel : ""), body: msg };
}

/** scrape extracts a Capture from a document for the given location. */
export function scrape(doc: Document, loc: Loc, selection = ""): Capture {
  const host = loc.hostname;
  let part: SitePart;
  if (host === "github.com") part = github(doc);
  else if (host.includes("gitlab")) part = gitlab(doc);
  else if (host.endsWith("notion.so") || host.endsWith("notion.com")) part = notion(doc);
  else if (host.endsWith("slack.com")) part = slack(doc);
  else part = { title: text(doc.querySelector("title")) };

  return {
    url: loc.href,
    title: part.title || text(doc.querySelector("title")),
    body: (part.body || "").slice(0, MAX_BODY),
    branchHint: part.branchHint || "",
    selection: selection.trim(),
  };
}
