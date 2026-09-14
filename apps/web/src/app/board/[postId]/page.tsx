"use client";

import { useCallback, useState } from "react";
import { useParams } from "next/navigation";
import { BoardCard } from "@/components/board-card";
import { NodeData } from "@/components/node-data";
import { useNode } from "@/components/node-provider";
import { useNodeData } from "@/components/use-node-data";
import { Badge, Identifier, PageHeading } from "@/components/ui";

export default function BoardThreadPage() {
  const id = decodeURIComponent(String(useParams<{ postId: string }>().postId));
  const { api, canMutate } = useNode();
  const [visibility, setVisibility] = useState<string>();
  const load = useCallback((signal: AbortSignal) => Promise.all([api.boardPost(id, signal), api.boardReplies(id, signal)]), [api, id]);
  const view = useNodeData(load, [load]);
  const post = view.data?.[0];
  const hidden = (visibility ?? post?.local_visibility) === "LOCALLY_HIDDEN";
  const toggle = async () => { const result = await api.setBoardHidden(id, !hidden); setVisibility(result.local_visibility); view.reload(); };
  return <>
    <PageHeading eyebrow="Board thread" title={post?.title || "Signed discussion"}><Identifier value={id} label="PostID" /></PageHeading>
    <NodeData loading={view.loading} error={view.error} empty={!post} retry={view.reload}>{post && <>
      <article className="card"><div className="badges"><Badge>OFF-CHAIN SIGNED</Badge><Badge>{post.authorization_status === "CURRENTLY_AUTHORIZED" ? "CURRENTLY AUTHORIZED" : "HISTORICAL / UNCONFIRMED"}</Badge></div><div className="markdown">{hidden ? "Hidden on this node." : post.body || "Content unavailable locally."}</div><hr /><p>Author <Identifier value={post.author_identity} label="IdentityID" /></p><button disabled={!canMutate} onClick={() => void toggle()}>{hidden ? "Unhide on this node" : "Hide on this node"}</button><p className="muted">This is local visibility only. It is not global deletion and does not change StateHash.</p></article>
      <section style={{marginTop:"1.5rem"}}><h2>Replies ({view.data?.[1].length ?? 0})</h2>{view.data?.[1].length ? <div className="list">{view.data[1].map((reply) => <BoardCard key={reply.post_id} post={reply} />)}</div> : <div className="empty-state">No replies are available on this node.</div>}</section>
    </>}</NodeData>
  </>;
}
