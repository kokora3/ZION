"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useNode } from "./node-provider";

const nav = [["/", "Overview"], ["/board", "Board"], ["/research", "Research"], ["/resources", "Resources"], ["/governance", "Governance"], ["/network", "Network"]] as const;

export function Shell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const node = useNode();
  const ready = node.connection === "connected" && node.health?.ready;
  return <div className="app-shell">
    <header className="topbar">
      <Link className="brand" href="/" aria-label="ZION overview"><span className="brand-mark">Z</span><span>ZION <small>ALPHA</small></span></Link>
      <div className="node-indicator" data-state={node.connection} aria-live="polite">
        <span className="status-dot" />
        <strong>{node.connection === "connected" ? (ready ? "Connected" : node.status?.sync_status === "SYNCING" ? "Syncing" : "Not ready") : node.connection}</strong>
        {node.status && <><span>{node.status.network_id}</span><span>{node.status.roles.join(" + ") || "NORMAL"}</span><span>{node.status.sync_status}</span></>}
      </div>
      <Link className="settings-link" href="/settings">Settings</Link>
    </header>
    <div className="alpha-strip"><strong>zion-alpha-1</strong> is not mainnet. Resets and migrations may occur; public P2P content may persist while local availability can change.</div>
    {node.status && !node.compatible && <div className="warning" role="alert"><strong>Incompatible node.</strong> Expected zion-alpha-1 / protocol 0.1. Mutating actions are disabled; diagnostics remain available.</div>}
    {node.stale && <div className="warning" role="status"><strong>Stale view.</strong> The local node is offline; cached screen data is not current.</div>}
    <div className="body-grid">
      <nav className="sidebar" aria-label="Primary navigation">
        {nav.map(([href, label]) => <Link key={href} href={href} aria-current={pathname === href || (href !== "/" && pathname.startsWith(`${href}/`)) ? "page" : undefined}>{label}</Link>)}
      </nav>
      <main id="main-content">{children}</main>
    </div>
  </div>;
}
