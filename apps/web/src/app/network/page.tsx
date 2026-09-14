"use client";

import Link from "next/link";
import { useCallback } from "react";
import { NodeData } from "@/components/node-data";
import { useNode } from "@/components/node-provider";
import { useNodeData } from "@/components/use-node-data";
import { Badge, Identifier, PageHeading } from "@/components/ui";

export default function NetworkPage() {
  const node = useNode(); const load = useCallback((signal: AbortSignal) => node.api.peers(signal), [node.api]); const view = useNodeData(load, [load]);
  return <><PageHeading eyebrow="General P2P · operational" title="Network & peers"><p>PeerID is transport identity; DNS/IP is location. Advertised roles are capability hints, not canonical authorization.</p></PageHeading>
    {node.status && <section className="card"><h2>This node</h2><Identifier value={node.status.peer_id || "P2P disabled"} label="PeerID" /><p>Advertised roles: {node.status.roles.join(" + ") || "none"}</p><p>Canonical validator authorization: <strong>{node.status.validator_authorized ? "AUTHORIZED" : "NOT AUTHORIZED"}</strong></p><div className="badges"><Badge>LOCAL ONLY</Badge><Badge>LOCAL DERIVED</Badge></div></section>}
    <section style={{marginTop:"1.5rem"}}><h2>Usable authenticated peers ({view.data?.length ?? 0})</h2><NodeData loading={view.loading} error={view.error} empty={view.data?.length === 0} retry={view.reload}><div className="list">{view.data?.map((peer) => <article className="card" key={peer.PeerID}><Identifier value={peer.PeerID} label="PeerID" /><p>Advertised roles: {peer.Roles?.join(" + ") || "none"} <span className="muted">(not validator authority)</span></p><details><summary>Contact addresses</summary><ul>{peer.Addresses?.map((address) => <li key={address}><code>{address}</code></li>)}</ul></details></article>)}</div></NodeData></section>
    <p><Link href="/identity">Inspect identity and membership</Link></p>
  </>;
}
