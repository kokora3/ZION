package chain

import (
	"fmt"
	"sort"

	"github.com/kokora3/zion/internal/governance"
	"github.com/kokora3/zion/internal/protocol"
	"github.com/kokora3/zion/internal/registry"
	"github.com/kokora3/zion/internal/research"
	"github.com/kokora3/zion/internal/resources"
)

type StoredResearch struct {
	ID             research.ID
	Body           research.EntryBody
	ProposalID     governance.ProposalID
	AdmittedHeight int64
}

type StoredResource struct {
	ID             resources.ID
	Body           resources.EntryBody
	ProposalID     governance.ProposalID
	AdmittedHeight int64
}

type SnapshotResearch struct {
	ID             research.ID           `cbor:"1,keyasint"`
	Body           research.EntryBody    `cbor:"2,keyasint"`
	ProposalID     governance.ProposalID `cbor:"3,keyasint"`
	AdmittedHeight int64                 `cbor:"4,keyasint"`
}

type SnapshotResource struct {
	ID             resources.ID          `cbor:"1,keyasint"`
	Body           resources.EntryBody   `cbor:"2,keyasint"`
	ProposalID     governance.ProposalID `cbor:"3,keyasint"`
	AdmittedHeight int64                 `cbor:"4,keyasint"`
}

// BootstrapRegistries is the explicit deterministic StateSchemaV2 to V3
// migration. It performs no I/O and preserves every pre-existing state field.
func BootstrapRegistries(state State) (State, error) {
	if state.SchemaVersion != StateSchemaV2 || state.Governance == nil || state.Research != nil || state.Resources != nil {
		return state, fmt.Errorf("registry bootstrap requires state schema v2")
	}
	next := cloneState(state)
	next.SchemaVersion = StateSchemaV3
	next.Research = make(map[string]StoredResearch)
	next.Resources = make(map[string]StoredResource)
	if _, err := next.Hash(); err != nil {
		return state, fmt.Errorf("invalid registry bootstrap state: %w", err)
	}
	return next, nil
}

func admitResearch(state State, record governance.ProposalRecord, height int64) error {
	if state.SchemaVersion != StateSchemaV3 || record.Body.Research == nil {
		return fail(GovernanceUnavailable, "research registry is not available")
	}
	body := *record.Body.Research
	id, err := body.ID()
	if err != nil {
		return fail(Invalid, "invalid research admission payload")
	}
	if _, exists := state.Research[id.String()]; exists {
		return fail(RegistryExists, "research entry already exists")
	}
	if err := validateReferenceTargets(body.CanonicalRefs, state); err != nil {
		return err
	}
	state.Research[id.String()] = StoredResearch{ID: cloneResearchID(id), Body: research.CloneBody(body),
		ProposalID: cloneProposalID(record.ID), AdmittedHeight: height}
	return nil
}

func admitResource(state State, record governance.ProposalRecord, height int64) error {
	if state.SchemaVersion != StateSchemaV3 || record.Body.Resource == nil {
		return fail(GovernanceUnavailable, "resource registry is not available")
	}
	body := *record.Body.Resource
	id, err := body.ID()
	if err != nil {
		return fail(Invalid, "invalid resource admission payload")
	}
	if _, exists := state.Resources[id.String()]; exists {
		return fail(RegistryExists, "resource entry already exists")
	}
	if err := validateReferenceTargets(body.CanonicalRefs, state); err != nil {
		return err
	}
	state.Resources[id.String()] = StoredResource{ID: cloneResourceID(id), Body: resources.CloneBody(body),
		ProposalID: cloneProposalID(record.ID), AdmittedHeight: height}
	return nil
}

func validateReferenceTargets(references []registry.CanonicalReference, state State) error {
	for _, reference := range references {
		switch reference.TargetKind {
		case registry.TargetResearch:
			if _, exists := state.Research[reference.TargetID]; !exists {
				return fail(NotFound, "canonical research reference target not found")
			}
		case registry.TargetResource:
			if _, exists := state.Resources[reference.TargetID]; !exists {
				return fail(NotFound, "canonical resource reference target not found")
			}
		case registry.TargetObject:
			// Syntax and digest are checked by EntryBody.Validate. Object bytes
			// are deliberately not consulted by consensus.
		default:
			return fail(Invalid, "invalid canonical reference target")
		}
	}
	return nil
}

func snapshotRegistries(state State, snapshot *Snapshot) error {
	researchKeys := make([]string, 0, len(state.Research))
	for key := range state.Research {
		researchKeys = append(researchKeys, key)
	}
	sort.Strings(researchKeys)
	for _, key := range researchKeys {
		entry := state.Research[key]
		derived, err := entry.Body.ID()
		proposal, exists := state.Governance.Proposals[entry.ProposalID.String()]
		if err != nil || entry.ID.String() != key || derived.String() != key || entry.ProposalID.Validate() != nil ||
			entry.AdmittedHeight <= 0 || !exists || proposal.Status != governance.Executed || proposal.Body.Kind != governance.ResearchAdmission ||
			validateReferenceTargetsForSnapshot(entry.Body.CanonicalRefs, state) != nil {
			return fmt.Errorf("invalid stored research entry %q", key)
		}
		snapshot.Research = append(snapshot.Research, SnapshotResearch{ID: cloneResearchID(entry.ID), Body: research.CloneBody(entry.Body),
			ProposalID: cloneProposalID(entry.ProposalID), AdmittedHeight: entry.AdmittedHeight})
	}
	resourceKeys := make([]string, 0, len(state.Resources))
	for key := range state.Resources {
		resourceKeys = append(resourceKeys, key)
	}
	sort.Strings(resourceKeys)
	for _, key := range resourceKeys {
		entry := state.Resources[key]
		derived, err := entry.Body.ID()
		proposal, exists := state.Governance.Proposals[entry.ProposalID.String()]
		if err != nil || entry.ID.String() != key || derived.String() != key || entry.ProposalID.Validate() != nil ||
			entry.AdmittedHeight <= 0 || !exists || proposal.Status != governance.Executed || proposal.Body.Kind != governance.ResourceAdmission ||
			validateReferenceTargetsForSnapshot(entry.Body.CanonicalRefs, state) != nil {
			return fmt.Errorf("invalid stored resource entry %q", key)
		}
		snapshot.Resources = append(snapshot.Resources, SnapshotResource{ID: cloneResourceID(entry.ID), Body: resources.CloneBody(entry.Body),
			ProposalID: cloneProposalID(entry.ProposalID), AdmittedHeight: entry.AdmittedHeight})
	}
	return nil
}

func validateReferenceTargetsForSnapshot(references []registry.CanonicalReference, state State) error {
	for _, reference := range references {
		switch reference.TargetKind {
		case registry.TargetResearch:
			if _, exists := state.Research[reference.TargetID]; !exists {
				return fmt.Errorf("missing research target")
			}
		case registry.TargetResource:
			if _, exists := state.Resources[reference.TargetID]; !exists {
				return fmt.Errorf("missing resource target")
			}
		case registry.TargetObject:
		default:
			return fmt.Errorf("invalid target kind")
		}
	}
	return nil
}

func restoreRegistries(snapshot Snapshot, state *State) error {
	if snapshot.SchemaVersion != StateSchemaV3 {
		if len(snapshot.Research) != 0 || len(snapshot.Resources) != 0 {
			return fmt.Errorf("registry snapshot requires state schema v3")
		}
		return nil
	}
	state.Research = make(map[string]StoredResearch, len(snapshot.Research))
	state.Resources = make(map[string]StoredResource, len(snapshot.Resources))
	for _, item := range snapshot.Research {
		key := item.ID.String()
		if key == "" || item.ProposalID.Validate() != nil {
			return fmt.Errorf("invalid snapshot research entry")
		}
		if _, exists := state.Research[key]; exists {
			return fmt.Errorf("duplicate snapshot research entry")
		}
		state.Research[key] = StoredResearch{ID: cloneResearchID(item.ID), Body: research.CloneBody(item.Body),
			ProposalID: cloneProposalID(item.ProposalID), AdmittedHeight: item.AdmittedHeight}
	}
	for _, item := range snapshot.Resources {
		key := item.ID.String()
		if key == "" || item.ProposalID.Validate() != nil {
			return fmt.Errorf("invalid snapshot resource entry")
		}
		if _, exists := state.Resources[key]; exists {
			return fmt.Errorf("duplicate snapshot resource entry")
		}
		state.Resources[key] = StoredResource{ID: cloneResourceID(item.ID), Body: resources.CloneBody(item.Body),
			ProposalID: cloneProposalID(item.ProposalID), AdmittedHeight: item.AdmittedHeight}
	}
	return nil
}

func cloneStoredResearch(entry StoredResearch) StoredResearch {
	entry.ID = cloneResearchID(entry.ID)
	entry.Body = research.CloneBody(entry.Body)
	entry.ProposalID = cloneProposalID(entry.ProposalID)
	return entry
}

func cloneStoredResource(entry StoredResource) StoredResource {
	entry.ID = cloneResourceID(entry.ID)
	entry.Body = resources.CloneBody(entry.Body)
	entry.ProposalID = cloneProposalID(entry.ProposalID)
	return entry
}

func cloneResearchID(id research.ID) research.ID {
	return research.ID{HashDigest: registry.CloneHash(id.HashDigest)}
}

func cloneResourceID(id resources.ID) resources.ID {
	return resources.ID{HashDigest: registry.CloneHash(id.HashDigest)}
}

func cloneProposalID(id governance.ProposalID) governance.ProposalID {
	return governance.ProposalID{HashDigest: protocol.HashDigest{Algorithm: id.Algorithm, Digest: append([]byte(nil), id.Digest...)}}
}
