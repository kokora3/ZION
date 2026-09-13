# ADR-0058: Phase 9 Object Content Is Not Confidential

## Status

Accepted.

## Decision

Phase 9 provides transport authentication and content-address integrity, not application-level confidentiality. The `LOCAL` visibility value is descriptive metadata and is not encryption or access control.

## Consequences

Operators must not store secrets expecting ZION object confidentiality. Access policy, encryption, author signatures, and registry authorization require later protocol decisions.
