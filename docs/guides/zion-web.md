# Run ZION Web locally

ZION Web is the official alpha client, not the node. Start a local `zion-node` first and keep `zionctl` available for safe signing workflows.

1. Configure the node API on loopback and allow the exact development origin:

   ```yaml
   api:
     listen: 127.0.0.1:42001
     allowed_origins:
       - http://127.0.0.1:3000
   ```

2. Start the node:

   ```text
   zion-node run --config configs/alpha-1/zion.yaml
   ```

3. Install and start the client:

   ```text
   cd apps/web
   npm ci
   npm run dev
   ```

4. Open `http://127.0.0.1:3000`. The header must show Connected, `zion-alpha-1`, the node sync state, and role hints. If it shows Offline, check the API URL, exact CORS origin, and that `zion-node` is running. Settings never select a central fallback.

Board shows signed public posts and replies from this node's derived index. Search is local. “Hide on this node” changes only this node's view; it is not deletion. Public P2P content may remain on other nodes. Board text is untrusted and shown without raw HTML.

Research and Resources are governance-admitted canonical metadata. Their external URI is a mutable HTTPS location; ObjectIDs identify immutable content. Missing objects are not fetched automatically: use “Fetch from peers” explicitly. A registered tool/model/repository is not automatically safe, installed, or executed.

Governance shows OPEN, APPROVED, REJECTED, EXPIRED, and EXECUTED as distinct node-reported states. APPROVED does not mean EXECUTED. Current node software has no general local member signer API, so voting and Research/Resource proposal actions use signed canonical payloads. Build and sign with authoritative Go tooling or another reviewed local client, then paste only the base64 signed transaction. The same rule applies to Board publishing. Never paste a private key into the web client.

Object downloads return verified canonical bytes and are not rendered or executed. Object sharing and Board publishing are public-alpha actions: independent nodes may retain bytes, while availability is never guaranteed.

For a local production build:

```text
cd apps/web
npm ci
npm run build
npm run start -- --port 3000
```

Set `NEXT_PUBLIC_ZION_API_URL` before building only when a different non-secret API origin is required in CSP. Never put bearer tokens, seeds, member keys, P2P keys, or validator keys in public environment variables. A public HTTPS deployment may be unable to reach local HTTP; local web plus local node is the supported alpha path.
