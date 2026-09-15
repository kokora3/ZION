package p2p

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	libpeer "github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

const peerCacheVersion = 1

type PeerSource string

const (
	SourceCache     PeerSource = "CACHE"
	SourceBootstrap PeerSource = "BOOTSTRAP"
	SourceFallback  PeerSource = "FALLBACK"
	SourceManual    PeerSource = "MANUAL"
	SourcePEX       PeerSource = "PEX"
	SourceInbound   PeerSource = "INBOUND"
)

type CachedPeer struct {
	PeerID            string     `json:"peer_id"`
	Addresses         []string   `json:"addresses"`
	Source            PeerSource `json:"source"`
	LastSuccessUnixMS int64      `json:"last_success_unix_ms,omitempty"`
	Failures          uint8      `json:"failures,omitempty"`
	NextAttemptUnixMS int64      `json:"next_attempt_unix_ms,omitempty"`
	Successful        bool       `json:"successful"`
}

type peerCacheFile struct {
	Version int          `json:"version"`
	Peers   []CachedPeer `json:"peers"`
}

type PeerCache struct {
	mu      sync.RWMutex
	path    string
	limits  Limits
	entries map[libpeer.ID]CachedPeer
	now     func() time.Time
	jitter  func(time.Duration) time.Duration
}

func OpenPeerCache(path string, limits Limits) (*PeerCache, error) {
	cache := &PeerCache{path: path, limits: limits, entries: make(map[libpeer.ID]CachedPeer), now: time.Now, jitter: randomJitter}
	data, err := readBoundedFile(path, MaxPeerCacheFileSize)
	if os.IsNotExist(err) {
		return cache, nil
	}
	if err != nil {
		return cache, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var file peerCacheFile
	if err := decoder.Decode(&file); err != nil {
		return cache, fmt.Errorf("decode peer cache: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return cache, fmt.Errorf("peer cache contains trailing data")
	}
	if file.Version != peerCacheVersion || len(file.Peers) > limits.MaxCachedPeers {
		return cache, fmt.Errorf("unsupported or oversized peer cache")
	}
	for _, record := range file.Peers {
		peerID, normalized, err := normalizeCachedPeer(record, limits.MaxAddressesPerPeer)
		if err != nil {
			return cache, err
		}
		if existing, ok := cache.entries[peerID]; ok {
			normalized = mergeCachedPeers(existing, normalized, limits.MaxAddressesPerPeer)
		}
		cache.entries[peerID] = normalized
	}
	return cache, nil
}

func readBoundedFile(path string, maximum int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() < 0 || info.Size() > maximum {
		return nil, fmt.Errorf("file %q exceeds size bound", path)
	}
	data := make([]byte, info.Size())
	if _, err := io.ReadFull(file, data); err != nil {
		return nil, err
	}
	return data, nil
}

func normalizeCachedPeer(record CachedPeer, maxAddresses int) (libpeer.ID, CachedPeer, error) {
	peerID, err := libpeer.Decode(record.PeerID)
	if err != nil {
		return "", CachedPeer{}, fmt.Errorf("invalid cached PeerID: %w", err)
	}
	addresses, err := normalizeAddressStrings(record.Addresses, maxAddresses)
	if err != nil {
		return "", CachedPeer{}, err
	}
	if record.Failures > 32 {
		record.Failures = 32
	}
	switch record.Source {
	case SourceCache, SourceBootstrap, SourceFallback, SourceManual, SourcePEX, SourceInbound:
	default:
		return "", CachedPeer{}, fmt.Errorf("invalid cached peer source %q", record.Source)
	}
	record.PeerID = peerID.String()
	record.Addresses = addresses
	return peerID, record, nil
}

func normalizeAddressStrings(values []string, maximum int) ([]string, error) {
	if len(values) == 0 || len(values) > HardMaxAddressesPerPeer {
		return nil, fmt.Errorf("invalid address count %d", len(values))
	}
	seen := make(map[string]struct{}, len(values))
	addresses := make([]string, 0, len(values))
	for _, value := range values {
		if len(value) == 0 || len(value) > MaxMultiaddrSize {
			return nil, fmt.Errorf("invalid multiaddr size")
		}
		address, err := ma.NewMultiaddr(value)
		if err != nil {
			return nil, fmt.Errorf("invalid cached multiaddr: %w", err)
		}
		canonical := address.String()
		if _, exists := seen[canonical]; exists {
			continue
		}
		seen[canonical] = struct{}{}
		addresses = append(addresses, canonical)
	}
	sort.Strings(addresses)
	if len(addresses) > maximum {
		addresses = addresses[:maximum]
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("cached peer has no usable address")
	}
	return addresses, nil
}

func mergeCachedPeers(a, b CachedPeer, maximum int) CachedPeer {
	addresses := append(append([]string(nil), a.Addresses...), b.Addresses...)
	normalized, _ := normalizeAddressStrings(addresses, maximum)
	if b.LastSuccessUnixMS >= a.LastSuccessUnixMS {
		a.Source = b.Source
		a.LastSuccessUnixMS = b.LastSuccessUnixMS
		a.Failures = b.Failures
		a.NextAttemptUnixMS = b.NextAttemptUnixMS
		a.Successful = b.Successful
	}
	a.Addresses = normalized
	return a
}

func (c *PeerCache) SuccessfulCandidates(self libpeer.ID) []CachedPeer {
	c.mu.RLock()
	defer c.mu.RUnlock()
	now := c.now().UnixMilli()
	result := make([]CachedPeer, 0, len(c.entries))
	for peerID, record := range c.entries {
		if peerID == self || !record.Successful || record.NextAttemptUnixMS > now {
			continue
		}
		copyRecord := record
		copyRecord.Addresses = append([]string(nil), record.Addresses...)
		result = append(result, copyRecord)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].LastSuccessUnixMS != result[j].LastSuccessUnixMS {
			return result[i].LastSuccessUnixMS > result[j].LastSuccessUnixMS
		}
		return result[i].PeerID < result[j].PeerID
	})
	return result
}

func (c *PeerCache) markSuccess(peerID libpeer.ID, addresses []ma.Multiaddr, source PeerSource) error {
	if peerID == "" {
		return fmt.Errorf("empty PeerID")
	}
	values := sortedAddressStrings(addresses, c.limits.MaxAddressesPerPeer)
	if len(values) == 0 {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	record := CachedPeer{PeerID: peerID.String(), Addresses: values, Source: source, LastSuccessUnixMS: c.now().UnixMilli(), Successful: true}
	if old, ok := c.entries[peerID]; ok {
		record = mergeCachedPeers(old, record, c.limits.MaxAddressesPerPeer)
		record.Failures = 0
		record.NextAttemptUnixMS = 0
		record.Successful = true
	}
	c.entries[peerID] = record
	c.evictLocked()
	return c.saveLocked()
}

func (c *PeerCache) markFailure(peerID libpeer.ID, addresses []ma.Multiaddr, source PeerSource) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	record, ok := c.entries[peerID]
	if !ok {
		values := sortedAddressStrings(addresses, c.limits.MaxAddressesPerPeer)
		if len(values) == 0 {
			return nil
		}
		record = CachedPeer{PeerID: peerID.String(), Addresses: values, Source: source}
	}
	if record.Failures < 32 {
		record.Failures++
	}
	delay := c.failureDelay(record.Failures)
	record.NextAttemptUnixMS = c.now().Add(delay).UnixMilli()
	c.entries[peerID] = record
	c.evictLocked()
	return c.saveLocked()
}

func randomJitter(window time.Duration) time.Duration {
	if window <= 1 {
		return 0
	}
	return time.Duration(rand.Int64N(int64(window)))
}

// failureDelay returns exponential backoff plus up to 25 percent jitter while
// keeping the absolute delay within BackoffMaximum. Reserving the top fifth of
// the range preserves jitter after repeated failures reach the cap.
func (c *PeerCache) failureDelay(failures uint8) time.Duration {
	maximum := c.limits.BackoffMaximum
	baseCap := maximum - maximum/5
	if baseCap < c.limits.BackoffInitial {
		baseCap = c.limits.BackoffInitial
	}
	if baseCap > maximum {
		baseCap = maximum
	}
	base := c.limits.BackoffInitial
	for i := uint8(1); i < failures && base < baseCap; i++ {
		if base > baseCap/2 {
			base = baseCap
			break
		}
		base *= 2
	}
	if base > baseCap {
		base = baseCap
	}
	window := min(base/4, maximum-base)
	return base + c.jitter(window)
}

func (c *PeerCache) nextAttemptDelay(self libpeer.ID, connected map[libpeer.ID]struct{}) (time.Duration, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	now := c.now()
	var earliest time.Duration
	found := false
	for peerID, record := range c.entries {
		if peerID == self || record.NextAttemptUnixMS == 0 {
			continue
		}
		if _, ok := connected[peerID]; ok {
			continue
		}
		delay := time.UnixMilli(record.NextAttemptUnixMS).Sub(now)
		if delay <= 0 {
			return 0, true
		}
		if !found || delay < earliest {
			earliest, found = delay, true
		}
	}
	return earliest, found
}

func (c *PeerCache) hasCandidate(self libpeer.ID, connected map[libpeer.ID]struct{}) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for peerID, record := range c.entries {
		if peerID == self || len(record.Addresses) == 0 {
			continue
		}
		if _, ok := connected[peerID]; !ok {
			return true
		}
	}
	return false
}

func (c *PeerCache) backoffActive(peerID libpeer.ID, addresses []ma.Multiaddr) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	record, ok := c.entries[peerID]
	if !ok || record.NextAttemptUnixMS <= c.now().UnixMilli() {
		return false
	}
	if len(addresses) == 0 {
		return true
	}
	failed := make(map[string]struct{}, len(record.Addresses))
	for _, address := range record.Addresses {
		failed[address] = struct{}{}
	}
	for _, address := range addresses {
		if _, seen := failed[address.String()]; !seen {
			return false
		}
	}
	return true
}

func (c *PeerCache) evictLocked() {
	if len(c.entries) <= c.limits.MaxCachedPeers {
		return
	}
	all := make([]CachedPeer, 0, len(c.entries))
	for _, record := range c.entries {
		all = append(all, record)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Successful != all[j].Successful {
			return !all[i].Successful
		}
		return all[i].LastSuccessUnixMS < all[j].LastSuccessUnixMS
	})
	for len(c.entries) > c.limits.MaxCachedPeers {
		peerID, _ := libpeer.Decode(all[0].PeerID)
		delete(c.entries, peerID)
		all = all[1:]
	}
}

func (c *PeerCache) saveLocked() error {
	peers := make([]CachedPeer, 0, len(c.entries))
	for _, record := range c.entries {
		copyRecord := record
		copyRecord.Addresses = append([]string(nil), record.Addresses...)
		peers = append(peers, copyRecord)
	}
	sort.Slice(peers, func(i, j int) bool { return peers[i].PeerID < peers[j].PeerID })
	data, err := json.MarshalIndent(peerCacheFile{Version: peerCacheVersion, Peers: peers}, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > MaxPeerCacheFileSize {
		return fmt.Errorf("peer cache exceeds size bound")
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(c.path), ".peers-*")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Remove(c.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(name, c.path)
}

func (c *PeerCache) Flush() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.saveLocked()
}
