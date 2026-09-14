import { afterEach, describe, expect, it, vi } from "vitest";
import { ZionAPI, normalizeBaseURL } from "./client";

const status = { network_id:"zion-alpha-1", genesis_id:"00", runtime_state:"RUNNING", roles:["NORMAL"], peer_id:"peer", sync_status:"SYNCED", accepted_height:7, state_hash:"zion:state:sha256:x", consensus_active:false, validator_authorized:false, protocol_version:"0.1", software_version:"v0.1.0-alpha.1", object_store_enabled:true, object_count:0, object_bytes:0, object_quota_bytes:100, board_enabled:true, indexed_posts:0, indexed_replies:0, hidden_local:0, board_sync_state:"ACTIVE" };

afterEach(() => vi.unstubAllGlobals());
describe("ZionAPI", () => {
  it("normalizes a safe base URL", () => expect(normalizeBaseURL("http://127.0.0.1:42001/")).toBe("http://127.0.0.1:42001"));
  it("rejects URL credentials", () => expect(() => normalizeBaseURL("http://secret@localhost:42001")).toThrow(/credentials/));
  it("decodes a successful typed status", async () => { vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify(status), {status:200}))); expect((await new ZionAPI().status()).accepted_height).toBe(7); });
  it.each([[401,"UNAUTHORIZED"],[404,"NOT_FOUND"]])("normalizes HTTP %i", async (code, stable) => { vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({code:stable,message:"no"}), {status:code}))); await expect(new ZionAPI().status()).rejects.toMatchObject({kind:"http",status:code,code:stable}); });
  it("rejects malformed responses", async () => { vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("not json", {status:200}))); await expect(new ZionAPI().status()).rejects.toMatchObject({kind:"malformed"}); });
  it("normalizes connection refusal", async () => { vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new TypeError("refused"))); await expect(new ZionAPI().status()).rejects.toMatchObject({kind:"offline"}); });
  it("times out bounded requests", async () => {
    vi.useFakeTimers();
    vi.stubGlobal("fetch", vi.fn((_url, init?:RequestInit) => new Promise((_resolve, reject) => {
      init?.signal?.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError")));
    })));
    const promise = new ZionAPI(undefined, "", 5).status();
    const assertion = expect(promise).rejects.toMatchObject({kind:"timeout"});
    await vi.advanceTimersByTimeAsync(6);
    await assertion;
    vi.useRealTimers();
  });
  it("honors caller cancellation", async () => {
    vi.stubGlobal("fetch", vi.fn((_url, init?:RequestInit) => new Promise((_resolve, reject) => {
      init?.signal?.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError")));
    })));
    const controller = new AbortController(); const promise = new ZionAPI().status(controller.signal); controller.abort();
    await expect(promise).rejects.toMatchObject({kind:"cancelled"});
  });
  it("never places bearer tokens in URLs", async () => { const fetcher = vi.fn().mockResolvedValue(new Response(JSON.stringify({live:true,ready:true}), {status:200})); vi.stubGlobal("fetch", fetcher); await new ZionAPI("http://localhost:42001", "top-secret").health(); const [url, init] = fetcher.mock.calls[0] as [string, RequestInit]; expect(url).not.toContain("top-secret"); expect((init.headers as Record<string,string>).Authorization).toBe("Bearer top-secret"); });
});
