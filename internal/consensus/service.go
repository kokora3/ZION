package consensus

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	cmtconfig "github.com/cometbft/cometbft/config"
	cmtjson "github.com/cometbft/cometbft/libs/json"
	cmtlog "github.com/cometbft/cometbft/libs/log"
	cbtnode "github.com/cometbft/cometbft/node"
	cmtp2p "github.com/cometbft/cometbft/p2p"
	"github.com/cometbft/cometbft/privval"
	"github.com/cometbft/cometbft/proxy"
	cmttypes "github.com/cometbft/cometbft/types"
	"github.com/kokora3/zion/internal/chain"
)

// Service owns one CometBFT node. CometBFT storage and transport remain
// separate from ZION application snapshots and general libp2p.
type Service struct {
	mu   sync.RWMutex
	cfg  *cmtconfig.Config
	app  *Application
	node *cbtnode.Node
}

func NewService(cfg *cmtconfig.Config, app *Application) (*Service, error) {
	if cfg == nil || app == nil {
		return nil, fmt.Errorf("CometBFT config and application are required")
	}
	return &Service{cfg: cfg, app: app}, nil
}

func (s *Service) SetCommitObserver(observer func(chain.State, int64, [][]byte) error) {
	s.app.SetCommitObserver(observer)
}

func (s *Service) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.node != nil {
		return fmt.Errorf("CometBFT service already started")
	}
	if err := validatePrivValidatorFiles(s.cfg.PrivValidatorKeyFile(), s.cfg.PrivValidatorStateFile()); err != nil {
		return err
	}
	filePV := privval.LoadFilePV(s.cfg.PrivValidatorKeyFile(), s.cfg.PrivValidatorStateFile())
	nodeKey, err := cmtp2p.LoadNodeKey(s.cfg.NodeKeyFile())
	if err != nil {
		return err
	}
	created, err := cbtnode.NewNode(ctx, s.cfg, filePV, nodeKey, proxy.NewLocalClientCreator(s.app),
		cbtnode.DefaultGenesisDocProviderFunc(s.cfg), cmtconfig.DefaultDBProvider,
		cbtnode.DefaultMetricsProvider(s.cfg.Instrumentation), cmtlog.NewNopLogger())
	if err != nil {
		return err
	}
	if err := created.Start(); err != nil {
		// CometBFT's BaseService clears its started flag when OnStart fails,
		// which makes Stop return ErrNotStarted even though NewNode has already
		// opened its stores and OnStart may have started partial resources.
		// Invoke the node cleanup hook directly so bind/start failures release
		// transports and database handles instead of leaking them until exit.
		created.OnStop()
		return err
	}
	s.node = created
	return nil
}

func validatePrivValidatorFiles(keyPath, statePath string) error {
	_, err := ValidatorPublicKey(keyPath)
	if err != nil {
		return err
	}
	stateData, err := os.ReadFile(statePath)
	if err != nil {
		return fmt.Errorf("read validator signing state: %w", err)
	}
	var state privval.FilePVLastSignState
	if err := cmtjson.Unmarshal(stateData, &state); err != nil {
		return fmt.Errorf("invalid validator signing state file")
	}
	return nil
}

// ValidatorPublicKey validates a CometBFT private-validator key file and
// returns only its public Ed25519 key. Private material never crosses this API.
func ValidatorPublicKey(keyPath string) ([]byte, error) {
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("read validator key: %w", err)
	}
	var key privval.FilePVKey
	if err := cmtjson.Unmarshal(keyData, &key); err != nil || key.PrivKey == nil || key.PrivKey.Type() != "ed25519" {
		return nil, fmt.Errorf("invalid validator key file")
	}
	return append([]byte(nil), key.PrivKey.PubKey().Bytes()...), nil
}

func (s *Service) Stop(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.node == nil {
		return nil
	}
	if s.node.Switch().IsRunning() {
		_ = s.node.Switch().Stop()
		s.node.Switch().Wait()
		time.Sleep(100 * time.Millisecond)
	}
	err := s.node.Stop()
	s.node.Wait()
	s.node = nil
	return err
}

func (s *Service) Node() *cbtnode.Node {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.node
}

func (s *Service) Submit(ctx context.Context, raw []byte) error {
	s.mu.RLock()
	current := s.node
	s.mu.RUnlock()
	if current == nil || !current.IsRunning() {
		return fmt.Errorf("CometBFT service is not active")
	}
	request, err := current.Mempool().CheckTx(cmttypes.Tx(append([]byte(nil), raw...)), "")
	if err != nil {
		return err
	}
	done := make(chan struct{})
	go func() { request.Wait(); close(done) }()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
	}
	response := request.Response.GetCheckTx()
	if response == nil || response.Code != CodeOK {
		return fmt.Errorf("CometBFT CheckTx rejected transaction")
	}
	return nil
}

func (s *Service) Active() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.node != nil && s.node.IsRunning()
}
