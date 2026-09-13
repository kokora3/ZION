package chain

import "testing"

func TestStateFromSnapshotRoundTripAndAllocationIsolation(t *testing.T) {
	scenario := setupGovernanceState(t)
	snapshot, err := scenario.state.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := StateFromSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	want, err := scenario.state.Hash()
	if err != nil {
		t.Fatal(err)
	}
	got, err := restored.Hash()
	if err != nil || got.String() != want.String() {
		t.Fatal("restored governance state differs from canonical source")
	}
	snapshot.Identities[0].ID.Digest[0] ^= 0xff
	snapshot.Identities[0].Keys[0].Public.Key[0] ^= 0xff
	snapshot.Governance.Validators[0].PublicKey[0] ^= 0xff
	afterMutation, err := restored.Hash()
	if err != nil || afterMutation.String() != want.String() {
		t.Fatal("restored state aliases snapshot allocations")
	}
}
