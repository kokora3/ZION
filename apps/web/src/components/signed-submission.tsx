"use client";

import { useState } from "react";
import Link from "next/link";
import { useNode } from "./node-provider";
import { Identifier } from "./ui";

export function SignedSubmission({ kind }: { kind: "transaction" | "board" }) {
  const { api, canMutate } = useNode();
  const [payload, setPayload] = useState("");
  const [result, setResult] = useState<{ id: string; status: string }>();
  const [error, setError] = useState<string>();
  const submit = async (event: React.FormEvent) => {
    event.preventDefault(); setError(undefined); setResult(undefined);
    try {
      if (kind === "board") {
        const value = await api.submitBoardEvent(payload.trim());
        setResult({ id: value.post_id, status: "Accepted by local Board path" });
      } else {
        const value = await api.submitTransaction(payload.trim());
        setResult({ id: value.tx_id, status: value.status });
      }
      setPayload("");
    } catch (cause) { setError(cause instanceof Error ? cause.message : "Submission failed"); }
  };
  return <form onSubmit={(event) => void submit(event)} className="card">
    <h2>Submit signed {kind === "board" ? "Board event" : "canonical transaction"}</h2>
    <p>This client does not hold member keys. Build and sign through <code>zionctl</code> or another local signer, then paste only the base64 canonical signed payload.</p>
    {kind === "board" && <p className="warning">Public P2P content may remain on independent nodes even if it is later hidden locally.</p>}
    <label>Base64 signed payload<textarea value={payload} onChange={(event) => setPayload(event.target.value)} spellCheck={false} autoComplete="off" required /></label>
    <button disabled={!canMutate || !payload.trim()}>{canMutate ? "Submit to local node" : "Unavailable until node is ready and compatible"}</button>
    {error && <p role="alert" className="warning">{error}</p>}
    {result && <div role="status"><strong>{result.status}</strong><Identifier value={result.id} label={kind === "board" ? "PostID" : "TxID"} />{kind === "transaction" && <Link href={`/transactions/${encodeURIComponent(result.id)}`}>Track node-observed finality</Link>}</div>}
  </form>;
}
