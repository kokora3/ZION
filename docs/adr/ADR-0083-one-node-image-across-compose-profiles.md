# ADR-0083: One zion-node Image Serves All Compose Profiles

## Status

Accepted.

## Decision

NORMAL, BOOTSTRAP, and VALIDATOR Compose profiles use the same versioned `zion-node` image and select capabilities only with external configuration. A profile does not confer canonical authority.

## Consequences

Distribution cannot drift into role-specific implementations. Bootstrap remains replaceable, and validator effectiveness still requires verified genesis/canonical authorization.

