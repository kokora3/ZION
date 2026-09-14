"use client";

import { useCallback } from "react";
import { useParams } from "next/navigation";
import { NodeData } from "@/components/node-data";
import { useNode } from "@/components/node-provider";
import { useNodeData } from "@/components/use-node-data";
import { Badge, Identifier, PageHeading } from "@/components/ui";

export default function TransactionPage() {
  const id = decodeURIComponent(String(useParams<{ txId: string }>().txId)); const { api } = useNode(); const load = useCallback((signal: AbortSignal) => api.transaction(id, signal), [api, id]); const view = useNodeData(load, [load]); const tx = view.data;
  return <><PageHeading eyebrow="Node-observed transaction" title="Transaction status"><Identifier value={id} label="TxID" /></PageHeading><NodeData loading={view.loading} error={view.error} empty={!tx} retry={view.reload}>{tx && <section className="card"><div className="badges"><Badge>{tx.status === "FINALIZED" ? "FINALIZED" : "SUBMITTED"}</Badge>{tx.status === "FINALIZED" && <Badge>CANONICAL</Badge>}</div><h2>{tx.status}</h2><p>{tx.status === "FINALIZED" ? `Committed at height ${tx.committed_height}` : "Accepted or relayed is not finality. Refresh after the node observes a committed result."}</p>{tx.state_hash && <Identifier value={tx.state_hash} label="StateHash" />}<button onClick={view.reload}>Refresh status</button></section>}</NodeData></>;
}
