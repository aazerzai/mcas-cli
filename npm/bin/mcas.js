#!/usr/bin/env node
// Thin wrapper that runs the binary fetched by scripts/install.js.
"use strict";

const { spawnSync } = require("child_process");
const fs = require("fs");
const path = require("path");

const ext = process.platform === "win32" ? ".exe" : "";
const bin = path.join(__dirname, `mcas-bin${ext}`);

if (!fs.existsSync(bin)) {
  console.error("mcas binary not found; try reinstalling @aazerzai/mcas-cli");
  process.exit(1);
}

const res = spawnSync(bin, process.argv.slice(2), { stdio: "inherit" });
if (res.error) {
  console.error(res.error.message);
  process.exit(1);
}
process.exit(res.status === null ? 1 : res.status);
