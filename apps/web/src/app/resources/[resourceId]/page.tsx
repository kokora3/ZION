"use client";

import { useCallback } from "react";
import { useParams } from "next/navigation";
import { NodeData } from "@/components/node-data";
import { useNode } from "@/components/node-provider";
import { useNodeData } from "@/components/use-node-data";
import { Badge, ExternalLink, Identifier, ObjectReferences, PageHeading } from "@/components/ui";

export default function ResourceDetailPage() {
  const id = decodeURIComponent(String(useParams<{ resourceId: string }>().resourceId)); const { api } = useNode();
  const load = useCallback((signal: AbortSignal) => api.resourceByID(id, signal), [api, id]); const view = useNodeData(load, [load]); const entry = view.data;
  return <><PageHeading eyebrow="Canonical resource" title={entry?.body.name || "Resource detail"}><Identifier value={id} label="ResourceID" /></PageHeading><NodeData loading={view.loading} error={view.error} empty={!entry} retry={view.reload}>{entry && <>
    <section className="card"><div className="badges"><Badge>CANONICAL</Badge><span className="badge">{entry.body.kind}</span></div><p>{entry.body.summary}</p><p className="warning">Registry admission is curation, not proof that software is safe. ZION Web will not run or install this resource.</p><dl className="stat-list"><dt>License</dt><dd>{entry.body.license || "Not specified"}</dd><dt>Maintainers</dt><dd>{entry.body.maintainers.map((value) => value.display_name).join(", ") || "Not specified"}</dd><dt>External location</dt><dd><ExternalLink href={entry.body.external_uri} /> <span className="muted">(mutable location)</span></dd><dt>Proposal</dt><dd><Identifier value={entry.proposal_id} label="ProposalID" /></dd><dt>Admission height</dt><dd>{entry.admitted_height}</dd></dl></section>
    <section><h2>Immutable content references</h2><ObjectReferences objects={entry.object_availability} /></section>
    <section><h2>Canonical relations</h2>{entry.body.canonical_refs.length ? entry.body.canonical_refs.map((reference) => <div className="card" key={`${reference.relation}:${reference.target_id}`}><Badge>CANONICAL</Badge><p>{reference.relation} → {reference.target_kind}</p><Identifier value={reference.target_id} label="canonical target" /></div>) : <p className="muted">No canonical relations.</p>}</section>
  </>}</NodeData></>;
}
