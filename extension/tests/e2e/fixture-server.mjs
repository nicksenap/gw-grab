// Minimal static server for e2e fixture pages.
import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { dirname, join, normalize } from "node:path";

const dir = dirname(fileURLToPath(import.meta.url));
const fixtures = join(dir, "..", "fixtures");

const server = createServer(async (req, res) => {
  const name = normalize(decodeURIComponent((req.url ?? "/").split("?")[0])).replace(/^(\.\.[/\\])+/, "");
  try {
    const body = await readFile(join(fixtures, name === "/" ? "github-pr.html" : name));
    res.writeHead(200, { "content-type": "text/html" });
    res.end(body);
  } catch {
    res.writeHead(404);
    res.end("not found");
  }
});

server.listen(5173, () => console.log("fixture server on :5173"));
