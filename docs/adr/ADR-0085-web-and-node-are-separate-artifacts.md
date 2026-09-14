# ADR-0085: Web and Node Are Separate Runtime Artifacts

## Status

Accepted.

## Decision

Distribute the optional Next.js Web client separately from the Go node image and portable node packages.

## Consequences

Server operators need not run Web. The browser remains a replaceable direct client of the local API and receives no server or signing authority.

