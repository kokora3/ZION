"use client";

import { useCallback, useState } from "react";
import { useParams } from "next/navigation";
import { ZionAPIError, type ObjectMeta } from "@/lib/zion-api";
import { NodeData } from "@/components/node-data";
import { useNode } from "@/components/node-provider";
import { useNodeData } from "@/components/use-node-data";
import { Badge, formatBytes, Identifier, PageHeading } from "@/components/ui";

export default function ObjectPage() {
  const id = decodeURIComponent(String(useParams<{ objectId: string }>().objectId)); const { api, canMutate } = useNode(); const [action, setAction] = useState("");
  const load = useCallback(async (signal: AbortSignal): Promise<ObjectMeta> => { try { return await api.objectMeta(id, signal); } catch (cause) { if (cause instanceof ZionAPIError && cause.status === 404) return { object_id:id, size:0, present:false }; throw cause; } }, [api, id]);
  const view = useNodeData(load, [load]);
  const fetchMissing = async () => { setAction("Fetching from bounded authenticated peers…"); try { const value = await api.fetchObject(id); setAction(value.status); view.reload(); } catch (cause) { setAction(cause instanceof Error ? cause.message : "Fetch failed"); } };
  const download = async () => { setAction("Reading verified local bytes…"); try { const value = await api.getObject(id); const bytes = Uint8Array.from(atob(value.object), (character) => character.charCodeAt(0)); const url = URL.createObjectURL(new Blob([bytes], {type:"application/cbor"})); const anchor = document.createElement("a"); anchor.href=url; anchor.download=`${id.replaceAll(":","_")}.cbor`; anchor.click(); URL.revokeObjectURL(url); setAction("Downloaded verified canonical bytes"); } catch (cause) { setAction(cause instanceof Error ? cause.message : "Download failed"); } };
  return <><PageHeading eyebrow="Content-addressed object" title="Object detail"><Identifier value={id} label="ObjectID" /></PageHeading><NodeData loading={view.loading} error={view.error} empty={!view.data} retry={view.reload}>{view.data && <section className="card"><div className="badges"><Badge>CONTENT-ADDRESSED</Badge><Badge>{view.data.present ? "AVAILABLE LOCALLY" : "MISSING LOCALLY"}</Badge><Badge>LOCAL ONLY</Badge></div><dl className="stat-list"><dt>ObjectID</dt><dd><Identifier value={id} label="ObjectID" /></dd><dt>Local size</dt><dd>{view.data.present ? formatBytes(view.data.size) : "Not available"}</dd><dt>Stored locally</dt><dd>{view.data.stored_at || "—"}</dd></dl><p>ZION verifies canonical bytes and ObjectID on storage and retrieval. Content addressing does not prove safety, endorsement, or permanence.</p><div className="actions">{view.data.present ? <button onClick={() => void download()}>Download canonical bytes</button> : <button disabled={!canMutate} onClick={() => void fetchMissing()}>Fetch from peers</button>}</div>{action && <p role="status">{action}</p>}</section>}</NodeData></>;
}
