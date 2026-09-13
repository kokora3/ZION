# Canonical Research and Resource Registries

Phase 11 adds two small, immutable canonical registries. Research entries describe research outputs; Resource entries describe repositories, datasets, documents, tools, websites, models, or other useful resources. They contain bounded metadata and references, never repository contents or large files.

```text
Candidate
   |
   v
Governance
   |
   v
Approved
   |
   v
Execute
   |
   v
Canonical Registry
   |
   v
StateHash/AppHash
```

## IDs and canonical bodies

`ResearchID` is `zion:research:sha256:<lowercase-hex>` and `ResourceID` is `zion:resource:sha256:<lowercase-hex>`. Each digest is SHA-256 over its validated canonical CBOR body. The ID is not in that body, so derivation has no self-hash cycle. The namespaces and Go types are distinct from ObjectID, ProposalID, IdentityID, TxID, and StateHash.

Research metadata bounds title (256 bytes), summary (1,024), contributors (32), external identifiers (16), ObjectIDs (16), canonical references (32), and an optional absolute HTTPS URI (2,048). Resource metadata uses a strict kind enum and bounds name, summary, URI, license, maintainers, ObjectIDs, and canonical references. Text is valid UTF-8 in NFC. The existing 4,096-byte governance proposal-payload limit remains the final bound.

Entries are immutable in v0.1. A changed body produces a different typed ID. `proposal_id` and chain `admitted_height` are deterministic provenance stored alongside the body but excluded from ID derivation.

## Admission and references

Only `RESEARCH_ADMISSION` and `RESOURCE_ADMISSION` proposals can mutate the registries. Existing one-ACTIVE-IdentityID/one-vote, minimum-three-participant, and strict-greater-than-two-thirds rules apply. Approval and execution remain separate. Execution revalidates the body, immutable ID uniqueness, and reference targets before atomically inserting the entry and marking the proposal executed. Direct register and administrator bypass routes do not exist.

Canonical references use an explicit vocabulary (`cites`, `uses`, `implements`, `related`, version 1) and target RESEARCH, RESOURCE, or OBJECT. Registry targets must already exist when execution occurs. Object targets require a strict ObjectID but their bytes need not be locally available. Apply performs no filesystem, object-store, P2P, DNS, HTTP, DOI, or repository access.

```text
Board Reference != Canonical Registry Reference
```

A canonical registry reference is governance-approved chain metadata. A Board reference is an author's signed off-chain community statement. Board events can point to ResearchID and ResourceID, but do not become canonical or confer endorsement.

## State, local discovery, and availability

State schema V3 is an explicit deterministic migration from V2 and adds ID-sorted Research and Resource snapshot arrays. V1/V2 decoders and frozen fixtures retain their original bytes and StateHashes. Registry metadata contributes to StateHash/AppHash; local object availability, external URI contents, local index bytes, and searches do not.

The versioned JSON registry index is a rebuildable local projection of canonical state. Missing, malformed, or oversized index data is discarded and rebuilt. List/get/search are bounded to 100 results and search input to 512 UTF-8 bytes. Search covers research titles, summaries, contributor names and external identifiers, and resource names, summaries, kinds, URIs, licenses and maintainers.

The `/v1` API and `zionctl research|resource list|get|search` expose read-only discovery. Object references include a local `present_local` hint; clients use the existing explicit Phase 9 object fetch endpoint when desired. An external HTTPS URI is a mutable location and is never fetched by consensus. Governance curation is not peer review, malware analysis, availability assurance, or permission to execute code.

Phase 12 may build higher-level product behavior. Phase 11 does not add graph inference, large-file chunking, repository cloning, automatic updates, execution, DHT discovery, or a central search service.
