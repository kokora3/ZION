# ADR-0081: zion-alpha-1 Release Artifacts Are Local Packages

## Status

Accepted.

## Decision

Build `v0.1.0-alpha.1` Windows amd64 and Linux amd64 archives locally with `-trimpath`, injected commit/build-date metadata, an inventory, and a deterministic SHA-256 manifest. Packaging does not require a hosted service, GitHub Release, container registry, or signing key.

## Consequences

Artifacts remain independently distributable and verifiable. The absence of code signing is stated rather than replaced with a fake signature; operators must obtain checksums through an appropriately trusted channel.
