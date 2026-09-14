"use client";

import { useCallback } from "react";
import { useNode } from "@/components/node-provider";
import { useNodeData } from "@/components/use-node-data";
import { Badge, formatBytes, Identifier, PageHeading } from "@/components/ui";
import { NodeData } from "@/components/node-data";

export default function OverviewPage() {
  const node = useNode();
  const load = useCallback((signal: AbortSignal) => Promise.all([node.api.state(signal), node.api.peers(signal)]), [node.api]);
  const view = useNodeData(load, [load]);
  const status = node.status;
  return <>
    <PageHeading eyebrow="Local node" title="Network overview"><p>A direct view of this machine&apos;s ZION node. Canonical values come from the node; operational counters are local.</p></PageHeading>
    <NodeData loading={view.loading} error={view.error} empty={!status} retry={view.reload}>
      {status && <>
        <section className="grid" aria-label="Node summary">
          <article className="card"><div className="eyebrow">Network</div><div className="metric">{status.network_id}</div><Badge>{node.compatible ? "CANONICAL" : "INCOMPATIBLE"}</Badge></article>
          <article className="card"><div className="eyebrow">Readiness</div><div className="metric">{node.health?.ready ? "Ready" : status.sync_status === "SYNCING" ? "Syncing" : "Not ready"}</div><p>Process {node.health?.live ? "alive" : "not live"} · runtime {status.runtime_state}</p></article>
          <article className="card"><div className="eyebrow">Accepted height</div><div className="metric">{status.accepted_height.toLocaleString()}</div><Badge>{status.sync_status === "SYNCED" ? "FINALIZED" : "LOCAL ONLY"}</Badge></article>
          <article className="card"><div className="eyebrow">Connected peers</div><div className="metric">{view.data?.[1].length ?? 0}</div><Badge>LOCAL DERIVED</Badge></article>
          <article className="card"><div className="eyebrow">Research</div><div className="metric">{view.data?.[0].research_count ?? 0}</div><Badge>CANONICAL</Badge></article>
          <article className="card"><div className="eyebrow">Resources</div><div className="metric">{view.data?.[0].resource_count ?? 0}</div><Badge>CANONICAL</Badge></article>
        </section>
        <section className="grid" style={{marginTop:"1rem"}}>
          <article className="card"><h2>Canonical application</h2><dl className="stat-list"><dt>StateHash</dt><dd><Identifier value={status.state_hash} label="StateHash" /></dd><dt>Software</dt><dd>{status.software_version}</dd><dt>Protocol</dt><dd>{status.protocol_version}</dd><dt>Validator authority</dt><dd>{status.validator_authorized ? "Authorized by canonical/genesis state" : "Not authorized"}</dd><dt>Consensus process</dt><dd>{status.consensus_active ? "Active" : "Inactive"}</dd></dl></article>
          <article className="card"><h2>General P2P identity</h2><dl className="stat-list"><dt>PeerID</dt><dd>{status.peer_id ? <Identifier value={status.peer_id} label="PeerID" /> : "P2P disabled"}</dd><dt>Roles</dt><dd>{status.roles.join(" + ") || "None"}</dd><dt>Genesis</dt><dd><Identifier value={status.genesis_id} label="Genesis fingerprint" /></dd></dl><p className="muted">Advertised roles are capabilities; they do not grant validator authority.</p></article>
          <article className="card"><h2>Local object store</h2><dl className="stat-list"><dt>Objects</dt><dd>{status.object_count}</dd><dt>Used</dt><dd>{formatBytes(status.object_bytes)}</dd><dt>Quota</dt><dd>{formatBytes(status.object_quota_bytes)}</dd></dl><Badge>LOCAL ONLY</Badge></article>
          <article className="card"><h2>Local Board view</h2><dl className="stat-list"><dt>Posts</dt><dd>{status.indexed_posts}</dd><dt>Replies</dt><dd>{status.indexed_replies}</dd><dt>Hidden here</dt><dd>{status.hidden_local}</dd><dt>Sync</dt><dd>{status.board_sync_state}</dd></dl><Badge>LOCAL DERIVED</Badge></article>
        </section>
      </>}
    </NodeData>
  </>;
}
