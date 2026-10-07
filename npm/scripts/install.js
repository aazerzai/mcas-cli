#!/usr/bin/env node
// Downloads the mcas binary for this platform from the GitHub Release that
// matches this package's version, and verifies its SHA-256.
"use strict";

const crypto = require("crypto");
const fs = require("fs");
const path = require("path");

const pkg = require("../package.json");

const OS = { darwin: "darwin", linux: "linux", win32: "windows" };
const ARCH = { x64: "amd64", arm64: "arm64" };

async function get(url) {
  const res = await fetch(url, { redirect: "follow" });
  if (!res.ok) throw new Error(`GET ${url} failed: HTTP ${res.status}`);
  return Buffer.from(await res.arrayBuffer());
}

async function main() {
  const os = OS[process.platform];
  const arch = ARCH[process.arch];
  if (!os || !arch) {
    throw new Error(`unsupported platform ${process.platform}/${process.arch}`);
  }
  const ext = os === "windows" ? ".exe" : "";
  const asset = `mcas_${os}_${arch}${ext}`;
  const base = `https://github.com/aazerzai/mcas-cli/releases/download/v${pkg.version}`;

  const [binary, sums] = await Promise.all([
    get(`${base}/${asset}`),
    get(`${base}/checksums.txt`),
  ]);

  const line = sums
    .toString("utf8")
    .split("\n")
    .find((l) => l.trim().endsWith(` ${asset}`));
  if (!line) throw new Error(`no checksum listed for ${asset}`);
  const want = line.trim().split(/\s+/)[0];
  const got = crypto.createHash("sha256").update(binary).digest("hex");
  if (want !== got) throw new Error(`checksum mismatch for ${asset}`);

  const dir = path.join(__dirname, "..", "bin");
  fs.mkdirSync(dir, { recursive: true });
  const dest = path.join(dir, `mcas-bin${ext}`);
  fs.writeFileSync(dest, binary, { mode: 0o755 });
}

main().catch((err) => {
  console.error(`@aazerzai/mcas-cli: ${err.message}`);
  process.exit(1);
});
