"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import { DEFAULT_API_URL, EXPECTED_NETWORK, ZionAPI, ZionAPIError, type NodeHealth, type NodeStatus } from "@/lib/zion-api";

type ConnectionState = "connecting" | "connected" | "offline" | "unauthorized" | "error";
type NodeContextValue = {
  api: ZionAPI; apiURL: string; token: string; health?: NodeHealth; status?: NodeStatus;
  connection: ConnectionState; error?: string; stale: boolean; compatible: boolean; canMutate: boolean;
  retry(): void; saveSettings(url: string, token: string): void;
};

const NodeContext = createContext<NodeContextValue | null>(null);
const initialURL = process.env.NEXT_PUBLIC_ZION_API_URL ?? DEFAULT_API_URL;

export function NodeProvider({ children }: { children: React.ReactNode }) {
  const [apiURL, setAPIURL] = useState(initialURL);
  const [token, setToken] = useState("");
  const [health, setHealth] = useState<NodeHealth>();
  const [status, setStatus] = useState<NodeStatus>();
  const [connection, setConnection] = useState<ConnectionState>("connecting");
  const [error, setError] = useState<string>();
  const [generation, setGeneration] = useState(0);
  const mounted = useRef(false);

  useEffect(() => {
    const storedURL = window.localStorage.getItem("zion.apiURL");
    const sessionToken = window.sessionStorage.getItem("zion.bearerToken");
    if (storedURL) setAPIURL(storedURL);
    if (sessionToken) setToken(sessionToken);
    mounted.current = true;
  }, []);

  const api = useMemo(() => new ZionAPI(apiURL, token), [apiURL, token]);
  const refresh = useCallback(async (signal?: AbortSignal) => {
    try {
      const [nextHealth, nextStatus] = await Promise.all([api.health(signal), api.status(signal)]);
      setHealth(nextHealth); setStatus(nextStatus); setConnection("connected"); setError(undefined);
    } catch (cause) {
      if (cause instanceof ZionAPIError && cause.kind === "cancelled") return;
      setConnection(cause instanceof ZionAPIError && cause.status === 401 ? "unauthorized" : cause instanceof ZionAPIError && cause.kind === "offline" ? "offline" : "error");
      setError(cause instanceof Error ? cause.message : "Node connection failed");
    }
  }, [api]);

  useEffect(() => {
    const controller = new AbortController();
    setConnection((current) => current === "connected" ? current : "connecting");
    void refresh(controller.signal);
    const interval = window.setInterval(() => void refresh(controller.signal), 5_000);
    return () => { controller.abort(); window.clearInterval(interval); };
  }, [refresh, generation]);

  const saveSettings = useCallback((url: string, nextToken: string) => {
    const normalized = new ZionAPI(url).baseURL;
    window.localStorage.setItem("zion.apiURL", normalized);
    if (nextToken) window.sessionStorage.setItem("zion.bearerToken", nextToken); else window.sessionStorage.removeItem("zion.bearerToken");
    setAPIURL(normalized); setToken(nextToken); setGeneration((value) => value + 1);
  }, []);

  const compatible = status?.network_id === EXPECTED_NETWORK && status.protocol_version === "0.1";
  const stale = connection !== "connected" && status !== undefined;
  const canMutate = connection === "connected" && compatible && health?.ready === true && status?.sync_status === "SYNCED";
  const value = useMemo<NodeContextValue>(() => ({ api, apiURL, token, health, status, connection, error, stale, compatible, canMutate, retry: () => setGeneration((value) => value + 1), saveSettings }), [api, apiURL, token, health, status, connection, error, stale, compatible, canMutate, saveSettings]);
  return <NodeContext.Provider value={value}>{children}</NodeContext.Provider>;
}

export function useNode(): NodeContextValue {
  const value = useContext(NodeContext);
  if (!value) throw new Error("useNode must be used inside NodeProvider");
  return value;
}
