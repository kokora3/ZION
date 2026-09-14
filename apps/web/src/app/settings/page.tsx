"use client";

import { useState } from "react";
import { useNode } from "@/components/node-provider";
import { PageHeading } from "@/components/ui";

export default function SettingsPage() {
  const node = useNode(); const [url, setURL] = useState(node.apiURL); const [token, setToken] = useState(node.token); const [error, setError] = useState("");
  const save = (event: React.FormEvent) => { event.preventDefault(); setError(""); try { node.saveSettings(url, token); } catch (cause) { setError(cause instanceof Error ? cause.message : "Invalid settings"); } };
  return <><PageHeading eyebrow="Client-local settings" title="Node connection"><p>Only this browser&apos;s non-authoritative connection preferences are changed.</p></PageHeading><form className="card" onSubmit={save}><label>Node API base URL<input type="url" value={url} onChange={(event) => setURL(event.target.value)} required /></label><p className="muted">Default: loopback Phase 8 API. ZION Web never falls back to a hosted service.</p><label>Optional bearer token<input type="password" value={token} onChange={(event) => setToken(event.target.value)} autoComplete="off" /></label><p className="warning">Bearer tokens are kept in session storage only, never in the URL or a public build variable. Browser storage is still accessible to code in this origin; prefer loopback without a token where possible.</p><button>Save and reconnect</button>{error && <p role="alert">{error}</p>}</form><section className="card" style={{marginTop:"1rem"}}><h2>HTTPS and local HTTP</h2><p>A public HTTPS site may be blocked from calling a local HTTP node by mixed-content or private-network browser rules. The supported alpha workflow is local ZION Web plus local <code>zion-node</code>. Remote binding keeps the node&apos;s explicit bearer, firewall, CORS, and TLS requirements.</p></section></>;
}
