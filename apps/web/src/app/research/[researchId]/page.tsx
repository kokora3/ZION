"use client";

import { useCallback } from "react";
import { useParams } from "next/navigation";
import { NodeData } from "@/components/node-data";
import { useNode } from "@/components/node-provider";
import { useNodeData } from "@/components/use-node-data";
import { Badge, ExternalLink, Identifier, ObjectReferences, PageHeading } from "@/components/ui";

export default function ResearchDetailPage() {
  const id = decodeURIComponent(String(useParams<{ researchId: string }>().researchId)); const { api } = useNode();
  const load = useCallback((signal: AbortSignal) => api.researchByID(id, signal), [api, id]); const view = useNodeData(load, [load]); const entry = view.data;
  return <><PageHeading eyebrow="Canonical research" title={entry?.body.title || "Research detail"}><Identifier value={id} label="ResearchID" /></PageHeading><NodeData loading={view.loading} error={view.error} empty={!entry} retry={view.reload}>{entry && <>
    <section className="card"><div className="badges"><Badge>CANONICAL</Badge><Badge>FINALIZED</Badge></div><p>{entry.body.summary}</p><dl className="stat-list"><dt>Contributors</dt><dd>{entry.body.contributors.map((value) => value.display_name).join(", ")}</dd><dt>Identifiers</dt><dd>{entry.body.external_identifiers.map((value) => `${value.scheme}: ${value.value}`).join(", ") || "None"}</dd><dt>External location</dt><dd><ExternalLink href={entry.body.external_uri} /> <span className="muted">(mutable location, not immutable content)</span></dd><dt>Proposal</dt><dd><Identifier value={entry.proposal_id} label="ProposalID" /></dd><dt>Admission height</dt><dd>{entry.admitted_height}</dd></dl></section>
    <section><h2>Immutable content references</h2><ObjectReferences objects={entry.object_availability} /></section>
    <section><h2>Canonical relations</h2>{entry.body.canonical_refs.length ? entry.body.canonical_refs.map((reference) => <div className="card" key={`${reference.relation}:${reference.target_id}`}><Badge>CANONICAL</Badge><p>{reference.relation} → {reference.target_kind}</p><Identifier value={reference.target_id} label="canonical target" /></div>) : <p className="muted">No canonical relations. Board/community relations remain separate off-chain statements.</p>}</section>
  </>}</NodeData></>;
}
