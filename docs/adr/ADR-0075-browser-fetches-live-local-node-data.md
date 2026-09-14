# ADR-0075: Live Node Data Is Fetched by the Browser

## Status

Accepted.

## Decision

Fetch live node state from the browser directly to the configured node. Do not use hosted SSR or a Next API route whose localhost would refer to the hosting server.

## Consequences

Local-first use is honest and replaceable. Hosted HTTPS/private-network limitations must be disclosed.
