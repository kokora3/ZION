package governance

import (
	"crypto/ed25519"
	"fmt"

	"github.com/kokora3/zion/internal/identity"
	"github.com/kokora3/zion/internal/protocol"
)

const (
	proposalDomain = "zion.governance.proposal"
	voteDomain     = "zion.governance.vote"
	finalizeDomain = "zion.governance.finalize"
	executeDomain  = "zion.governance.execute"
)

type signingEnvelope struct {
	Domain        string                 `cbor:"1,keyasint"`
	SchemaVersion protocol.SchemaVersion `cbor:"2,keyasint"`
	NetworkID     protocol.NetworkID     `cbor:"3,keyasint"`
	Payload       any                    `cbor:"4,keyasint"`
}

func sign(domain string, network protocol.NetworkID, payload any, privateKey ed25519.PrivateKey) (identity.Signature, error) {
	publicKey := identity.PublicFromPrivate(privateKey)
	keyID, err := identity.DeriveKeyID(publicKey)
	if err != nil {
		return identity.Signature{}, err
	}
	canonical, err := protocol.CanonicalEncode(signingEnvelope{domain, Schema, network, payload})
	if err != nil {
		return identity.Signature{}, err
	}
	return identity.Signature{Algorithm: identity.Ed25519, KeyID: keyID, Bytes: ed25519.Sign(privateKey, canonical)}, nil
}

func verify(domain string, network protocol.NetworkID, payload any, publicKey identity.PublicKey, signature identity.Signature) error {
	if err := publicKey.Validate(); err != nil {
		return err
	}
	if err := signature.Validate(); err != nil {
		return err
	}
	keyID, err := identity.DeriveKeyID(publicKey)
	if err != nil || keyID.String() != signature.KeyID.String() {
		return fmt.Errorf("governance signer key mismatch")
	}
	canonical, err := protocol.CanonicalEncode(signingEnvelope{domain, Schema, network, payload})
	if err != nil {
		return err
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey.Key), canonical, signature.Bytes) {
		return fmt.Errorf("invalid governance signature")
	}
	return nil
}

func CreateProposal(body ProposalBody, privateKey ed25519.PrivateKey) (ProposalAuthorization, error) {
	id, err := body.ID()
	if err != nil {
		return ProposalAuthorization{}, err
	}
	signature, err := sign(proposalDomain, body.NetworkID, body, privateKey)
	return ProposalAuthorization{Body: body, ProposalID: id, Signature: signature}, err
}

func VerifyProposal(proposal ProposalAuthorization, publicKey identity.PublicKey) error {
	id, err := proposal.Body.ID()
	if err != nil || id.String() != proposal.ProposalID.String() {
		return fmt.Errorf("proposal ID mismatch")
	}
	return verify(proposalDomain, proposal.Body.NetworkID, proposal.Body, publicKey, proposal.Signature)
}

func CreateVote(body VoteBody, privateKey ed25519.PrivateKey) (VoteAuthorization, error) {
	if body.SchemaVersion != Schema || body.NetworkID == "" || body.ProposalID.Validate() != nil || body.Voter.Validate() != nil || body.Choice.Validate() != nil {
		return VoteAuthorization{}, fmt.Errorf("invalid vote body")
	}
	signature, err := sign(voteDomain, body.NetworkID, body, privateKey)
	return VoteAuthorization{Body: body, Signature: signature}, err
}

func VerifyVote(vote VoteAuthorization, publicKey identity.PublicKey) error {
	if vote.Body.SchemaVersion != Schema || vote.Body.ProposalID.Validate() != nil || vote.Body.Voter.Validate() != nil || vote.Body.Choice.Validate() != nil {
		return fmt.Errorf("invalid vote body")
	}
	return verify(voteDomain, vote.Body.NetworkID, vote.Body, publicKey, vote.Signature)
}

func CreateFinalize(body ActionBody, privateKey ed25519.PrivateKey) (ActionAuthorization, error) {
	return createAction(finalizeDomain, body, privateKey)
}

func VerifyFinalize(action ActionAuthorization, publicKey identity.PublicKey) error {
	return verifyAction(finalizeDomain, action, publicKey)
}

func CreateExecute(body ActionBody, privateKey ed25519.PrivateKey) (ActionAuthorization, error) {
	return createAction(executeDomain, body, privateKey)
}

func VerifyExecute(action ActionAuthorization, publicKey identity.PublicKey) error {
	return verifyAction(executeDomain, action, publicKey)
}

func createAction(domain string, body ActionBody, privateKey ed25519.PrivateKey) (ActionAuthorization, error) {
	if err := validateActionBody(body); err != nil {
		return ActionAuthorization{}, err
	}
	signature, err := sign(domain, body.NetworkID, body, privateKey)
	return ActionAuthorization{Body: body, Signature: signature}, err
}

func verifyAction(domain string, action ActionAuthorization, publicKey identity.PublicKey) error {
	if err := validateActionBody(action.Body); err != nil {
		return err
	}
	return verify(domain, action.Body.NetworkID, action.Body, publicKey, action.Signature)
}

func validateActionBody(body ActionBody) error {
	if body.SchemaVersion != Schema || body.NetworkID == "" || body.ProposalID.Validate() != nil || body.Submitter.Validate() != nil {
		return fmt.Errorf("invalid governance action body")
	}
	return nil
}
