package node

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/kokora3/zion/internal/api"
	"github.com/kokora3/zion/internal/chain"
	"github.com/kokora3/zion/internal/consensus"
	"github.com/kokora3/zion/internal/protocol"
	libnetwork "github.com/libp2p/go-libp2p/core/network"
	libpeer "github.com/libp2p/go-libp2p/core/peer"
)

const serviceTimeout = 10 * time.Second

func (r *Runtime) installStreamHandlers() {
	r.p2p.Host().SetStreamHandler(TxProtocolID, r.handleRelay)
	r.p2p.Host().SetStreamHandler(StateProtocolID, r.handleState)
}

func (r *Runtime) discoverAndSync() {
	discoveryContext, cancel := context.WithTimeout(r.ctx, 30*time.Second)
	if err := r.p2p.Discover(discoveryContext); err != nil {
		r.cfg.Logger.Warn("P2P discovery completed without a candidate", "error", err)
	}
	cancel()
	if r.cfg.FreshStateIsAuthoritative {
		return
	}
	delay := time.Duration(0)
	for {
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-r.ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
		syncContext, stop := context.WithTimeout(r.ctx, r.cfg.SyncTimeout)
		err := r.Synchronize(syncContext)
		stop()
		if err == nil {
			r.cfg.Logger.Debug("state synchronization completed")
			delay = r.cfg.SyncInterval
		} else if delay < r.cfg.SyncInterval {
			r.cfg.Logger.Warn("state synchronization failed", "error", err)
			delay = r.cfg.SyncInterval
		} else {
			delay = min(delay*2, time.Minute)
		}
	}
}

func (r *Runtime) SubmitTransaction(ctx context.Context, raw []byte) (api.Submission, error) {
	tx, err := consensus.DecodeTransaction(raw, r.cfg.NetworkID)
	if err != nil {
		return api.Submission{Status: string(RelayRejected)}, err
	}
	txID, err := tx.ID()
	if err != nil {
		return api.Submission{Status: string(RelayRejected)}, err
	}
	key := txID.String()
	r.mu.Lock()
	if previous, exists := r.tx[key]; exists {
		r.mu.Unlock()
		_ = previous
		return api.Submission{TxID: key, Status: string(RelayKnown)}, nil
	}
	state, height, syncStatus := cloneState(r.state), r.height, r.sync
	r.mu.Unlock()
	if syncStatus == Synced {
		if _, _, err := chain.ApplyWithContext(state, tx, chain.ExecutionContext{Height: height + 1}); err != nil {
			return api.Submission{TxID: key, Status: string(RelayRejected)}, err
		}
	}
	r.mu.RLock()
	consensusService := r.consensus
	r.mu.RUnlock()
	if consensusService != nil && consensusService.Active() {
		if err := consensusService.Submit(ctx, raw); err != nil {
			return api.Submission{TxID: key, Status: string(RelayRejected)}, err
		}
		r.mu.Lock()
		r.rememberPending(TransactionRecord{TxID: key, Status: "SUBMITTED_TO_CONSENSUS"})
		r.mu.Unlock()
		return api.Submission{TxID: key, Status: "SUBMITTED_TO_CONSENSUS"}, nil
	}
	if err := r.relayToPeer(ctx, raw); err != nil {
		return api.Submission{TxID: key, Status: string(RelayRejected)}, err
	}
	r.mu.Lock()
	r.rememberPending(TransactionRecord{TxID: key, Status: string(RelayAccepted)})
	r.mu.Unlock()
	return api.Submission{TxID: key, Status: string(RelayAccepted)}, nil
}

func (r *Runtime) relayToPeer(ctx context.Context, raw []byte) error {
	message, err := EncodeTxRelay(TxRelayMessage{SchemaVersion: RuntimeWireSchema, Transaction: raw}, r.cfg.NetworkID)
	if err != nil {
		return err
	}
	for _, remote := range r.p2p.UsablePeers() {
		streamCtx, cancel := context.WithTimeout(ctx, serviceTimeout)
		stream, err := r.p2p.Host().NewStream(streamCtx, remote.PeerID, TxProtocolID)
		if err == nil {
			_ = stream.SetDeadline(time.Now().Add(serviceTimeout))
			err = writeRuntimeFrame(stream, message, MaxTxFrame)
			if err == nil {
				var data []byte
				data, err = readRuntimeFrame(stream, 2048)
				if err == nil {
					var response TxRelayResponse
					err = decodeCanonicalBounded(data, 2048, &response)
					if err == nil && response.SchemaVersion == RuntimeWireSchema && (response.Status == RelayAccepted || response.Status == RelayKnown) {
						_ = stream.Close()
						cancel()
						return nil
					}
				}
			}
			_ = stream.Reset()
		}
		cancel()
	}
	return fmt.Errorf("no consensus-capable peer accepted transaction")
}

func (r *Runtime) handleRelay(stream libnetwork.Stream) {
	defer stream.Close()
	if !r.p2p.IsUsable(stream.Conn().RemotePeer()) {
		_ = stream.Reset()
		return
	}
	select {
	case r.relaySem <- struct{}{}:
		defer func() { <-r.relaySem }()
	default:
		_ = stream.Reset()
		return
	}
	_ = stream.SetDeadline(time.Now().Add(serviceTimeout))
	data, err := readRuntimeFrame(stream, MaxTxFrame)
	response := TxRelayResponse{SchemaVersion: RuntimeWireSchema, Status: RelayRejected}
	if err == nil {
		message, decodeErr := DecodeTxRelay(data, r.cfg.NetworkID)
		if decodeErr == nil {
			tx, _ := consensus.DecodeTransaction(message.Transaction, r.cfg.NetworkID)
			txID, _ := tx.ID()
			response.TxID = txID.String()
			r.mu.RLock()
			_, known := r.tx[response.TxID]
			r.mu.RUnlock()
			if known {
				response.Status = RelayKnown
			} else {
				r.mu.RLock()
				consensusService := r.consensus
				r.mu.RUnlock()
				if consensusService != nil && consensusService.Active() && consensusService.Submit(r.ctx, message.Transaction) == nil {
					response.Status = RelayAccepted
					r.mu.Lock()
					r.rememberPending(TransactionRecord{TxID: response.TxID, Status: "SUBMITTED_TO_CONSENSUS"})
					r.mu.Unlock()
				}
			}
		}
	}
	encoded, _ := encodeBounded(response, 2048)
	_ = writeRuntimeFrame(stream, encoded, 2048)
}

func (r *Runtime) currentOffer() (StateSnapshotOffer, error) {
	r.mu.RLock()
	state, height := cloneState(r.state), r.height
	finalized := make([]FinalityNotice, 0, min(256, len(r.txOrder)))
	start := max(0, len(r.txOrder)-256)
	for _, key := range r.txOrder[start:] {
		record := r.tx[key]
		if record.Status == "FINALIZED" {
			finalized = append(finalized, FinalityNotice{TxID: key, Height: record.CommittedHeight,
				ResultCode: record.ResultCode, StateHash: record.StateHash})
		}
	}
	r.mu.RUnlock()
	canonical, err := state.CanonicalBytes()
	if err != nil {
		return StateSnapshotOffer{}, err
	}
	hash, err := state.Hash()
	if err != nil {
		return StateSnapshotOffer{}, err
	}
	return StateSnapshotOffer{SchemaVersion: RuntimeWireSchema, NetworkID: r.cfg.NetworkID,
		NetworkFingerprint: r.cfg.GenesisID, Height: height, StateHash: hash, CanonicalState: canonical,
		Finalized: finalized}, nil
}

func (r *Runtime) handleState(stream libnetwork.Stream) {
	defer stream.Close()
	if !r.p2p.IsUsable(stream.Conn().RemotePeer()) {
		_ = stream.Reset()
		return
	}
	select {
	case r.syncSem <- struct{}{}:
		defer func() { <-r.syncSem }()
	default:
		_ = stream.Reset()
		return
	}
	_ = stream.SetDeadline(time.Now().Add(serviceTimeout))
	data, err := readRuntimeFrame(stream, 1024)
	var request StateSyncRequest
	if err != nil || decodeCanonicalBounded(data, 1024, &request) != nil || request.SchemaVersion != RuntimeWireSchema {
		_ = stream.Reset()
		return
	}
	offer, err := r.currentOffer()
	if err != nil {
		_ = stream.Reset()
		return
	}
	encoded, err := EncodeStateOffer(offer)
	if err != nil {
		_ = stream.Reset()
		return
	}
	_ = writeRuntimeFrame(stream, encoded, MaxStateFrame)
}

func (r *Runtime) requestState(ctx context.Context, peerID libpeer.ID) (StateSnapshotOffer, error) {
	streamCtx, cancel := context.WithTimeout(ctx, serviceTimeout)
	defer cancel()
	stream, err := r.p2p.Host().NewStream(streamCtx, peerID, StateProtocolID)
	if err != nil {
		return StateSnapshotOffer{}, err
	}
	defer stream.Close()
	_ = stream.SetDeadline(time.Now().Add(serviceTimeout))
	request, _ := encodeBounded(StateSyncRequest{SchemaVersion: RuntimeWireSchema}, 1024)
	if err := writeRuntimeFrame(stream, request, 1024); err != nil {
		return StateSnapshotOffer{}, err
	}
	data, err := readRuntimeFrame(stream, MaxStateFrame)
	if err != nil {
		return StateSnapshotOffer{}, err
	}
	return DecodeStateOffer(data)
}

func (r *Runtime) verifyOffer(offer StateSnapshotOffer) (chain.State, error) {
	if offer.NetworkID != r.cfg.NetworkID || !equalHash(offer.NetworkFingerprint, r.cfg.GenesisID) {
		return chain.State{}, fmt.Errorf("state offer network or genesis mismatch")
	}
	var snapshot chain.Snapshot
	if err := protocol.CanonicalDecode(offer.CanonicalState, &snapshot); err != nil {
		return chain.State{}, err
	}
	state, err := chain.StateFromSnapshot(snapshot)
	if err != nil {
		return chain.State{}, err
	}
	canonical, err := state.CanonicalBytes()
	if err != nil || !bytes.Equal(canonical, offer.CanonicalState) {
		return chain.State{}, fmt.Errorf("state offer is not canonical")
	}
	hash, err := state.Hash()
	if err != nil || hash.String() != offer.StateHash.String() {
		return chain.State{}, fmt.Errorf("state offer hash mismatch")
	}
	return state, nil
}

func (r *Runtime) Synchronize(ctx context.Context) error {
	r.syncMu.Lock()
	defer r.syncMu.Unlock()
	r.mu.Lock()
	if r.consensus != nil && r.consensus.Active() {
		r.mu.Unlock()
		return fmt.Errorf("live validator state cannot be replaced by general P2P sync")
	}
	r.sync = Syncing
	r.mu.Unlock()
	var best *StateSnapshotOffer
	var bestState chain.State
	for _, remote := range r.p2p.UsablePeers() {
		offer, err := r.requestState(ctx, remote.PeerID)
		if err != nil {
			continue
		}
		state, err := r.verifyOffer(offer)
		if err != nil {
			continue
		}
		if best == nil || offer.Height > best.Height {
			copyOffer := offer
			best = &copyOffer
			bestState = state
		} else if offer.Height == best.Height && offer.StateHash.String() != best.StateHash.String() {
			r.mu.Lock()
			r.sync = SyncError
			r.mu.Unlock()
			return fmt.Errorf("conflicting state hashes at equal height %d", offer.Height)
		}
	}
	if best == nil {
		r.mu.Lock()
		r.sync = SyncError
		r.mu.Unlock()
		return fmt.Errorf("no valid state snapshot source")
	}
	return r.installOffer(*best, bestState)
}

func (r *Runtime) installOffer(offer StateSnapshotOffer, offeredState chain.State) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if offer.Height < r.height {
		r.sync = Stale
		return fmt.Errorf("state snapshot rollback from %d to %d rejected", r.height, offer.Height)
	}
	currentHash, _ := r.state.Hash()
	if offer.Height == r.height {
		if currentHash.String() != offer.StateHash.String() {
			r.sync = SyncError
			return fmt.Errorf("conflicting state hash at current height")
		}
		if !r.persisted {
			if err := r.store.Save(offeredState, offer.Height); err != nil {
				r.sync = SyncError
				return err
			}
			r.persisted = true
		}
		for _, notice := range offer.Finalized {
			r.remember(TransactionRecord{TxID: notice.TxID, Status: "FINALIZED", CommittedHeight: notice.Height,
				ResultCode: notice.ResultCode, StateHash: notice.StateHash})
		}
		r.sync = Synced
		return nil
	}
	if err := r.store.Save(offeredState, offer.Height); err != nil {
		r.sync = SyncError
		return err
	}
	r.state, r.height, r.sync, r.persisted = offeredState, offer.Height, Synced, true
	r.cfg.Logger.Info("state snapshot installed", "height", offer.Height, "state_hash", offer.StateHash.String())
	_ = r.registryIndex.Rebuild(offeredState)
	for _, notice := range offer.Finalized {
		r.remember(TransactionRecord{TxID: notice.TxID, Status: "FINALIZED", CommittedHeight: notice.Height,
			ResultCode: notice.ResultCode, StateHash: notice.StateHash})
	}
	return nil
}
