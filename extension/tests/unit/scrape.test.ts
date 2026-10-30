import { describe, expect, it } from "vitest";
import { JSDOM } from "jsdom";
import { scrape } from "../../src/scrape.js";

function doc(html: string): Document {
  return new JSDOM(html).window.document;
}

describe("scrape", () => {
  it("extracts a GitHub PR title + head branch", () => {
    const d = doc(`
      <title>PR · acme/widget</title>
      <bdi class="js-issue-title">Add cool thing</bdi>
      <span class="head-ref"><span class="css-truncate-target">feat/cool-thing</span></span>
      <div class="comment-body">This adds the cool thing.</div>
    `);
    const cap = scrape(d, { href: "https://github.com/acme/widget/pull/7", hostname: "github.com" });
    expect(cap.title).toBe("Add cool thing");
    expect(cap.branchHint).toBe("feat/cool-thing");
    expect(cap.body).toContain("cool thing");
    expect(cap.url).toBe("https://github.com/acme/widget/pull/7");
  });

  it("extracts a GitLab MR title + source branch", () => {
    const d = doc(`
      <h1 class="title">Speed up ingest</h1>
      <span class="js-source-branch">perf/ingest</span>
      <div class="description"><div class="md">Make it faster.</div></div>
    `);
    const cap = scrape(d, { href: "https://gitlab.com/g/p/-/merge_requests/3", hostname: "gitlab.com" });
    expect(cap.title).toBe("Speed up ingest");
    expect(cap.branchHint).toBe("perf/ingest");
    expect(cap.body).toContain("faster");
  });

  it("extracts a Slack message body", () => {
    const d = doc(`
      <div class="c-message_kit__blocks">Can we fix the flaky deploy?</div>
      <div class="p-view_header__channel_title">deploys</div>
    `);
    const cap = scrape(d, { href: "https://x.slack.com/archives/C1/p1", hostname: "x.slack.com" });
    expect(cap.title).toBe("Can we fix the flaky deploy?");
    expect(cap.body).toContain("flaky");
  });

  it("falls back to <title> on Slack with only a channel", () => {
    const d = doc(`<div class="p-view_header__channel_title">incidents</div>`);
    const cap = scrape(d, { href: "https://x.slack.com/archives/C1/p1", hostname: "x.slack.com" });
    expect(cap.title).toBe("Slack: incidents");
  });

  it("falls back to <title> for unknown hosts", () => {
    const d = doc(`<title>Ticket #42 — Investigate latency</title>`);
    const cap = scrape(d, { href: "https://tracker.example.com/42", hostname: "tracker.example.com" });
    expect(cap.title).toBe("Ticket #42 — Investigate latency");
    expect(cap.branchHint).toBe("");
    expect(cap.body).toBe("");
  });

  it("captures selection and trims it", () => {
    const d = doc(`<title>x</title>`);
    const cap = scrape(d, { href: "https://e.com", hostname: "e.com" }, "  selected text  ");
    expect(cap.selection).toBe("selected text");
  });

  it("clips very long bodies to 4000 chars", () => {
    const long = "a".repeat(9000);
    const d = doc(`<title>t</title><div class="comment-body">${long}</div>`);
    const cap = scrape(d, { href: "https://github.com/a/b/pull/1", hostname: "github.com" });
    expect(cap.body.length).toBe(4000);
  });

  it("collapses whitespace in extracted text", () => {
    const d = doc(`<bdi class="js-issue-title">Add    cool\n\n  thing</bdi>`);
    const cap = scrape(d, { href: "https://github.com/a/b/pull/1", hostname: "github.com" });
    expect(cap.title).toBe("Add cool thing");
  });
});
