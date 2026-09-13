package consensus

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	cmted25519 "github.com/cometbft/cometbft/crypto/ed25519"
	cmttypes "github.com/cometbft/cometbft/types"
	"github.com/kokora3/zion/internal/chain"
	"github.com/kokora3/zion/internal/protocol"
)

// LoadGenesis reads and verifies the public CometBFT genesis document without
// loading any validator private material. The returned InitialState is only a
// placeholder; NewApplicationFromState receives the verified durable state.
func LoadGenesis(path string, network protocol.NetworkID) (Genesis, error) {
	doc, err := cmttypes.GenesisDocFromFile(path)
	if err != nil {
		return Genesis{}, err
	}
	if err := doc.ValidateAndComplete(); err != nil {
		return Genesis{}, err
	}
	application, err := decodeAppGenesis(doc.AppState)
	if err != nil || application.SchemaVersion != GenesisSchema || application.NetworkID != network {
		return Genesis{}, fmt.Errorf("invalid ZION application genesis")
	}
	stateHash, err := parseGenesisStateHash(application.StateHash)
	if err != nil || !bytes.Equal(stateHash.Digest, doc.AppHash) {
		return Genesis{}, fmt.Errorf("CometBFT AppHash differs from ZION genesis StateHash")
	}
	validators := make([]Validator, len(doc.Validators))
	for index, validator := range doc.Validators {
		if validator.PubKey == nil || validator.PubKey.Type() != "ed25519" {
			return Genesis{}, fmt.Errorf("validator %d does not use Ed25519", index)
		}
		validators[index] = Validator{Name: validator.Name, PublicKey: append([]byte(nil), validator.PubKey.Bytes()...), Power: validator.Power}
	}
	validatorGenesis, err := NewGenesis(network, validators)
	if err != nil {
		return Genesis{}, fmt.Errorf("invalid genesis validators: %w", err)
	}
	validatorHash := validatorGenesis.ValidatorSetHash
	if application.ValidatorSetHash != "sha256:"+hex.EncodeToString(validatorHash.Digest) {
		return Genesis{}, fmt.Errorf("application genesis validator-set hash mismatch")
	}
	canonicalAppState, err := json.Marshal(application)
	if err != nil {
		return Genesis{}, err
	}
	genesisID := protocol.HashBytes(canonicalAppState)
	expectedChainID := fmt.Sprintf("%s-%s", network, hex.EncodeToString(genesisID.Digest[:6]))
	if doc.ChainID != expectedChainID {
		return Genesis{}, fmt.Errorf("CometBFT chain ID differs from ZION genesis identity")
	}
	return Genesis{NetworkID: network, StateHash: stateHash, ValidatorSetHash: validatorHash,
		GenesisID: genesisID, ChainID: doc.ChainID, AppState: canonicalAppState,
		Validators: validatorGenesis.Validators, InitialState: chain.Genesis(network)}, nil
}

func parseGenesisStateHash(value string) (chain.StateHash, error) {
	const prefix = "zion:state:sha256:"
	if !strings.HasPrefix(value, prefix) || len(value) != len(prefix)+64 {
		return chain.StateHash{}, fmt.Errorf("invalid genesis StateHash")
	}
	digest, err := hex.DecodeString(strings.TrimPrefix(value, prefix))
	if err != nil || hex.EncodeToString(digest) != strings.TrimPrefix(value, prefix) {
		return chain.StateHash{}, fmt.Errorf("invalid genesis StateHash")
	}
	hash := chain.StateHash{HashDigest: protocol.HashDigest{Algorithm: protocol.HashAlgorithmSHA256, Digest: digest}}
	return hash, hash.Validate()
}

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
	InitialState     chain.State
}

type appGenesis struct {
	SchemaVersion    protocol.SchemaVersion `json:"schema_version"`
	NetworkID        protocol.NetworkID     `json:"network_id"`
	StateHash        string                 `json:"state_hash"`
	ValidatorSetHash string                 `json:"validator_set_hash"`
}

func NewGenesis(network protocol.NetworkID, validators []Validator) (Genesis, error) {
	return NewGenesisWithState(network, validators, chain.Genesis(network))
}

func NewGenesisWithState(network protocol.NetworkID, validators []Validator, initialState chain.State) (Genesis, error) {
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
	if initialState.NetworkID != network {
		return Genesis{}, fmt.Errorf("initial application state network mismatch")
	}
	stateHash, err := initialState.Hash()
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
		GenesisID: genesisID, ChainID: chainID, AppState: appState, Validators: normalized, InitialState: initialState}, nil
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
