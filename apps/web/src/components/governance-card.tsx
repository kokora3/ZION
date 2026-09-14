import Link from "next/link";
import type { GovernanceProposal } from "@/lib/zion-api";
import { Badge, Identifier } from "./ui";

export function GovernanceCard({ proposal }: { proposal: GovernanceProposal }) {
  return <article className="card"><div className="badges"><Badge>CANONICAL</Badge><span className="badge">{proposal.status}</span>{proposal.status === "EXECUTED" && <Badge>FINALIZED</Badge>}</div>
    <h2><Link href={`/governance/${encodeURIComponent(proposal.proposal_id)}`}>{proposal.kind.replaceAll("_", " ")}</Link></h2>
    <Identifier value={proposal.proposal_id} label="ProposalID" />
    <p>YES {proposal.yes} · NO {proposal.no} · ABSTAIN {proposal.abstain} · participation {proposal.participants}/{proposal.electorate_count}</p>
    {proposal.status === "APPROVED" && <p className="warning"><strong>Approved, not executed.</strong> Approval alone has not applied the proposal.</p>}
    <p className="muted">Height {proposal.start_height}–{proposal.end_height}</p>
  </article>;
}
