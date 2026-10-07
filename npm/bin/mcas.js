#!/usr/bin/env node
// Thin wrapper that runs the platform-specific mcas binary, which npm installs
// from the matching optional dependency.
"use strict";

const { spawnSync } = require("child_process");

const pkg = `@aazerzai/mcas-cli-${process.platform}-${process.arch}`;
const ext = process.platform === "win32" ? ".exe" : "";

let bin;
try {
  bin = require.resolve(`${pkg}/bin/mcas${ext}`);
} catch {
  console.error(
    `mcas: no binary package for ${process.platform}/${process.arch} (${pkg}).\n` +
      "Reinstall without --no-optional / --omit=optional, or check that your platform is supported.",
  );
  process.exit(1);
}

const res = spawnSync(bin, process.argv.slice(2), { stdio: "inherit" });
if (res.error) {
  console.error(res.error.message);
  process.exit(1);
}
process.exit(res.status === null ? 1 : res.status);
