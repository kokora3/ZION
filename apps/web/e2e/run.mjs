import { spawn } from "node:child_process";
import path from "node:path";
import process from "node:process";

const root = path.resolve(import.meta.dirname, "..");
const children = [];
function start(command, args, options = {}) {
  const child = spawn(command, args, { cwd:root, stdio:"inherit", windowsHide:true, ...options });
  children.push(child);
  return child;
}
function stopAll() {
  for (const child of children.reverse()) {
    if (!child.killed) child.kill();
  }
}
async function waitFor(url, timeout = 60_000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    try { const response = await fetch(url); if (response.ok) return; } catch { /* process may still be starting */ }
    await new Promise((resolve) => setTimeout(resolve, 200));
  }
  throw new Error(`Timed out waiting for ${url}`);
}
function exited(child) { return new Promise((resolve, reject) => { child.once("error", reject); child.once("exit", (code) => resolve(code ?? 1)); }); }

process.once("SIGINT", () => { stopAll(); process.exit(130); });
process.once("SIGTERM", () => { stopAll(); process.exit(143); });

let exitCode = 1;
try {
  start(path.join(root, ".e2e", "zion-web-e2e-node.exe"), [], { env:{...process.env,ZION_WEB_E2E:"1"} });
  start(process.execPath, [path.join(root,"node_modules","next","dist","bin","next"),"dev","--hostname","127.0.0.1","--port","3000"], { env:{...process.env,NEXT_PUBLIC_ZION_API_URL:"http://127.0.0.1:42001"} });
  await Promise.all([waitFor("http://127.0.0.1:42001/v1/health"), waitFor("http://127.0.0.1:3000")]);
  const runner = start(process.execPath, [path.join(root,"node_modules","@playwright","test","cli.js"),"test"]);
  exitCode = await exited(runner);
} catch (error) {
  console.error(error);
} finally {
  stopAll();
}
process.exit(exitCode);
