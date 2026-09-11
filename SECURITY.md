# Security Policy

ZION alpha software is experimental. Do not assume production security or use it for security-critical assets or data.

Please do not publicly disclose suspected security vulnerabilities before coordination. Prefer a GitHub private security advisory for this repository when available. No security-reporting email address is currently published.

Changes involving cryptography, consensus, canonical encoding, identity, or parsers require additional review.

## Phase 3 identity boundaries

Private keys never appear in public protocol objects, logs, or public state exports. Deterministic fixture keys are **TEST ONLY — PUBLIC FIXTURE — NOT SECRET — NEVER USE IN PRODUCTION**.

A valid signature is not current authorization. Historical signatures from retired keys may remain cryptographically valid, while Phase 4 canonical chain state will decide which keys are currently authorized. Normal rotation requires old-key authorization and new-key proof of possession; its signed sequence alone does not reject a valid historical replay.

Valid identity does not imply ACTIVE membership, and ACTIVE membership does not imply validator authority. A member IdentityID is not a P2P Peer ID. Unknown algorithms fail closed; malformed keys, IDs, signatures, and encoded structures must fail safely without panic. This is not a claim of formal verification.
