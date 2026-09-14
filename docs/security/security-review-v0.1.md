# ZION v0.1 Alpha Security Review Checklist

Reviewed for `v0.1.0-alpha.1`:

- Canonical Apply uses ordered canonical inputs only; no clock, random, filesystem, DNS, HTTP, Web, metrics, logs, or object availability input.
- Member, validator, and P2P key domains remain separate; private keys are absent from state, export, API, P2P messages, logs, fixtures intended as protocol values, and release packages.
- Identity creation proves possession; rotation requires current old-key authorization plus new-key proof and sequence.
- Membership and validator changes require canonical governance; no API/config/bootstrap/Web admin bypass exists.
- Transaction, governance, P2P, state-sync, object, Board, registry, API, cache, index, and export inputs are bounded and malformed input fails closed.
- StateHash/AppHash, frozen vectors, atomic rejection, map/allocation determinism, race tests, and owned decoder fuzz targets are part of the final gate.
- Export recomputes canonical bytes/StateHash and import is explicit, fresh-normal-only, genesis-bound, and no-overwrite.
- Local metrics have bounded labels and share API access controls; structured logs exclude secrets and payload bodies.
- Browser rendering treats public content as untrusted and has no private-key custody or central backend.
- Dependency inventories and ecosystem vulnerability scans are recorded for the release; this is not legal advice or formal verification.

Known trust boundaries remain: permissioned alpha validators/genesis, authenticated-but-not-Sybil-resistant P2P, non-light-client state sync, public unencrypted Board/object data, operator-managed software distribution/keys, and possible alpha reset.
