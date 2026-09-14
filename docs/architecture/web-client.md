# Official Web Client

ZION Web is a replaceable Next.js client of the versioned local `zion-node` API. It has no chain, consensus, signing, identifier, P2P, Board-admission, or governance authority and uses no production database or hosted application backend.

```text
Browser / ZION Web
        |
        | bounded JSON over /v1
        v
local zion-node
        |
        +-- canonical chain and governance
        +-- CometBFT finality
        +-- signed Board and object store
        +-- libp2p and derived indexes
```

Live node requests originate in the browser. Next.js renders the local application shell and supplies local static assets; it does not proxy the user's `localhost` through hosted SSR. The supported alpha deployment is `next dev` or a local production `next build` plus `next start`. The build uses standalone output but is not a protocol service. Closing the web client does not stop node, CLI, consensus, P2P, or persistence.

## Connection and readiness

The default API base is `http://127.0.0.1:42001`. A non-secret URL may be changed in Settings and persisted in local storage. An optional bearer token is kept only in session storage, is sent in the Authorization header, and is never put in a URL, log, or `NEXT_PUBLIC_*` value. The node's loopback, exact-origin CORS, remote bearer, firewall, and TLS model remains authoritative.

A public HTTPS site may be prohibited from calling a local HTTP endpoint by mixed-content or private-network browser policy. Operators should run both components locally. A deliberately remote API needs HTTPS, explicit remote binding, an exact CORS origin, and a bearer token; the UI does not weaken those controls.

The global indicator distinguishes transport connection, process liveness, readiness, synchronization, stale cached display, wrong network, and incompatible protocol. Mutation controls require a connected, ready, `SYNCED`, `zion-alpha-1` protocol-0.1 node. An HTTP response alone does not imply readiness or finality.

## Authority and signing boundaries

The UI consistently distinguishes `CANONICAL`, `FINALIZED`, `OFF-CHAIN SIGNED`, `CURRENTLY AUTHORIZED`, `HISTORICAL / UNCONFIRMED`, `LOCAL DERIVED`, `LOCAL ONLY`, and local object availability. It never uses a generic “verified” label for unrelated guarantees. `APPROVED` governance status is visibly different from `EXECUTED`.

No safe general member keystore/signer is exposed by the current runtime. Phase 12 therefore uses signed-payload mode: users create canonical signed transactions or Board events with `zionctl` or another local signer and submit only base64 public canonical bytes. The browser never asks for, stores, derives, or receives a private key or seed. Research/Resource proposal and vote screens explain this boundary rather than simulating an action.

The Go API now exposes a client-safe governance projection with string ProposalID, proposer, status, heights, electorate count, authoritative tally, votes, and bounded typed payload. Identity inspection exposes the current public active KeyID. These are JSON projections only; canonical CBOR and state hashes are unchanged.

## Routes and data

- `/` shows health, sync, height, StateHash, roles, PeerID, validator authorization, peers, registry counts, Board counts, and object quota.
- `/board` and `/board/[postId]` show the local derived feed/search/thread, independent signature and current-authorization labels, signed-event submission, and node-local hide/unhide.
- `/research` and `/resources` provide bounded local search over canonical entries. Detail routes distinguish canonical relations, mutable external HTTPS locations, and content-addressed object availability.
- `/governance` and proposal detail show node-authoritative lifecycle, participation, YES/NO/ABSTAIN, and signed transaction submission/finality tracking.
- `/objects/[objectId]` performs explicit local inspection, bounded peer fetch, and verified-byte download without rendering arbitrary payloads.
- `/network`, `/identity`, and `/settings` show operational peers, canonical public identity/membership, and client-local connection settings.

No page implements canonical CBOR, TxID/ObjectID/ResearchID/ResourceID/StateHash derivation, signature validation, governance arithmetic, or P2P.

## Untrusted content and browser security

Board text is rendered as React text; raw HTML is never enabled and `dangerouslySetInnerHTML` is not used. Arbitrary remote images are not loaded. External registry links require an explicit click, accept HTTPS only, display the hostname, and use `noopener noreferrer`. Registered tools, models, and repositories have no Run or Install action because governance admission is not an executable-safety guarantee.

The local Next server sends CSP, frame-ancestor denial, no-referrer, MIME sniffing protection, and restrictive permissions headers. CSP `connect-src` includes the build-time `NEXT_PUBLIC_ZION_API_URL` origin; a production build using another remote origin must set that non-secret value before building. No analytics, telemetry, advertisements, session replay, remote error collector, or central search service is present.

Client-held display state and settings are non-canonical. When the node is unreachable, retained screen information is labeled stale and no central fallback is attempted. Requests have timeouts and cancellation; polling is moderate and stops on unmount.

Phase 13 may package, harden, observe, and release the complete alpha. It must not turn this client into protocol authority or introduce browser key custody without a separate reviewed keystore decision.
