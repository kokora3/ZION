# ADR-0024: Key-Rotation Replay Enforced by Canonical State

## Status
Accepted.

## Context
A historical Ed25519 signature remains cryptographically valid after its key is retired, so signature verification alone cannot establish current authorization or prevent replay.

## Decision
Require each rotation sequence to equal the canonical stored sequence plus one and require its old `KeyID` to be currently active. On success, retire the old key, activate the new public key, and advance the stored sequence atomically.

## Consequences
Prior rotations fail sequence validation, retired keys cannot authorize later rotations, `IdentityID` remains stable, and membership remains unchanged.
