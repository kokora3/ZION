import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { BoardCard } from "./board-card";
import { GovernanceCard } from "./governance-card";
import { Badge, ExternalLink, Identifier } from "./ui";
import type { BoardPost, GovernanceProposal } from "@/lib/zion-api";

afterEach(() => vi.unstubAllGlobals());
const post: BoardPost = { post_id:"zion" + "x".repeat(120), kind:"POST", author_identity:"zion:id:sha256:" + "a".repeat(64), author_key_id:"zion:key:sha256:" + "b".repeat(64), created_at:1, content_object:"zion:obj:sha256:" + "c".repeat(64), references:[], content_present:true, signature_status:"SIGNATURE_VALID", authorization_status:"CURRENTLY_AUTHORIZED", author_current_membership:"ACTIVE", local_visibility:"VISIBLE", parent_present:true, title:"Safe plain text", body:"<img src=x onerror=alert(1)>" };
const proposal: GovernanceProposal = { proposal_id:"zion:proposal:sha256:"+"d".repeat(64), kind:"RESEARCH_ADMISSION", proposer:post.author_identity, status:"APPROVED", start_height:2, end_height:12, electorate_count:4, yes:3, no:0, abstain:0, participants:3, votes:[] };

describe("information authority UI", () => {
  it("renders distinct canonical and local labels", () => { render(<><Badge>CANONICAL</Badge><Badge>OFF-CHAIN SIGNED</Badge><Badge>LOCAL DERIVED</Badge></>); expect(screen.getByText("CANONICAL")).toBeVisible(); expect(screen.getByText("OFF-CHAIN SIGNED")).toBeVisible(); });
  it("renders Board HTML-like input only as text with trust labels", () => { const { container } = render(<BoardCard post={post} />); expect(screen.getByText("OFF-CHAIN SIGNED")).toBeVisible(); expect(screen.getByText(/<img src=x/)).toBeVisible(); expect(container.querySelector("img")).toBeNull(); });
  it("marks hidden content as local rather than deletion", () => { render(<BoardCard post={{...post,local_visibility:"LOCALLY_HIDDEN"}} />); expect(screen.getByText("Hidden on this node.")).toBeVisible(); expect(screen.getByText("LOCAL ONLY")).toBeVisible(); });
  it("keeps APPROVED distinct from EXECUTED", () => { render(<GovernanceCard proposal={proposal} />); expect(screen.getByText(/Approved, not executed/)).toBeVisible(); expect(screen.queryByText("FINALIZED")).toBeNull(); });
  it("blocks unsafe external schemes", () => { render(<ExternalLink href="javascript:alert(1)" />); expect(screen.getByText(/Blocked non-HTTPS URI/)).toBeVisible(); expect(screen.queryByRole("link")).toBeNull(); });
  it("supports long namespaced identifiers without discarding the full value", () => { render(<Identifier value={post.post_id} label="PostID" />); expect(screen.getByLabelText(`PostID: ${post.post_id}`)).toHaveTextContent(post.post_id); expect(screen.getByRole("button", {name:"Copy PostID"})).toBeVisible(); });
});
