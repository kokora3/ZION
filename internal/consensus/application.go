package consensus

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sync"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/kokora3/zion/internal/chain"
	"github.com/kokora3/zion/internal/governance"
)

const (
	ApplicationVersion uint64 = 1
	Codespace                 = "zion"
	CodeOK             uint32 = 0
	CodeMalformed      uint32 = 1
	CodeRejected       uint32 = 2
	CodeInvariant      uint32 = 3
)

type Status struct {
	Height   int64
	AppHash  []byte
	Snapshot chain.Snapshot
	Pending  bool
}

type pendingBlock struct {
	height int64
	state  chain.State
}

// Application implements CometBFT's current ABCI interface. Consensus height
// and engine metadata remain outside canonical ZION State.
type Application struct {
	abci.BaseApplication
	mu        sync.RWMutex
	genesis   Genesis
	committed chain.State
	height    int64
	pending   *pendingBlock
}

func NewApplication(genesis Genesis) (*Application, error) {
	state := genesis.InitialState
	hash, err := state.Hash()
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(hash.Digest, genesis.StateHash.Digest) {
		return nil, fmt.Errorf("application genesis StateHash mismatch")
	}
	return &Application{genesis: genesis, committed: state}, nil
}

func (app *Application) Info(context.Context, *abci.InfoRequest) (*abci.InfoResponse, error) {
	app.mu.RLock()
	defer app.mu.RUnlock()
	hash, err := app.committed.Hash()
	if err != nil {
		return nil, fmt.Errorf("committed state invariant: %w", err)
	}
	return &abci.InfoResponse{Data: "zion", Version: "zion-protocol-v0.1", AppVersion: ApplicationVersion,
		LastBlockHeight: app.height, LastBlockAppHash: append([]byte(nil), hash.Digest...)}, nil
}

func (app *Application) InitChain(_ context.Context, req *abci.InitChainRequest) (*abci.InitChainResponse, error) {
	app.mu.Lock()
	defer app.mu.Unlock()
	if req.ChainId != app.genesis.ChainID {
		return nil, fmt.Errorf("consensus chain ID mismatch")
	}
	actualGenesis, err := decodeAppGenesis(req.AppStateBytes)
	if err != nil {
		return nil, fmt.Errorf("invalid application genesis: %w", err)
	}
	expectedGenesis, err := decodeAppGenesis(app.genesis.AppState)
	if err != nil || !reflect.DeepEqual(actualGenesis, expectedGenesis) {
		return nil, fmt.Errorf("application genesis identity mismatch")
	}
	if len(req.Validators) != len(app.genesis.Validators) {
		return nil, fmt.Errorf("initial validator set mismatch")
	}
	expected := make(map[string]int64, len(app.genesis.Validators))
	for _, validator := range app.genesis.Validators {
		expected[string(validator.PublicKey)] = validator.Power
	}
	for _, update := range req.Validators {
		if update.PubKeyType != "ed25519" || expected[string(update.PubKeyBytes)] != update.Power {
			return nil, fmt.Errorf("initial validator set mismatch")
		}
		delete(expected, string(update.PubKeyBytes))
	}
	if len(expected) != 0 {
		return nil, fmt.Errorf("initial validator set mismatch")
	}
	hash, err := app.committed.Hash()
	if err != nil {
		return nil, fmt.Errorf("genesis state invariant: %w", err)
	}
	return &abci.InitChainResponse{AppHash: append([]byte(nil), hash.Digest...)}, nil
}

func decodeAppGenesis(raw []byte) (appGenesis, error) {
	var result appGenesis
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return appGenesis{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return appGenesis{}, fmt.Errorf("trailing genesis data")
	}
	return result, nil
}

func (app *Application) CheckTx(_ context.Context, req *abci.CheckTxRequest) (*abci.CheckTxResponse, error) {
	app.mu.RLock()
	defer app.mu.RUnlock()
	_, _, code, data, _ := app.evaluate(app.committed, req.Tx, app.height+1)
	return &abci.CheckTxResponse{Code: code, Data: data, Codespace: codespace(code),
		GasWanted: int64(len(req.Tx)), GasUsed: int64(len(req.Tx))}, nil
}

func (app *Application) PrepareProposal(_ context.Context, req *abci.PrepareProposalRequest) (*abci.PrepareProposalResponse, error) {
	app.mu.RLock()
	defer app.mu.RUnlock()
	working := app.committed
	selected := make([][]byte, 0, len(req.Txs))
	var total int64
	for _, raw := range req.Txs {
		if req.MaxTxBytes >= 0 && total+int64(len(raw)) > req.MaxTxBytes {
			continue
		}
		next, err, code, _, _ := app.evaluate(working, raw, req.Height)
		if err != nil || code != CodeOK {
			continue
		}
		working = next
		selected = append(selected, append([]byte(nil), raw...))
		total += int64(len(raw))
	}
	return &abci.PrepareProposalResponse{Txs: selected}, nil
}

func (app *Application) ProcessProposal(_ context.Context, req *abci.ProcessProposalRequest) (*abci.ProcessProposalResponse, error) {
	app.mu.RLock()
	defer app.mu.RUnlock()
	working := app.committed
	for _, raw := range req.Txs {
		next, err, code, _, _ := app.evaluate(working, raw, req.Height)
		if err != nil || code != CodeOK {
			return &abci.ProcessProposalResponse{Status: abci.PROCESS_PROPOSAL_STATUS_REJECT}, nil
		}
		working = next
	}
	return &abci.ProcessProposalResponse{Status: abci.PROCESS_PROPOSAL_STATUS_ACCEPT}, nil
}

func (app *Application) FinalizeBlock(_ context.Context, req *abci.FinalizeBlockRequest) (*abci.FinalizeBlockResponse, error) {
	app.mu.Lock()
	defer app.mu.Unlock()
	if app.pending != nil {
		return nil, fmt.Errorf("application invariant: block already pending commit")
	}
	if req.Height != app.height+1 {
		return nil, fmt.Errorf("application invariant: height %d after %d", req.Height, app.height)
	}
	working := app.committed
	results := make([]*abci.ExecTxResult, 0, len(req.Txs))
	validatorUpdates := make([]abci.ValidatorUpdate, 0)
	for _, raw := range req.Txs {
		next, _, code, data, updates := app.evaluate(working, raw, req.Height)
		results = append(results, &abci.ExecTxResult{Code: code, Data: data, Codespace: codespace(code),
			GasWanted: int64(len(raw)), GasUsed: int64(len(raw))})
		if code == CodeOK {
			working = next
			for _, update := range updates {
				validatorUpdates = append(validatorUpdates, abci.ValidatorUpdate{
					Power: update.Power, PubKeyBytes: append([]byte(nil), update.PublicKey...), PubKeyType: "ed25519",
				})
			}
		}
	}
	hash, err := working.Hash()
	if err != nil {
		return nil, fmt.Errorf("application invariant: resulting state: %w", err)
	}
	app.pending = &pendingBlock{height: req.Height, state: working}
	return &abci.FinalizeBlockResponse{TxResults: results, AppHash: append([]byte(nil), hash.Digest...),
		ValidatorUpdates: validatorUpdates}, nil
}

func (app *Application) Commit(context.Context, *abci.CommitRequest) (*abci.CommitResponse, error) {
	app.mu.Lock()
	defer app.mu.Unlock()
	if app.pending == nil {
		return nil, fmt.Errorf("application invariant: commit without pending block")
	}
	app.committed = app.pending.state
	app.height = app.pending.height
	app.pending = nil
	return &abci.CommitResponse{}, nil
}

func (app *Application) Status() (Status, error) {
	app.mu.RLock()
	defer app.mu.RUnlock()
	hash, err := app.committed.Hash()
	if err != nil {
		return Status{}, err
	}
	snapshot, err := app.committed.Snapshot()
	if err != nil {
		return Status{}, err
	}
	return Status{Height: app.height, AppHash: append([]byte(nil), hash.Digest...), Snapshot: snapshot,
		Pending: app.pending != nil}, nil
}

func (app *Application) evaluate(state chain.State, raw []byte, height int64) (chain.State, error, uint32, []byte, []governance.ValidatorUpdate) {
	tx, err := DecodeTransaction(raw, app.genesis.NetworkID)
	if err != nil {
		return state, err, CodeMalformed, []byte(decodeKind(err)), nil
	}
	next, receipt, err := chain.ApplyWithContext(state, tx, chain.ExecutionContext{Height: height})
	if err != nil {
		return state, err, CodeRejected, []byte(receipt.Code), nil
	}
	return next, nil, CodeOK, []byte(receipt.Code), receipt.ValidatorUpdates
}

func decodeKind(err error) DecodeErrorKind {
	var decode *DecodeError
	if errors.As(err, &decode) {
		return decode.Kind
	}
	return DecodeMalformed
}

func codespace(code uint32) string {
	if code == CodeOK {
		return ""
	}
	return Codespace
}
