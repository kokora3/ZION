"use client";

import { useCallback } from "react";
import { useParams } from "next/navigation";
import { NodeData } from "@/components/node-data";
import { useNode } from "@/components/node-provider";
import { SignedSubmission } from "@/components/signed-submission";
import { useNodeData } from "@/components/use-node-data";
import { Badge, Identifier, PageHeading } from "@/components/ui";

export default function GovernanceDetailPage() {
  const id = decodeURIComponent(String(useParams<{ proposalId: string }>().proposalId)); const { api } = useNode();
  const load = useCallback((signal: AbortSignal) => api.proposal(id, signal), [api, id]); const view = useNodeData(load, [load]); const proposal = view.data;
  return <><PageHeading eyebrow="Canonical proposal" title={proposal?.kind.replaceAll("_", " ") || "Proposal detail"}><Identifier value={id} label="ProposalID" /></PageHeading><NodeData loading={view.loading} error={view.error} empty={!proposal} retry={view.reload}>{proposal && <>
    <section className="grid"><article className="card"><div className="badges"><Badge>CANONICAL</Badge><span className="badge">{proposal.status}</span></div><div className="metric">{proposal.status}</div>{proposal.status === "APPROVED" && <p className="warning"><strong>APPROVED is not EXECUTED.</strong> Canonical effects are still pending a valid execution transaction.</p>}<dl className="stat-list"><dt>Proposer</dt><dd><Identifier value={proposal.proposer} label="IdentityID" /></dd><dt>Voting heights</dt><dd>{proposal.start_height}–{proposal.end_height}</dd><dt>Electorate</dt><dd>{proposal.electorate_count}</dd></dl></article>
    <article className="card"><h2>Authoritative tally</h2><div className="metric">{proposal.participants}/{proposal.electorate_count}</div><p>YES {proposal.yes} · NO {proposal.no} · ABSTAIN {proposal.abstain}</p><p className="muted">The browser does not recalculate approval authority.</p></article></section>
    <section><h2>Votes</h2>{proposal.votes.length ? <div className="list">{proposal.votes.map((vote) => <div className="card" key={vote.voter}><strong>{vote.choice}</strong><Identifier value={vote.voter} label="voter IdentityID" /><Identifier value={vote.key_id} label="KeyID" /></div>)}</div> : <div className="empty-state">No votes recorded.</div>}</section>
    <section style={{marginTop:"1.5rem"}}><SignedSubmission kind="transaction" /></section>
  </>}</NodeData></>;
}
