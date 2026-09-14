"use client";

import { useCallback, useEffect, useState } from "react";
import { ZionAPIError } from "@/lib/zion-api";
import { useNode } from "./node-provider";

export function useNodeData<T>(load: (signal: AbortSignal) => Promise<T>, dependencies: readonly unknown[] = []) {
  const { connection } = useNode();
  const [data, setData] = useState<T>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>();
  const [generation, setGeneration] = useState(0);
  const reload = useCallback(() => setGeneration((value) => value + 1), []);
  useEffect(() => {
    if (connection !== "connected") { setLoading(false); return; }
    const controller = new AbortController();
    setLoading(true); setError(undefined);
    void load(controller.signal).then(setData).catch((cause: unknown) => {
      if (!(cause instanceof ZionAPIError && cause.kind === "cancelled")) setError(cause instanceof Error ? cause.message : "Request failed");
    }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
    // load functions are deliberately stabilized by page components.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [connection, generation, ...dependencies]);
  return { data, loading, error, reload, setData };
}
