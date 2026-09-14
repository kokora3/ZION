import path from "node:path";
import { execFileSync } from "node:child_process";
import { expect, test } from "@playwright/test";

type Fixture = {
  transaction:string;
  board_event:string;
  post_id:string;
  research_id:string;
  resource_id:string;
  missing_object_id:string;
  research_transactions:string[];
  resource_transactions:string[];
  admitted_research_id:string;
  admitted_resource_id:string;
};
const webRoot = path.resolve(import.meta.dirname, "..");
const fixture = JSON.parse(execFileSync(path.resolve(webRoot, ".e2e/zion-web-e2e-node.exe"), ["--fixture"], { encoding:"utf8" })) as Fixture;

test.describe.serial("ZION Web against a real local Runtime", () => {
  test("overview exposes readiness and canonical/operational boundaries", async ({page}) => {
    await page.goto("/");
    await expect(page.getByText("Connected", {exact:true})).toBeVisible();
    await expect(page.getByText("zion-alpha-1", {exact:true}).first()).toBeVisible();
    await expect(page.getByText("75", {exact:true})).toBeVisible();
    await expect(page.getByText("StateHash", {exact:true})).toBeVisible();
    await expect(page.getByText("NORMAL", {exact:true}).first()).toBeVisible();
    await expect(page.getByText("Validator authority", {exact:true})).toBeVisible();
  });

  test("Board feed, search, thread and local visibility work without chain mutation", async ({page,request}) => {
    const before = await (await request.get("http://127.0.0.1:42001/v1/status")).json() as {state_hash:string};
    await page.goto("/board");
    await expect(page.getByRole("link", {name:"Welcome to the ZION alpha Board"})).toBeVisible();
    await page.getByPlaceholder(/Search this node/).fill("Welcome");
    await page.getByRole("button", {name:"Local search"}).click();
    await page.getByRole("link", {name:"Welcome to the ZION alpha Board"}).click();
    await expect(page.getByText("Replies (1)")).toBeVisible();
    await expect(page.getByText(/bounded signed reply/)).toBeVisible();
    await page.getByRole("button", {name:"Hide on this node"}).click();
    await expect(page.getByRole("button", {name:"Unhide on this node"})).toBeVisible();
    await expect(page.getByText(/not global deletion/)).toBeVisible();
    const after = await (await request.get("http://127.0.0.1:42001/v1/status")).json() as {state_hash:string};
    expect(after.state_hash).toBe(before.state_hash);
    await page.getByRole("button", {name:"Unhide on this node"}).click();
  });

  test("Research and Resource discovery shows object and URI semantics", async ({page}) => {
    await page.goto("/research");
    await expect(page.getByRole("link", {name:"Deterministic AI security evaluation"})).toBeVisible();
    await page.getByPlaceholder(/Title, summary/).fill("evaluation");
    await page.getByRole("button", {name:"Local search"}).click();
    await page.getByRole("link", {name:"Deterministic AI security evaluation"}).click();
    await expect(page.getByText("CANONICAL", {exact:true}).first()).toBeVisible();
    await expect(page.getByText("MISSING LOCALLY", {exact:true})).toBeVisible();
    await expect(page.getByRole("button", {name:"Fetch from peers"})).toBeVisible();
    await expect(page.getByText(/mutable location/)).toBeVisible();
    await page.goto("/resources");
    await page.getByRole("link", {name:"Bounded Analysis Toolkit"}).click();
    await expect(page.getByText("TOOL", {exact:true})).toBeVisible();
    await expect(page.getByText(/will not run or install/)).toBeVisible();
    await expect(page.getByRole("button", {name:/Run|Install/})).toHaveCount(0);
    await expect(page.getByText("Canonical relations")).toBeVisible();
  });

  test("Governance keeps OPEN, APPROVED, and EXECUTED distinct", async ({page}) => {
    await page.goto("/governance");
    await expect(page.getByText("OPEN", {exact:true})).toBeVisible();
    await expect(page.getByText("APPROVED", {exact:true})).toBeVisible();
    await expect(page.getByText("EXECUTED", {exact:true}).first()).toBeVisible();
    await expect(page.getByText(/Approved, not executed/)).toBeVisible();
    await expect(page.getByText(/YES 3 · NO 0 · ABSTAIN 0/).first()).toBeVisible();
  });

  test("syncing is live but not ready and mutation controls are guarded", async ({page, request}) => {
    const status = await (await request.get("http://127.0.0.1:42001/v1/status")).json() as Record<string, unknown>;
    await page.route("http://127.0.0.1:42001/v1/status", (route) => route.fulfill({json:{...status, sync_status:"SYNCING"}}));
    await page.route("http://127.0.0.1:42001/v1/health", (route) => route.fulfill({json:{live:true, ready:false}}));
    await page.goto("/governance");
    await expect(page.getByText("Syncing", {exact:true})).toBeVisible();
    await expect(page.getByRole("button", {name:"Unavailable until node is ready and compatible"})).toBeDisabled();
    await page.unrouteAll({behavior:"wait"});
  });

  test("already-signed transaction and Board event use normal node paths", async ({page}) => {
    await page.goto("/governance");
    const transactionForm=page.getByRole("heading",{name:"Submit signed canonical transaction"}).locator("..");
    await transactionForm.getByLabel("Base64 signed payload").fill(fixture.transaction);
    await transactionForm.getByRole("button",{name:"Submit to local node"}).click();
    await expect(transactionForm.getByText("SUBMITTED_TO_CONSENSUS")).toBeVisible();
    await transactionForm.getByRole("link",{name:"Track node-observed finality"}).click();
    await expect(page.getByText("FINALIZED",{exact:true}).first()).toBeVisible();
    await expect(page.getByText(/Committed at height/)).toBeVisible();
    await page.goto("/board");
    const boardForm=page.getByRole("heading",{name:"Submit signed Board event"}).locator("..");
    await boardForm.getByLabel("Base64 signed payload").fill(fixture.board_event);
    await boardForm.getByRole("button",{name:"Submit to local node"}).click();
    await expect(boardForm.getByText(/Accepted by local Board path/)).toBeVisible();
    await expect(boardForm.getByText(fixture.post_id,{exact:true})).toBeVisible();
  });

  test("signed governance imports admit Research and Resource canonically", async ({page}) => {
    const submitAll = async (transactions: string[]) => {
      for (const transaction of transactions) {
        await page.goto("/governance");
        const form = page.getByRole("heading", {name:"Submit signed canonical transaction"}).locator("..");
        await form.getByLabel("Base64 signed payload").fill(transaction);
        await form.getByRole("button", {name:"Submit to local node"}).click();
        await expect(form.getByText("SUBMITTED_TO_CONSENSUS")).toBeVisible();
      }
    };
    await submitAll(fixture.research_transactions);
    await page.goto(`/research/${encodeURIComponent(fixture.admitted_research_id)}`);
    await expect(page.getByRole("heading", {name:"Web-imported canonical research"})).toBeVisible();
    await expect(page.getByText("CANONICAL", {exact:true}).first()).toBeVisible();
    await submitAll(fixture.resource_transactions);
    await page.goto(`/resources/${encodeURIComponent(fixture.admitted_resource_id)}`);
    await expect(page.getByRole("heading", {name:"Web-imported canonical resource"})).toBeVisible();
    await expect(page.getByText("DATASET", {exact:true})).toBeVisible();
    await expect(page.getByText("Canonical relations")).toBeVisible();
  });

  test("offline state has no central fallback and reconnects", async ({page}) => {
    await page.route("http://127.0.0.1:42001/**", (route) => route.abort());
    await page.goto("/");
    await expect(page.getByText(/Cannot reach the configured ZION node/)).toBeVisible();
    await expect(page.getByText(/no central fallback/i)).toBeVisible();
    await page.unroute("http://127.0.0.1:42001/**");
    await page.getByRole("button",{name:"Retry"}).click();
    await expect(page.getByText("Connected",{exact:true})).toBeVisible();
  });
});
