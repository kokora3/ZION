# ADR-0073: Official ZION Web Is a Replaceable Local-Node Client

## Status

Accepted.

## Decision

Implement the official web application as a Next.js client of the stable local `/v1` node API. It owns presentation and client-local connection preferences only.

## Consequences

The node and CLI work without the web client. Closing or replacing the UI cannot stop or redefine protocol behavior.
