import Link from "next/link";
import type { BoardPost } from "@/lib/zion-api";
import { Badge, Identifier } from "./ui";

export function BoardCard({ post }: { post: BoardPost }) {
  const current = post.authorization_status === "CURRENTLY_AUTHORIZED";
  return <article className="card">
    <div className="badges"><Badge>OFF-CHAIN SIGNED</Badge><Badge>{current ? "CURRENTLY AUTHORIZED" : "HISTORICAL / UNCONFIRMED"}</Badge><Badge>LOCAL DERIVED</Badge>{post.local_visibility === "LOCALLY_HIDDEN" && <Badge>LOCAL ONLY</Badge>}</div>
    <h2><Link href={`/board/${encodeURIComponent(post.post_id)}`}>{post.title || (post.kind === "REPLY" ? "Reply" : "Untitled post")}</Link></h2>
    <p>{post.local_visibility === "LOCALLY_HIDDEN" ? "Hidden on this node." : post.body?.slice(0, 280) || (post.content_present ? "Content has no preview." : "Content is missing locally.")}</p>
    <div className="stat-list"><dt>PostID</dt><dd><Identifier value={post.post_id} label="PostID" /></dd><dt>Author</dt><dd><Identifier value={post.author_identity} label="IdentityID" /></dd></div>
    <p className="muted">Membership: {post.author_current_membership} · Signature: {post.signature_status} · {post.content_present ? "content local" : "content missing"}</p>
  </article>;
}
