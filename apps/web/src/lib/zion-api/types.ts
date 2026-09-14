export type NodeHealth = { live: boolean; ready: boolean };

export type NodeStatus = {
  network_id: string;
  genesis_id: string;
  runtime_state: string;
  roles: string[];
  peer_id: string;
  sync_status: string;
  accepted_height: number;
  state_hash: string;
  consensus_active: boolean;
  validator_authorized: boolean;
  protocol_version: string;
  object_store_enabled: boolean;
  object_count: number;
  object_bytes: number;
  object_quota_bytes: number;
  board_enabled: boolean;
  indexed_posts: number;
  indexed_replies: number;
  hidden_local: number;
  board_sync_state: string;
};

export type StateSummary = {
  schema_version: number;
  network_id: string;
  state_hash: string;
  accepted_height: number;
  identity_count: number;
  membership_counts: Record<string, number>;
  governance_proposal_count: number;
  validator_count: number;
  research_count: number;
  resource_count: number;
};

export type Peer = { PeerID: string; Addresses: string[]; Roles: string[]; Version: unknown };
export type BoardReference = { Relation: string; TargetKind: "OBJECT" | "RESEARCH" | "RESOURCE"; TargetID: string };
export type BoardPost = {
  post_id: string;
  kind: "POST" | "REPLY";
  author_identity: string;
  author_key_id: string;
  created_at: number;
  parent_post?: string;
  content_object: string;
  references: BoardReference[];
  unresolved_references?: BoardReference[];
  content_present: boolean;
  signature_status: string;
  authorization_status: string;
  author_current_membership: string;
  local_visibility: string;
  parent_present: boolean;
  title?: string;
  body?: string;
};

export type CanonicalReference = { relation: string; target_kind: "RESEARCH" | "RESOURCE" | "OBJECT"; target_id: string };
export type ObjectAvailability = { object_id: string; present_local: boolean };
export type ResearchEntry = {
  research_id: string;
  body: {
    schema_version: number; title: string; summary: string;
    contributors: Array<{ display_name: string; identity_id?: unknown }>;
    external_identifiers: Array<{ scheme: string; value: string }>;
    external_uri?: string; object_refs: unknown[]; canonical_refs: CanonicalReference[];
  };
  proposal_id: string;
  admitted_height: number;
  object_availability: ObjectAvailability[];
};

export type ResourceEntry = {
  resource_id: string;
  body: {
    schema_version: number; kind: string; name: string; summary: string;
    external_uri?: string; license?: string;
    maintainers: Array<{ display_name: string; identity_id?: unknown }>;
    object_refs: unknown[]; canonical_refs: CanonicalReference[];
  };
  proposal_id: string;
  admitted_height: number;
  object_availability: ObjectAvailability[];
};

export type ProposalVote = { voter: string; choice: "YES" | "NO" | "ABSTAIN"; key_id: string };
export type GovernanceProposal = {
  proposal_id: string;
  kind: string;
  proposer: string;
  status: "OPEN" | "APPROVED" | "REJECTED" | "EXPIRED" | "EXECUTED";
  start_height: number;
  end_height: number;
  electorate_count: number;
  yes: number;
  no: number;
  abstain: number;
  participants: number;
  votes: ProposalVote[];
  payload?: unknown;
};

export type IdentityView = {
  identity_id: string; sequence: number; revoked: boolean; key_count: number;
  active_key_id?: string; membership: string;
};
export type MembershipView = { identity_id: string; status: string };
export type ObjectMeta = { object_id: string; size: number; present: boolean; stored_at?: string };
export type ObjectResult = { object_id: string; size: number; status: string };
export type Submission = { tx_id: string; status: string };
export type TransactionStatus = { tx_id: string; status: string; committed_height?: number; result_code?: string; state_hash?: string };
