# ADR-0021: TxID Namespace and Derivation

## Status
Accepted.

## Context
Transactions need stable, cross-language identifiers distinct from object, identity, key, and state identifiers.

## Decision
Derive `TxID` as SHA-256 over the complete RFC 8949 Core Deterministic CBOR transaction envelope and render it as `zion:tx:sha256:<lowercase-hex>`. Do not place a TxID field in the hashed transaction.

## Consequences
Equivalent transactions have equivalent identifiers, changed canonical transaction fields change the digest, self-inclusion is impossible, and parsers can reject the wrong namespace or malformed digest.
