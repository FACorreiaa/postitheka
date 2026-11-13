#!/usr/bin/env node
// Gzip size budgets for the built web app (web/dist), checked by `task check`.
//
// Two numbers, kept apart on purpose (BRD-001):
// - entry: what every visitor downloads before the first paint, the scripts and stylesheets
//   index.html names, plus what they preload. Must not grow without a decision.
// - lazy: every other script, loaded on demand (the 3D mascot's three.js chunk). It may grow
//   when a lazy feature lands, and its budget is raised in the same change.
//
// `node scripts/size-budget.mjs --update` rewrites scripts/size-budget.json to the current
// sizes plus 1%, rounded up to the next KiB. Review that diff like code.
//
// The 1%: Node's gzip does not give the same bytes everywhere. The same dist measured 855 B
// (0.6%) more on CI's Linux x86 runner than on an Apple silicon Mac, and a budget set on the
// Mac with 205 B to spare failed CI.
import { readFileSync, readdirSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { gzipSync } from "node:zlib";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const dist = join(root, "web", "dist");
const budgetFile = join(root, "scripts", "size-budget.json");

const gz = (file) => gzipSync(readFileSync(join(dist, file)), { level: 9 }).length;

const html = readFileSync(join(dist, "index.html"), "utf8");
const entryFiles = new Set(
  [...html.matchAll(/(?:src|href)="\/(assets\/[^"]+\.(?:js|css))"/g)].map((m) => m[1]),
);
const scripts = readdirSync(join(dist, "assets"))
  .filter((name) => name.endsWith(".js"))
  .map((name) => `assets/${name}`);

const sum = (files) => files.reduce((total, file) => total + gz(file), 0);
const sizes = {
  entry: sum([...entryFiles]),
  lazy: sum(scripts.filter((file) => !entryFiles.has(file))),
};

if (process.argv.includes("--update")) {
  const rounded = Object.fromEntries(Object.entries(sizes).map(([k, v]) => [k, Math.ceil((v * 1.01) / 1024) * 1024]));
  writeFileSync(budgetFile, `${JSON.stringify(rounded, null, 2)}\n`);
  console.log("size-budget: updated", rounded);
  process.exit(0);
}

const budget = JSON.parse(readFileSync(budgetFile, "utf8"));
let over = false;
for (const [name, size] of Object.entries(sizes)) {
  const limit = budget[name];
  const line = `${name}: ${size} B gzip (budget ${limit} B)`;
  if (size > limit) {
    over = true;
    console.error(`size-budget: OVER ${line}`);
  } else {
    console.log(`size-budget: ${line}`);
  }
}
if (over) {
  console.error("size-budget: the entry must not grow without a decision; a lazy feature raises `lazy` with --update in the same change");
  process.exit(1);
}
