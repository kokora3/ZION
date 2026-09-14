import type {
  BoardPost, GovernanceProposal, IdentityView, MembershipView, NodeHealth, NodeStatus,
  ObjectMeta, ObjectResult, Peer, ResearchEntry, ResourceEntry, StateSummary, Submission, TransactionStatus,
} from "./types";

export const EXPECTED_NETWORK = "zion-alpha-1";
export const DEFAULT_API_URL = "http://127.0.0.1:42001";
export const MAX_PAGE_SIZE = 100;

export type APIErrorKind = "http" | "offline" | "timeout" | "cancelled" | "malformed";

export class ZionAPIError extends Error {
  constructor(
    message: string,
    public readonly kind: APIErrorKind,
    public readonly status?: number,
    public readonly code?: string,
  ) { super(message); this.name = "ZionAPIError"; }
}

type RequestOptions = { method?: "GET" | "POST"; body?: unknown; signal?: AbortSignal; timeoutMs?: number };

function record(value: unknown): Record<string, unknown> {
  if (value === null || typeof value !== "object" || Array.isArray(value)) throw new ZionAPIError("Node returned malformed JSON", "malformed");
  return value as Record<string, unknown>;
}
function array(value: unknown): unknown[] {
  if (!Array.isArray(value)) throw new ZionAPIError("Node returned malformed JSON", "malformed");
  return value;
}
function required<T extends "string" | "number" | "boolean">(source: Record<string, unknown>, key: string, type: T): T extends "string" ? string : T extends "number" ? number : boolean {
  const value = source[key];
  if (typeof value !== type || (type === "number" && !Number.isFinite(value))) throw new ZionAPIError(`Node response is missing ${key}`, "malformed");
  return value as never;
}

export function normalizeBaseURL(value: string): string {
  const parsed = new URL(value.trim());
  if (parsed.protocol !== "http:" && parsed.protocol !== "https:") throw new Error("Node API URL must use http or https");
  if (parsed.username || parsed.password || parsed.search || parsed.hash) throw new Error("Node API URL cannot contain credentials, query, or fragment");
  return parsed.toString().replace(/\/$/, "");
}

export class ZionAPI {
  readonly baseURL: string;
  constructor(baseURL = DEFAULT_API_URL, private readonly token = "", private readonly defaultTimeoutMs = 8_000) {
    this.baseURL = normalizeBaseURL(baseURL);
  }

  private async request(path: string, options: RequestOptions = {}): Promise<unknown> {
    const controller = new AbortController();
    const timeout = window.setTimeout(() => controller.abort("timeout"), options.timeoutMs ?? this.defaultTimeoutMs);
    const onAbort = () => controller.abort("cancelled");
    options.signal?.addEventListener("abort", onAbort, { once: true });
    try {
      const headers: Record<string, string> = { Accept: "application/json" };
      if (options.body !== undefined) headers["Content-Type"] = "application/json";
      if (this.token) headers.Authorization = `Bearer ${this.token}`;
      const response = await fetch(`${this.baseURL}/v1${path}`, {
        method: options.method ?? "GET", headers, body: options.body === undefined ? undefined : JSON.stringify(options.body),
        signal: controller.signal, cache: "no-store",
      });
      let payload: unknown;
      try { payload = await response.json(); } catch { throw new ZionAPIError("Node returned malformed JSON", "malformed", response.status); }
      if (!response.ok) {
        const detail = record(payload);
        throw new ZionAPIError(typeof detail.message === "string" ? detail.message : `Node request failed (${response.status})`, "http", response.status, typeof detail.code === "string" ? detail.code : undefined);
      }
      return payload;
    } catch (error) {
      if (error instanceof ZionAPIError) throw error;
      if (controller.signal.aborted) {
        const cancelled = options.signal?.aborted || controller.signal.reason === "cancelled";
        throw new ZionAPIError(cancelled ? "Request cancelled" : "Node request timed out", cancelled ? "cancelled" : "timeout");
      }
      throw new ZionAPIError("Cannot reach the configured ZION node", "offline");
    } finally {
      window.clearTimeout(timeout);
      options.signal?.removeEventListener("abort", onAbort);
    }
  }

  async health(signal?: AbortSignal): Promise<NodeHealth> {
    const value = record(await this.request("/health", { signal }));
    return { live: required(value, "live", "boolean"), ready: required(value, "ready", "boolean") };
  }
  async status(signal?: AbortSignal): Promise<NodeStatus> {
    const value = record(await this.request("/status", { signal }));
    for (const key of ["network_id", "runtime_state", "sync_status", "state_hash", "peer_id", "protocol_version"] as const) required(value, key, "string");
    required(value, "accepted_height", "number");
    if (!Array.isArray(value.roles)) throw new ZionAPIError("Node response is missing roles", "malformed");
    return value as NodeStatus;
  }
  async state(signal?: AbortSignal): Promise<StateSummary> { return record(await this.request("/state", { signal })) as StateSummary; }
  async peers(signal?: AbortSignal): Promise<Peer[]> { return array(await this.request("/peers", { signal })) as Peer[]; }
  async boardPosts(offset = 0, limit = 20, signal?: AbortSignal): Promise<BoardPost[]> { return array(await this.request(`/board/posts?offset=${offset}&limit=${limit}`, { signal })) as BoardPost[]; }
  async boardPost(id: string, signal?: AbortSignal): Promise<BoardPost> { return record(await this.request(`/board/posts/${encodeURIComponent(id)}`, { signal })) as BoardPost; }
  async boardReplies(id: string, signal?: AbortSignal): Promise<BoardPost[]> { return array(await this.request(`/board/posts/${encodeURIComponent(id)}/replies?offset=0&limit=100`, { signal })) as BoardPost[]; }
  async boardSearch(query: string, signal?: AbortSignal): Promise<BoardPost[]> { return array(await this.request(`/board/search?q=${encodeURIComponent(query)}&offset=0&limit=50`, { signal })) as BoardPost[]; }
  async setBoardHidden(id: string, hidden: boolean): Promise<{ post_id: string; local_visibility: string }> { return record(await this.request(`/board/posts/${encodeURIComponent(id)}/${hidden ? "hide" : "unhide"}`, { method: "POST" })) as { post_id: string; local_visibility: string }; }
  async submitBoardEvent(base64: string): Promise<BoardPost> { return record(await this.request("/board/events", { method: "POST", body: { encoding: "base64", event: base64 } })) as BoardPost; }
  async research(query = "", signal?: AbortSignal): Promise<ResearchEntry[]> { return array(await this.request(query ? `/research/search?q=${encodeURIComponent(query)}&offset=0&limit=50` : "/research?offset=0&limit=50", { signal })) as ResearchEntry[]; }
  async researchByID(id: string, signal?: AbortSignal): Promise<ResearchEntry> { return record(await this.request(`/research/${encodeURIComponent(id)}`, { signal })) as ResearchEntry; }
  async resources(query = "", signal?: AbortSignal): Promise<ResourceEntry[]> { return array(await this.request(query ? `/resources/search?q=${encodeURIComponent(query)}&offset=0&limit=50` : "/resources?offset=0&limit=50", { signal })) as ResourceEntry[]; }
  async resourceByID(id: string, signal?: AbortSignal): Promise<ResourceEntry> { return record(await this.request(`/resources/${encodeURIComponent(id)}`, { signal })) as ResourceEntry; }
  async proposals(signal?: AbortSignal): Promise<GovernanceProposal[]> { return array(await this.request("/governance/proposals?offset=0&limit=100", { signal })) as GovernanceProposal[]; }
  async proposal(id: string, signal?: AbortSignal): Promise<GovernanceProposal> { return record(await this.request(`/governance/proposals/${encodeURIComponent(id)}`, { signal })) as GovernanceProposal; }
  async identity(id: string, signal?: AbortSignal): Promise<IdentityView> { return record(await this.request(`/identities/${encodeURIComponent(id)}`, { signal })) as IdentityView; }
  async membership(id: string, signal?: AbortSignal): Promise<MembershipView> { return record(await this.request(`/memberships/${encodeURIComponent(id)}`, { signal })) as MembershipView; }
  async objectMeta(id: string, signal?: AbortSignal): Promise<ObjectMeta> { return record(await this.request(`/objects/${encodeURIComponent(id)}/meta`, { signal })) as ObjectMeta; }
  async fetchObject(id: string): Promise<ObjectResult> { return record(await this.request(`/objects/${encodeURIComponent(id)}/fetch`, { method: "POST", timeoutMs: 20_000 })) as ObjectResult; }
  async getObject(id: string): Promise<{ encoding: string; object: string; object_id: string; size: number }> { return record(await this.request(`/objects/${encodeURIComponent(id)}`)) as { encoding: string; object: string; object_id: string; size: number }; }
  async putObject(base64: string): Promise<ObjectResult> { return record(await this.request("/objects", { method: "POST", body: { encoding: "base64", object: base64 }, timeoutMs: 20_000 })) as ObjectResult; }
  async submitTransaction(base64: string): Promise<Submission> { return record(await this.request("/transactions", { method: "POST", body: { encoding: "base64", transaction: base64 } })) as Submission; }
  async transaction(id: string, signal?: AbortSignal): Promise<TransactionStatus> { return record(await this.request(`/transactions/${encodeURIComponent(id)}`, { signal })) as TransactionStatus; }
}
