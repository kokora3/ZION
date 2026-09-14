"use client";

import Link from "next/link";
import { useNode } from "./node-provider";

export function AsyncState({ loading, error, empty, onRetry, children }: { loading: boolean; error?: string; empty?: boolean; onRetry(): void; children: React.ReactNode }) {
  const node = useNode();
  if (node.connection !== "connected") return <section className="empty-state" role="status"><h2>Local node {node.connection}</h2><p>{node.error ?? `Cannot reach ${node.apiURL}`}</p>{node.stale && <p>Previously loaded data is stale and must not be treated as current finality.</p>}<div className="actions"><button onClick={node.retry}>Retry</button><Link href="/settings">Settings</Link></div><code>zion-node run --config &lt;zion.yaml&gt;</code></section>;
  if (loading) return <section className="empty-state" role="status"><span className="spinner" /> Loading from your local node…</section>;
  if (error) return <section className="empty-state" role="alert"><h2>Request failed</h2><p>{error}</p><button onClick={onRetry}>Retry</button></section>;
  if (empty) return <section className="empty-state"><h2>No local results</h2><p>This node has no matching data in its current local view.</p></section>;
  return <>{children}</>;
}
