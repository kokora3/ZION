package chain

import (
	"bytes"
	"fmt"

	"github.com/kokora3/zion/internal/governance"
	"github.com/kokora3/zion/internal/identity"
	"github.com/kokora3/zion/internal/membership"
	"github.com/kokora3/zion/internal/protocol"
)

// StateFromSnapshot reconstructs the in-memory form of a canonical snapshot.
// It accepts no alternate state representation and requires the reconstructed
// state to reproduce the exact canonical snapshot bytes.
func StateFromSnapshot(snapshot Snapshot) (State, error) {
	state := State{
		SchemaVersion: snapshot.SchemaVersion, NetworkID: snapshot.NetworkID,
		ProtocolVersion: snapshot.ProtocolVersion,
		Identities:      make(map[string]StoredIdentity, len(snapshot.Identities)),
		Memberships:     make(map[string]membership.Status, len(snapshot.Memberships)),
	}
	for _, item := range snapshot.Identities {
		key := item.ID.String()
		if key == "" {
			return State{}, fmt.Errorf("invalid snapshot identity")
		}
		if _, exists := state.Identities[key]; exists {
			return State{}, fmt.Errorf("duplicate snapshot identity")
		}
		stored := StoredIdentity{ID: cloneIdentityID(item.ID), Keys: make(map[string]identity.PublicKey, len(item.Keys)),
			Active: make(map[string]bool, len(item.Keys)), Sequence: item.Sequence, Revoked: item.Revoked}
		for _, snapshotKey := range item.Keys {
			keyID := snapshotKey.ID.String()
			if keyID == "" {
				return State{}, fmt.Errorf("invalid snapshot key")
			}
			if _, exists := stored.Keys[keyID]; exists {
				return State{}, fmt.Errorf("duplicate snapshot key")
			}
			stored.Keys[keyID] = clonePublicKey(snapshotKey.Public)
			stored.Active[keyID] = snapshotKey.Active
		}
		state.Identities[key] = stored
	}
	for _, item := range snapshot.Memberships {
		key := item.ID.String()
		if _, exists := state.Memberships[key]; exists {
			return State{}, fmt.Errorf("duplicate snapshot membership")
		}
		state.Memberships[key] = item.Status
	}
	if snapshot.Governance != nil {
		governanceState := governance.State{Policy: snapshot.Governance.Policy,
			Proposals:  make(map[string]governance.ProposalRecord, len(snapshot.Governance.Proposals)),
			Validators: make(map[string]governance.Validator, len(snapshot.Governance.Validators))}
		for _, item := range snapshot.Governance.Proposals {
			key := item.ID.String()
			if key == "" {
				return State{}, fmt.Errorf("invalid snapshot proposal")
			}
			if _, exists := governanceState.Proposals[key]; exists {
				return State{}, fmt.Errorf("duplicate snapshot proposal")
			}
			votes := make(map[string]governance.VoteRecord, len(item.Votes))
			for _, vote := range item.Votes {
				voter := vote.Voter.String()
				if _, exists := votes[voter]; exists {
					return State{}, fmt.Errorf("duplicate snapshot vote")
				}
				vote.Voter = cloneIdentityID(vote.Voter)
				vote.KeyID = cloneKeyID(vote.KeyID)
				votes[voter] = vote
			}
			electorate := make([]identity.IdentityID, len(item.Electorate))
			for index, member := range item.Electorate {
				electorate[index] = cloneIdentityID(member)
			}
			proposalID := governance.ProposalID{HashDigest: protocol.HashDigest{
				Algorithm: item.ID.Algorithm, Digest: append([]byte(nil), item.ID.Digest...)}}
			governanceState.Proposals[key] = governance.ProposalRecord{ID: proposalID,
				Body: governance.CloneProposalBody(item.Body), Status: item.Status, StartHeight: item.StartHeight,
				EndHeight: item.EndHeight, Electorate: electorate, Votes: votes}
		}
		for _, validator := range snapshot.Governance.Validators {
			key := governance.ValidatorKey(validator.PublicKey)
			if _, exists := governanceState.Validators[key]; exists {
				return State{}, fmt.Errorf("duplicate snapshot validator")
			}
			governanceState.Validators[key] = governance.CloneValidator(validator)
		}
		state.Governance = &governanceState
	}
	if err := restoreRegistries(snapshot, &state); err != nil {
		return State{}, err
	}
	want, err := protocol.CanonicalEncode(snapshot)
	if err != nil {
		return State{}, err
	}
	got, err := state.CanonicalBytes()
	if err != nil {
		return State{}, fmt.Errorf("invalid canonical snapshot: %w", err)
	}
	if !bytes.Equal(want, got) {
		return State{}, fmt.Errorf("snapshot is not in canonical order")
	}
	return state, nil
}
