package p2p

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"

	libcrypto "github.com/libp2p/go-libp2p/core/crypto"
)

// LoadOrCreatePeerKey persists a libp2p Ed25519 identity independently of all
// ZION member and CometBFT consensus keys.
func LoadOrCreatePeerKey(path string) (libcrypto.PrivKey, error) {
	data, err := readBoundedFile(path, MaxPeerKeyFileSize)
	if err == nil {
		if len(data) == 0 {
			return nil, fmt.Errorf("invalid P2P key file size %d", len(data))
		}
		key, err := libcrypto.UnmarshalPrivateKey(data)
		if err != nil {
			return nil, fmt.Errorf("decode P2P key: %w", err)
		}
		if key.Type() != libcrypto.Ed25519 {
			return nil, fmt.Errorf("P2P key must be Ed25519")
		}
		return key, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read P2P key: %w", err)
	}
	key, _, err := libcrypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate P2P key: %w", err)
	}
	encoded, err := libcrypto.MarshalPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("encode P2P key: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create P2P key directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".peer-key-*")
	if err != nil {
		return nil, fmt.Errorf("create temporary P2P key: %w", err)
	}
	tempName := temporary.Name()
	defer os.Remove(tempName)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return nil, err
	}
	if _, err := temporary.Write(encoded); err != nil {
		_ = temporary.Close()
		return nil, err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return nil, err
	}
	if err := temporary.Close(); err != nil {
		return nil, err
	}
	if err := os.Rename(tempName, path); err != nil {
		// Another concurrent creator may have won; never overwrite its key.
		if existing, readErr := readBoundedFile(path, MaxPeerKeyFileSize); readErr == nil {
			existingKey, decodeErr := libcrypto.UnmarshalPrivateKey(existing)
			if decodeErr == nil && existingKey.Type() == libcrypto.Ed25519 {
				return existingKey, nil
			}
		}
		return nil, fmt.Errorf("install P2P key: %w", err)
	}
	return key, nil
}
