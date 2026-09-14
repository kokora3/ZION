package node

import "github.com/kokora3/zion/internal/governance"

// GovernanceProposalView is the stable JSON projection exposed to replaceable
// clients. Typed protocol IDs remain strings here; clients must not reconstruct
// identifiers from Go's embedded digest representation.
type GovernanceProposalView struct {
	ProposalID      string                    `json:"proposal_id"`
	Kind            governance.ProposalKind   `json:"kind"`
	Proposer        string                    `json:"proposer"`
	Status          governance.ProposalStatus `json:"status"`
	StartHeight     int64                     `json:"start_height"`
	EndHeight       int64                     `json:"end_height"`
	ElectorateCount int                       `json:"electorate_count"`
	Yes             uint64                    `json:"yes"`
	No              uint64                    `json:"no"`
	Abstain         uint64                    `json:"abstain"`
	Participants    uint64                    `json:"participants"`
	Votes           []GovernanceVoteView      `json:"votes"`
	Payload         any                       `json:"payload,omitempty"`
}

type GovernanceVoteView struct {
	Voter  string                `json:"voter"`
	Choice governance.VoteChoice `json:"choice"`
	KeyID  string                `json:"key_id"`
}

func governanceProposalView(snapshot governance.ProposalSnapshot) GovernanceProposalView {
	tally := governance.Tally{}
	votes := make([]GovernanceVoteView, len(snapshot.Votes))
	for position, vote := range snapshot.Votes {
		votes[position] = GovernanceVoteView{Voter: vote.Voter.String(), Choice: vote.Choice, KeyID: vote.KeyID.String()}
		tally.Participants++
		switch vote.Choice {
		case governance.Yes:
			tally.Yes++
		case governance.No:
			tally.No++
		case governance.Abstain:
			tally.Abstain++
		}
	}
	view := GovernanceProposalView{ProposalID: snapshot.ID.String(), Kind: snapshot.Body.Kind,
		Proposer: snapshot.Body.Proposer.String(), Status: snapshot.Status, StartHeight: snapshot.StartHeight,
		EndHeight: snapshot.EndHeight, ElectorateCount: len(snapshot.Electorate), Yes: tally.Yes, No: tally.No,
		Abstain: tally.Abstain, Participants: tally.Participants, Votes: votes}
	switch snapshot.Body.Kind {
	case governance.ResearchAdmission:
		view.Payload = snapshot.Body.Research
	case governance.ResourceAdmission:
		view.Payload = snapshot.Body.Resource
	case governance.MembershipChange:
		if snapshot.Body.Membership != nil {
			view.Payload = map[string]any{"target": snapshot.Body.Membership.Target.String(), "expected": snapshot.Body.Membership.Expected, "requested": snapshot.Body.Membership.Requested}
		}
	case governance.ValidatorSetChange:
		if snapshot.Body.ValidatorSet != nil {
			view.Payload = map[string]any{"action": snapshot.Body.ValidatorSet.Action, "operator": snapshot.Body.ValidatorSet.Validator.Operator.String(), "power": snapshot.Body.ValidatorSet.Validator.Power}
		}
	case governance.ProtocolUpgrade:
		view.Payload = snapshot.Body.Upgrade
	case governance.Migration:
		view.Payload = snapshot.Body.Migration
	}
	return view
}
