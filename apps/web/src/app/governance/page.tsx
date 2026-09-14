"use client";

import { useCallback } from "react";
import { GovernanceCard } from "@/components/governance-card";
import { NodeData } from "@/components/node-data";
import { useNode } from "@/components/node-provider";
import { SignedSubmission } from "@/components/signed-submission";
import { useNodeData } from "@/components/use-node-data";
import { PageHeading } from "@/components/ui";

export default function GovernancePage() {
  const { api } = useNode(); const load = useCallback((signal: AbortSignal) => api.proposals(signal), [api]); const view = useNodeData(load, [load]);
  return <><PageHeading eyebrow="Canonical decisions" title="Governance"><p>The node is authoritative for eligibility, tallies, status, and execution. One ACTIVE IdentityID has one governance vote.</p></PageHeading>
    <NodeData loading={view.loading} error={view.error} empty={view.data?.length === 0} retry={view.reload}><div className="list">{view.data?.map((proposal) => <GovernanceCard key={proposal.proposal_id} proposal={proposal} />)}</div></NodeData>
    <section style={{marginTop:"2rem"}}><SignedSubmission kind="transaction" /><div className="card" style={{marginTop:"1rem"}}><h2>Vote or propose Research / Resource</h2><p>No safe local member signer exists in the current node runtime. Use the authoritative Go builders and local signing workflow to produce an already-signed canonical vote or proposal transaction, then submit it above. ZION Web never asks for a private key and never fakes proposal creation.</p><p>Transaction state can be <strong>accepted/submitted</strong> before it is <strong>finalized</strong>. Only the node reports finality.</p></div></section>
  </>;
}
