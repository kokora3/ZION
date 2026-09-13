# ADR-0066: Research and Resource Admission Requires Governance

## Status

Accepted.

## Decision

Activate the reserved `RESEARCH_ADMISSION` and `RESOURCE_ADMISSION` proposal kinds. Registry mutation occurs only through approved `GovernanceExecute` using the unchanged Phase 6 policy.

## Consequences

Member signatures, local configuration, API callers, and node operators cannot register entries directly.
