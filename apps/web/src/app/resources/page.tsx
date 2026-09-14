"use client";

import { useCallback, useState } from "react";
import { NodeData } from "@/components/node-data";
import { useNode } from "@/components/node-provider";
import { ResourceCard } from "@/components/registry-card";
import { useNodeData } from "@/components/use-node-data";
import { Badge, PageHeading } from "@/components/ui";

export default function ResourcesPage() {
  const { api } = useNode(); const [query, setQuery] = useState(""); const [active, setActive] = useState("");
  const load = useCallback((signal: AbortSignal) => api.resources(active, signal), [api, active]); const view = useNodeData(load, [load]);
  return <><PageHeading eyebrow="Canonical registry" title="Resources"><p>Governance-admitted metadata, not an execution or malware-safety guarantee. Search is <Badge>LOCAL DERIVED</Badge>.</p></PageHeading>
    <form className="search" role="search" onSubmit={(event) => { event.preventDefault(); setActive(query.trim()); }}><label className="sr-only" htmlFor="resource-search">Search resources</label><input id="resource-search" maxLength={512} value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Name, summary, license, maintainer" /><button>Local search</button></form>
    <NodeData loading={view.loading} error={view.error} empty={view.data?.length === 0} retry={view.reload}><div className="list">{view.data?.map((entry) => <ResourceCard key={entry.resource_id} entry={entry} />)}</div></NodeData></>;
}
