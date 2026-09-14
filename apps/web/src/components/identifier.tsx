"use client";

import { useState } from "react";

export function Identifier({ value, label = "Identifier" }: { value: string; label?: string }) {
  const [copied, setCopied] = useState(false);
  const copy = async () => { await navigator.clipboard.writeText(value); setCopied(true); window.setTimeout(() => setCopied(false), 1_200); };
  return <span className="identifier" title={value} aria-label={`${label}: ${value}`}>
    <code>{value || "—"}</code>
    {value && <button type="button" className="copy" onClick={() => void copy()} aria-label={`Copy full ${label}`}>{copied ? "Copied" : "Copy"}</button>}
  </span>;
}
