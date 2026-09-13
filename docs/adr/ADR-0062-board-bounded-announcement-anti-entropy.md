# ADR-0062: Board Synchronization Uses Bounded Announcement and Anti-Entropy

## Status

Accepted.

## Decision

Versioned libp2p protocols provide bounded event announcements and cursor-paged anti-entropy. Content is pulled through the Phase 9 object protocol only after event admission.

## Consequences

Offline peers can repair missed events without a central server. Every peer and message remains untrusted and independently verified; replication and global completeness are not guaranteed.
