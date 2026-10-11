// Static checks for site/: every local link and asset exists, no inline
// script/style or event handlers (the CSP forbids them), and every page has
// a title, description and canonical URL. Usage: node tools/check-site.mjs site
import { readFileSync, existsSync, readdirSync } from "node:fs";
import { join } from "node:path";

const root = process.argv[2] ?? "site";
const pages = readdirSync(root).filter((f) => f.endsWith(".html"));
const problems = [];

for (const page of pages) {
  const html = readFileSync(join(root, page), "utf8");
  const where = (msg) => problems.push(`${page}: ${msg}`);
  if (/<script(?![^>]*\b(src=|type="application\/ld\+json"))[^>]*>/i.test(html)) where("inline <script>");
  if (/<style[\s>]/i.test(html) || /\sstyle="/i.test(html)) where("inline style");
  if (/\son[a-z]+="/i.test(html)) where("inline event handler");
  if (!/<title>[^<]+<\/title>/.test(html)) where("missing <title>");
  if (!/<meta name="description" content="[^"]+">/.test(html)) where("missing description");
  if (page !== "404.html" && !/<link rel="canonical" href="https:\/\/goli\.theanalogyarchitect\.com/.test(html)) where("missing canonical");
  for (const [, ref] of html.matchAll(/(?:href|src)="(\/[^"#?]*)/g)) {
    const path = ref === "/" ? "index.html" : ref.slice(1);
    const candidates = [path, `${path}.html`, join(path, "index.html")];
    if (!candidates.some((p) => existsSync(join(root, p)))) where(`broken local link ${ref}`);
  }
}

if (problems.length) {
  console.error(problems.join("\n"));
  process.exit(1);
}
console.log(`site OK: ${pages.length} pages checked`);
