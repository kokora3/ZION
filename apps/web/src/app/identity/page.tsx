"use client";

import { useState } from "react";
import { useNode } from "@/components/node-provider";
import type { IdentityView, MembershipView } from "@/lib/zion-api";
import { Badge, Identifier, PageHeading } from "@/components/ui";

export default function IdentityPage() {
  const { api } = useNode(); const [id, setID] = useState(""); const [identity, setIdentity] = useState<IdentityView>(); const [membership, setMembership] = useState<MembershipView>(); const [error, setError] = useState("");
  const inspect = async (event: React.FormEvent) => { event.preventDefault(); setError(""); try { const [nextIdentity, nextMembership] = await Promise.all([api.identity(id.trim()), api.membership(id.trim())]); setIdentity(nextIdentity); setMembership(nextMembership); } catch (cause) { setIdentity(undefined); setMembership(undefined); setError(cause instanceof Error ? cause.message : "Lookup failed"); } };
  return <><PageHeading eyebrow="Canonical public state" title="Identity & membership"><p>Identity is cryptographic continuity. Membership is a separate canonical authorization state.</p></PageHeading><form className="card" onSubmit={(event) => void inspect(event)}><label>IdentityID<input value={id} onChange={(event) => setID(event.target.value)} placeholder="zion:id:sha256:…" required /></label><button>Inspect local canonical state</button>{error && <p role="alert" className="warning">{error}</p>}</form>{identity && membership && <section className="card" style={{marginTop:"1rem"}}><div className="badges"><Badge>CANONICAL</Badge><span className="badge">{membership.status}</span></div><dl className="stat-list"><dt>IdentityID</dt><dd><Identifier value={identity.identity_id} label="IdentityID" /></dd><dt>Active KeyID</dt><dd>{identity.active_key_id ? <Identifier value={identity.active_key_id} label="KeyID" /> : "No active key"}</dd><dt>Rotation sequence</dt><dd>{identity.sequence}</dd><dt>Keys recorded</dt><dd>{identity.key_count}</dd><dt>Membership</dt><dd>{membership.status}</dd><dt>Revoked</dt><dd>{identity.revoked ? "yes" : "no"}</dd></dl></section>}</>;
}
