"use client";

import Link from "next/link";
import type { ObjectAvailability } from "@/lib/zion-api";
import { useNode } from "./node-provider";

export function PageHeading({ eyebrow, title, children }: { eyebrow: string; title: string; children?: React.ReactNode }) {
  return <header className="page-heading"><div><div className="eyebrow">{eyebrow}</div><h1>{title}</h1>{children}</div></header>;
}

export type BadgeLabel = "CANONICAL" | "OFF-CHAIN SIGNED" | "LOCAL DERIVED" | "LOCAL ONLY" | "FINALIZED" | "SUBMITTED" | "HISTORICAL / UNCONFIRMED" | "MISSING LOCALLY" | "AVAILABLE LOCALLY" | "CONTENT-ADDRESSED" | "CURRENTLY AUTHORIZED";
export function Badge({ children }: { children: BadgeLabel | string }) {
  const slug = children.toLowerCase().replace(/[^a-z0-9]+/g, "-");
  return <span className={`badge badge-${slug}`}>{children}</span>;
}

export function Identifier({ value, label = "identifier" }: { value: string; label?: string }) {
  const copy = () => void navigator.clipboard?.writeText(value);
  return <span className="identifier" title={value}><code aria-label={`${label}: ${value}`}>{value}</code><button type="button" className="copy" onClick={copy} aria-label={`Copy ${label}`}>Copy</button></span>;
}

export function ExternalLink({ href }: { href?: string }) {
  if (!href) return <span className="muted">No external URI</span>;
  let url: URL;
  try { url = new URL(href); } catch { return <span className="muted">Invalid external URI</span>; }
  if (url.protocol !== "https:") return <span className="muted">Blocked non-HTTPS URI ({url.hostname || "unknown host"})</span>;
  return <a href={url.href} target="_blank" rel="noopener noreferrer">Open {url.hostname} <span aria-hidden="true">↗</span></a>;
}

export function ObjectReferences({ objects }: { objects: ObjectAvailability[] }) {
  const { api, canMutate } = useNode();
  const fetchObject = async (event: React.MouseEvent<HTMLButtonElement>, id: string) => {
    const button = event.currentTarget; button.disabled = true; button.textContent = "Fetching…";
    try { const result = await api.fetchObject(id); button.textContent = result.status; window.location.reload(); }
    catch (cause) { button.textContent = cause instanceof Error ? cause.message : "Fetch failed"; button.disabled = false; }
  };
  if (objects.length === 0) return <p className="muted">No content-addressed objects referenced.</p>;
  return <div className="list">{objects.map((object) => <div className="card" key={object.object_id}>
    <div className="badges"><Badge>CONTENT-ADDRESSED</Badge><Badge>{object.present_local ? "AVAILABLE LOCALLY" : "MISSING LOCALLY"}</Badge></div>
    <Identifier value={object.object_id} label="ObjectID" />
    <div className="actions"><Link className="button" href={`/objects/${encodeURIComponent(object.object_id)}`}>Inspect</Link>{!object.present_local && <button disabled={!canMutate} onClick={(event) => void fetchObject(event, object.object_id)}>Fetch from peers</button>}</div>
  </div>)}</div>;
}

export function formatBytes(value: number): string {
  if (!Number.isFinite(value)) return "—";
  if (value < 1024) return `${value} B`;
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KiB`;
  return `${(value / (1024 * 1024)).toFixed(1)} MiB`;
}
