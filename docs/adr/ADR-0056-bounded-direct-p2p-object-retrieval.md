# ADR-0056: Bounded Direct P2P Retrieval by ObjectID

## Status

Accepted.

## Decision

Use the versioned `/zion/object/0.1.0` canonical-CBOR protocol over authenticated Phase 7 peers. Requests and responses, candidates, concurrent work, handlers, frames, and deadlines are bounded. Every `FOUND` response is revalidated against the requested ObjectID before local storage.

## Consequences

PEX supplies untrusted contact hints, not content trust. Retrieval is best-effort direct peer querying with deterministic byte validation; it is not consensus, a DHT, provider routing, or an availability guarantee.
