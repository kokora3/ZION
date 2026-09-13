# ADR-0049: Versioned Local API Is Loopback-Only by Default

## Status

Accepted.

## Decision

Expose only /v1 routes, bind loopback by default, require a bearer token for non-loopback binding, and reject wildcard CORS.

## Consequences

Remote exposure is explicit local operator risk and API data remains non-canonical.
