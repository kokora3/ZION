# Minimal Governance

Phase 6 makes membership and validator-set authority canonical without adding tokens, stake, reputation, or a permanent administrator. Consensus orders governance transactions; the deterministic state machine verifies authorization, records votes, decides outcomes, and executes approved effects.

Identity, membership, and validation are separate. IdentityID is the stable voting identity and its current active signing key authorizes a transaction. Membership (PENDING, ACTIVE, SUSPENDED, REVOKED) determines governance eligibility. A validator has a distinct CometBFT consensus public key and may explicitly name an operator IdentityID; the consensus key is never derived from the member key.

## Policy and lifecycle

The alpha policy is one currently ACTIVE member per vote, with at least three unique participating identities. Votes are YES, NO, or ABSTAIN. Abstention satisfies participation but is excluded from the approval denominator. Approval uses integer arithmetic only:

    YES * 3 > (YES + NO) * 2

Thus 3 YES approves, 2 YES plus 1 NO rejects, two voters expire for insufficient participation, and 2 YES plus 1 ABSTAIN approves. A key rotation does not create another vote because duplicate detection keys votes by IdentityID.

Proposal status progresses deterministically:

    OPEN -> APPROVED -> EXECUTED
         -> REJECTED
         -> EXPIRED

An ACTIVE member signs and submits a proposal. The state machine snapshots the sorted ACTIVE electorate and derives ProposalID from the canonical proposal body. Votes require inclusion in that snapshot and current ACTIVE authorization. A member activated later cannot vote on the already-open proposal; a snapshotted member suspended before voting cannot cast a vote.

Voting closes at the proposal's canonical end height. Any currently ACTIVE member may submit the separately domain-signed finalization transaction at or after that height. Finalization does not use wall-clock time. An approved proposal needs a separately signed execution transaction, and each proposal can execute only once.

## Authorization and execution

Proposal, vote, finalization, and execution signatures use distinct domain-separated envelopes and are network-bound. Retired member keys remain historically verifiable but are not currently authorized.

A membership proposal states target, expected status, and requested status. Execution rechecks the expected status; a stale proposal fails atomically and remains APPROVED. Supported transitions are PENDING to ACTIVE, ACTIVE to SUSPENDED or REVOKED, and SUSPENDED to ACTIVE or REVOKED. Direct MembershipChange remains rejected.

A validator proposal states ADD or REMOVE, an explicit operator IdentityID, a distinct Ed25519 consensus public key, equal power 1, and the expected validator-set hash. The operator must be ACTIVE at execution. Removal cannot take the set below three validators. Successful execution emits the deterministic ABCI validator update; local files or configuration never authorize it. CometBFT v1.0.1 applies an update returned while finalizing height H to the validator set effective at H+2 and performs its own BFT quorum calculation.

Protocol-upgrade and migration proposals canonically record decisions. They do not download or hot-swap software, nor perform Phase 13 export/import. Phase 11 activates `RESEARCH_ADMISSION` and `RESOURCE_ADMISSION`; their approved execution inserts one immutable bounded entry into StateSchemaV3 after deterministic revalidation.

## Canonical state and limits

Governance is an explicit deterministic StateSchemaV1 to StateSchemaV2 bootstrap transition. V1 snapshot bytes and Phase 4 golden hashes remain unchanged. V2 includes the policy, sorted proposals, electorate snapshots, sorted vote records, statuses, and sorted validator set in StateHash.

The protocol limits canonical transactions to 65,536 bytes, proposal payloads to 4,096 canonical bytes, and electorate/votes per proposal to 1,024. Voting lasts 10 consensus blocks. Finalized proposal history remains canonical; Phase 6 adds no pruning or garbage collection.

Rejected operations use copy-on-write atomicity: membership, proposals, votes, execution state, validator state, and StateHash remain unchanged.
