"use client";

import Link from "next/link";
import { useNode } from "./node-provider";

export function NodeData({ loading, error, empty, retry, children }: { loading: boolean; error?: string; empty?: boolean; retry(): void; children: React.ReactNode }) {
  const node = useNode();
  if (node.connection !== "connected") return <section className="empty-state" role="status"><h2>{node.connection === "unauthorized" ? "Authentication required" : "Local node offline"}</h2><p>{node.error ?? "Cannot connect to zion-node."}</p><p><code>{node.apiURL}</code></p><div className="actions" style={{justifyContent:"center"}}><button onClick={node.retry}>Retry</button><Link className="button" href="/settings">Settings</Link></div><p className="muted">Start the local service with <code>zion-node run --config &lt;zion.yaml&gt;</code>. No central fallback is used.</p></section>;
  if (loading) return <section className="empty-state" role="status"><span className="status-dot" /> Loading from local node…</section>;
  if (error) return <section className="empty-state" role="alert"><h2>Request failed</h2><p>{error}</p><button onClick={retry}>Retry</button></section>;
  if (empty) return <section className="empty-state"><h2>No local results</h2><p>This node has no matching data in its current view.</p></section>;
  return <>{children}</>;
}
