import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { NodeProvider, useNode } from "./node-provider";

const baseStatus = { network_id:"zion-alpha-1", genesis_id:"00", runtime_state:"RUNNING", roles:["NORMAL"], peer_id:"peer", sync_status:"SYNCED", accepted_height:7, state_hash:"state", consensus_active:false, validator_authorized:false, protocol_version:"0.1", software_version:"v0.1.0-alpha.1", object_store_enabled:true, object_count:0, object_bytes:0, object_quota_bytes:100, board_enabled:true, indexed_posts:0, indexed_replies:0, hidden_local:0, board_sync_state:"ACTIVE" };
function Probe() { const node = useNode(); return <div>{node.connection}|{String(node.compatible)}|{String(node.canMutate)}|{node.stale ? "stale" : "fresh"}</div>; }
function response(status = baseStatus, health = {live:true,ready:true}) { return vi.fn((url:string) => Promise.resolve(new Response(JSON.stringify(url.endsWith("/health") ? health : status), {status:200}))); }

afterEach(() => { vi.unstubAllGlobals(); window.localStorage.clear(); window.sessionStorage.clear(); });
describe("node connection states", () => {
  it("reports connected compatible readiness", async () => { vi.stubGlobal("fetch", response()); render(<NodeProvider><Probe /></NodeProvider>); expect(await screen.findByText("connected|true|true|fresh")).toBeVisible(); });
  it("keeps syncing distinct from ready", async () => { vi.stubGlobal("fetch", response({...baseStatus,sync_status:"SYNCING"},{live:true,ready:false})); render(<NodeProvider><Probe /></NodeProvider>); expect(await screen.findByText("connected|true|false|fresh")).toBeVisible(); });
  it("blocks mutations for a wrong network", async () => { vi.stubGlobal("fetch", response({...baseStatus,network_id:"another-network"})); render(<NodeProvider><Probe /></NodeProvider>); expect(await screen.findByText("connected|false|false|fresh")).toBeVisible(); });
  it("reports offline without central fallback", async () => { vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new TypeError("refused"))); render(<NodeProvider><Probe /></NodeProvider>); await waitFor(() => expect(screen.getByText(/^offline\|/)).toBeVisible()); });
});
