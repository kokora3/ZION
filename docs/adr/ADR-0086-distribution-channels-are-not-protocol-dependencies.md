# ADR-0086: GitHub Releases and GHCR Are Distribution Channels

## Status

Accepted.

## Decision

Prepare deliberate GitHub Release and GHCR workflows without making either service a runtime or protocol dependency.

## Consequences

Artifacts remain locally buildable and verifiable. Registry or hosting failure cannot change protocol semantics, canonical state, or the ability to run existing artifacts.

