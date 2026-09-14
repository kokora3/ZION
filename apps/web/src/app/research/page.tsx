"use client";

import { useCallback, useState } from "react";
import { NodeData } from "@/components/node-data";
import { useNode } from "@/components/node-provider";
import { ResearchCard } from "@/components/registry-card";
import { useNodeData } from "@/components/use-node-data";
import { Badge, PageHeading } from "@/components/ui";

export default function ResearchPage() {
  const { api } = useNode(); const [query, setQuery] = useState(""); const [active, setActive] = useState("");
  const load = useCallback((signal: AbortSignal) => api.research(active, signal), [api, active]); const view = useNodeData(load, [load]);
  return <><PageHeading eyebrow="Canonical registry" title="Research"><p>Governance-admitted compact metadata. Search results are a <Badge>LOCAL DERIVED</Badge> projection of canonical entries.</p></PageHeading>
    <form className="search" role="search" onSubmit={(event) => { event.preventDefault(); setActive(query.trim()); }}><label className="sr-only" htmlFor="research-search">Search research</label><input id="research-search" maxLength={512} value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Title, summary, contributor, identifier" /><button>Local search</button></form>
    <NodeData loading={view.loading} error={view.error} empty={view.data?.length === 0} retry={view.reload}><div className="list">{view.data?.map((entry) => <ResearchCard key={entry.research_id} entry={entry} />)}</div></NodeData></>;
}
