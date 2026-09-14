import Link from "next/link";
import type { ResearchEntry, ResourceEntry } from "@/lib/zion-api";
import { Badge, Identifier } from "./ui";

export function ResearchCard({ entry }: { entry: ResearchEntry }) {
  return <article className="card"><div className="badges"><Badge>CANONICAL</Badge><Badge>FINALIZED</Badge></div><h2><Link href={`/research/${encodeURIComponent(entry.research_id)}`}>{entry.body.title}</Link></h2><p>{entry.body.summary}</p><Identifier value={entry.research_id} label="ResearchID" /><p className="muted">Admitted at height {entry.admitted_height} · {entry.object_availability.length} object reference(s)</p></article>;
}
export function ResourceCard({ entry }: { entry: ResourceEntry }) {
  return <article className="card"><div className="badges"><Badge>CANONICAL</Badge><span className="badge">{entry.body.kind}</span></div><h2><Link href={`/resources/${encodeURIComponent(entry.resource_id)}`}>{entry.body.name}</Link></h2><p>{entry.body.summary}</p><Identifier value={entry.resource_id} label="ResourceID" /><p className="muted">Admitted at height {entry.admitted_height} · {entry.object_availability.length} object reference(s)</p></article>;
}
