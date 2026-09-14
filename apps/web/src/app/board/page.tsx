"use client";

import { useCallback, useState } from "react";
import { BoardCard } from "@/components/board-card";
import { NodeData } from "@/components/node-data";
import { useNode } from "@/components/node-provider";
import { SignedSubmission } from "@/components/signed-submission";
import { useNodeData } from "@/components/use-node-data";
import { PageHeading } from "@/components/ui";

export default function BoardPage() {
  const { api } = useNode();
  const [query, setQuery] = useState("");
  const [activeQuery, setActiveQuery] = useState("");
  const load = useCallback((signal: AbortSignal) => activeQuery ? api.boardSearch(activeQuery, signal) : api.boardPosts(0, 30, signal), [api, activeQuery]);
  const view = useNodeData(load, [load]);
  return <>
    <PageHeading eyebrow="Off-chain signed · local derived feed" title="Community Board"><p>Signed public discussion discovered by this node. The feed is not a globally ordered chain.</p></PageHeading>
    <form className="search" role="search" onSubmit={(event) => { event.preventDefault(); setActiveQuery(query.trim()); }}><label className="sr-only" htmlFor="board-search">Search this node</label><input id="board-search" maxLength={512} value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search this node’s Board index" /><button>Local search</button></form>
    <NodeData loading={view.loading} error={view.error} empty={view.data?.length === 0} retry={view.reload}><div className="list">{view.data?.map((post) => <BoardCard key={post.post_id} post={post} />)}</div></NodeData>
    <div style={{marginTop:"2rem"}}><SignedSubmission kind="board" /></div>
  </>;
}
