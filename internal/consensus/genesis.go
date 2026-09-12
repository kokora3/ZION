package consensus

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	cmted25519 "github.com/cometbft/cometbft/crypto/ed25519"
	cmttypes "github.com/cometbft/cometbft/types"
	"github.com/kokora3/zion/internal/chain"
	"github.com/kokora3/zion/internal/protocol"
)

const (
	GenesisSchema         protocol.SchemaVersion = 1
	DefaultValidatorPower int64                  = 1
	DefaultValidatorCount                        = 4
)

// Validator is public consensus configuration. It is deliberately unrelated
// to member IdentityID and contains no consensus private key.
type Validator struct {
	Name      string `cbor:"1,keyasint"`
	PublicKey []byte `cbor:"2,keyasint"`
	Power     int64  `cbor:"3,keyasint"`
}

type Genesis struct {
	NetworkID        protocol.NetworkID
	StateHash        chain.StateHash
	ValidatorSetHash protocol.HashDigest
	GenesisID        protocol.HashDigest
	ChainID          string
	AppState         []byte
	Validators       []Validator
}

type appGenesis struct {
	SchemaVersion    protocol.SchemaVersion `json:"schema_version"`
	NetworkID        protocol.NetworkID     `json:"network_id"`
	StateHash        string                 `json:"state_hash"`
	ValidatorSetHash string                 `json:"validator_set_hash"`
}

func NewGenesis(network protocol.NetworkID, validators []Validator) (Genesis, error) {
	if network == "" {
		return Genesis{}, fmt.Errorf("empty network ID")
	}
	if len(validators) != DefaultValidatorCount {
		return Genesis{}, fmt.Errorf("validator count %d, want %d", len(validators), DefaultValidatorCount)
	}
	normalized := make([]Validator, len(validators))
	copy(normalized, validators)
	seenNames := make(map[string]struct{}, len(normalized))
	seenKeys := make(map[string]struct{}, len(normalized))
	for i := range normalized {
		normalized[i].PublicKey = append([]byte(nil), normalized[i].PublicKey...)
		if normalized[i].Name == "" || len(normalized[i].PublicKey) != ed25519.PublicKeySize {
			return Genesis{}, fmt.Errorf("invalid validator %d", i)
		}
		if normalized[i].Power != DefaultValidatorPower {
			return Genesis{}, fmt.Errorf("validator %q power %d, want equal power %d", normalized[i].Name, normalized[i].Power, DefaultValidatorPower)
		}
		key := hex.EncodeToString(normalized[i].PublicKey)
		if _, ok := seenKeys[key]; ok {
			return Genesis{}, fmt.Errorf("duplicate validator consensus key")
		}
		if _, ok := seenNames[normalized[i].Name]; ok {
			return Genesis{}, fmt.Errorf("duplicate validator name")
		}
		seenKeys[key], seenNames[normalized[i].Name] = struct{}{}, struct{}{}
	}
	sort.Slice(normalized, func(i, j int) bool { return bytes.Compare(normalized[i].PublicKey, normalized[j].PublicKey) < 0 })
	validatorHash, err := protocol.HashCanonical(normalized)
	if err != nil {
		return Genesis{}, fmt.Errorf("hash validator set: %w", err)
	}
	stateHash, err := chain.Genesis(network).Hash()
	if err != nil {
		return Genesis{}, fmt.Errorf("hash application genesis: %w", err)
	}
	doc := appGenesis{GenesisSchema, network, stateHash.String(),
		"sha256:" + hex.EncodeToString(validatorHash.Digest)}
	appState, err := json.Marshal(doc)
	if err != nil {
		return Genesis{}, err
	}
	genesisID := protocol.HashBytes(appState)
	chainID := fmt.Sprintf("%s-%s", network, hex.EncodeToString(genesisID.Digest[:6]))
	return Genesis{NetworkID: network, StateHash: stateHash, ValidatorSetHash: validatorHash,
		GenesisID: genesisID, ChainID: chainID, AppState: appState, Validators: normalized}, nil
}

// CometGenesis constructs engine configuration from public genesis data. The
// caller supplies time so fixture/test genesis can be completely deterministic.
func (g Genesis) CometGenesis(genesisTime time.Time) *cmttypes.GenesisDoc {
	validators := make([]cmttypes.GenesisValidator, len(g.Validators))
	for i, validator := range g.Validators {
		publicKey := cmted25519.PubKey(append([]byte(nil), validator.PublicKey...))
		validators[i] = cmttypes.GenesisValidator{Address: publicKey.Address(), PubKey: publicKey,
			Power: validator.Power, Name: validator.Name}
	}
	return &cmttypes.GenesisDoc{GenesisTime: genesisTime.UTC(), ChainID: g.ChainID, Validators: validators,
		AppHash: append([]byte(nil), g.StateHash.Digest...), AppState: json.RawMessage(append([]byte(nil), g.AppState...))}
}
