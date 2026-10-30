// content.ts — injected into the active tab via chrome.scripting. It runs the
// pure scrape() against the live document and stashes the result on a global.
//
// esbuild bundles this as an IIFE, so the build appends a `__groveGrabResult;`
// footer: that trailing bare expression is the script's completion value, which
// is what chrome.scripting.executeScript returns to the background worker.
import { scrape } from "./scrape.js";

declare global {
  // eslint-disable-next-line no-var
  var __groveGrabResult: import("./types.js").Capture | undefined;
}

globalThis.__groveGrabResult = scrape(
  document,
  { href: location.href, hostname: location.hostname },
  window.getSelection?.()?.toString() ?? "",
);
