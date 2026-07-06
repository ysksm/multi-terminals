package remoteterm

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Key material file names, created inside the app's base directory (the same
// directory jsonstore uses).
const (
	// PrivateKeyFile holds this instance's auto-generated Ed25519 seed.
	PrivateKeyFile = "remote_key"
	// PublicKeyFile holds the matching public key for easy copying.
	PublicKeyFile = "remote_key.pub"
	// AuthorizedKeysFile lists public keys allowed to connect to this
	// instance's remote-terminal endpoint, one per line.
	AuthorizedKeysFile = "remote_authorized_keys"
)

// keyPrefix tags serialized keys with their algorithm.
const keyPrefix = "ed25519:"

// authContext domain-separates remote-auth signatures so the key cannot be
// abused to sign data for another protocol.
var authContext = []byte("multi-terminals remote-auth v1:")

// Identity is this instance's Ed25519 keypair used to authenticate outgoing
// remote-terminal connections.
type Identity struct {
	priv ed25519.PrivateKey
	pub  ed25519.PublicKey
}

// ErrIdentityExists is returned by IdentityStore.Generate when a key already
// exists (use Regenerate to replace it deliberately).
var ErrIdentityExists = errors.New("remote identity already exists")

// LoadOrCreateIdentity loads the instance keypair from dir, generating and
// persisting a new one when absent. Retained for tests and callers that want
// the legacy auto-create behaviour; the app itself creates keys on demand via
// IdentityStore.
func LoadOrCreateIdentity(dir string) (*Identity, error) {
	id, found, err := loadIdentity(dir)
	if err != nil {
		return nil, err
	}
	if found {
		return id, nil
	}
	return generateIdentity(dir)
}

// loadIdentity reads an existing instance keypair from dir. It returns
// (nil, false, nil) when no key file exists yet — an expected state now that
// key creation is a deliberate user action — and an error only when a present
// key file cannot be parsed.
func loadIdentity(dir string) (*Identity, bool, error) {
	privPath := filepath.Join(dir, PrivateKeyFile)
	data, err := os.ReadFile(privPath)
	switch {
	case err == nil:
		seed, err := decodeKey(string(data), ed25519.SeedSize)
		if err != nil {
			return nil, false, fmt.Errorf("remote identity: parse %s: %w", privPath, err)
		}
		priv := ed25519.NewKeyFromSeed(seed)
		return &Identity{priv: priv, pub: priv.Public().(ed25519.PublicKey)}, true, nil
	case os.IsNotExist(err):
		return nil, false, nil
	default:
		return nil, false, fmt.Errorf("remote identity: read %s: %w", privPath, err)
	}
}

// generateIdentity creates a fresh keypair and persists it to dir, overwriting
// any existing key files (private key with 0600 permissions).
func generateIdentity(dir string) (*Identity, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("remote identity: generate key: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("remote identity: create dir: %w", err)
	}
	seedLine := keyPrefix + base64.StdEncoding.EncodeToString(priv.Seed()) + "\n"
	if err := os.WriteFile(filepath.Join(dir, PrivateKeyFile), []byte(seedLine), 0o600); err != nil {
		return nil, fmt.Errorf("remote identity: write private key: %w", err)
	}
	id := &Identity{priv: priv, pub: pub}
	// The .pub file is a convenience copy; failing to write it must not lose
	// the already-persisted private key.
	pubLine := id.PublicKeyString() + "\n"
	if err := os.WriteFile(filepath.Join(dir, PublicKeyFile), []byte(pubLine), 0o644); err != nil {
		return nil, fmt.Errorf("remote identity: write public key: %w", err)
	}
	return id, nil
}

// deleteIdentity removes the persisted keypair from dir. A missing file is not
// an error.
func deleteIdentity(dir string) error {
	for _, name := range []string{PrivateKeyFile, PublicKeyFile} {
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remote identity: remove %s: %w", name, err)
		}
	}
	return nil
}

// IdentityStore manages this instance's signing keypair as an explicit,
// user-controlled resource: the key is created on demand (not auto-generated on
// startup), can be regenerated, and can be deleted. The current key is cached
// in memory and kept in sync with disk. Safe for concurrent use.
type IdentityStore struct {
	mu  sync.Mutex
	dir string
	id  *Identity // nil when no key exists yet
}

// NewIdentityStore returns a store backed by dir, loading an existing key if
// one is present. A missing key is not an error — it means no key has been
// created yet.
func NewIdentityStore(dir string) (*IdentityStore, error) {
	id, _, err := loadIdentity(dir)
	if err != nil {
		return nil, err
	}
	return &IdentityStore{dir: dir, id: id}, nil
}

// Current returns the current identity and whether one exists. The signature
// matches the provider Runner expects, so store.Current can be passed directly.
func (s *IdentityStore) Current() (*Identity, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.id, s.id != nil
}

// Generate creates and persists a new key. It returns ErrIdentityExists if a
// key already exists, so a create action never silently overwrites one.
func (s *IdentityStore) Generate() (*Identity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.id != nil {
		return nil, ErrIdentityExists
	}
	id, err := generateIdentity(s.dir)
	if err != nil {
		return nil, err
	}
	s.id = id
	return id, nil
}

// Regenerate replaces the key with a freshly generated one (creating it if
// absent). Note: the public key changes, so other machines that authorized the
// old key must re-authorize the new one.
func (s *IdentityStore) Regenerate() (*Identity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, err := generateIdentity(s.dir)
	if err != nil {
		return nil, err
	}
	s.id = id
	return id, nil
}

// Delete removes the key so the instance has none until one is created again.
func (s *IdentityStore) Delete() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := deleteIdentity(s.dir); err != nil {
		return err
	}
	s.id = nil
	return nil
}

// PublicKeyString returns the shareable serialized public key
// ("ed25519:<base64>") to paste into another instance's authorized keys.
func (id *Identity) PublicKeyString() string {
	return keyPrefix + base64.StdEncoding.EncodeToString(id.pub)
}

// Fingerprint returns a short SHA-256 digest of the public key for display,
// in the same style as OpenSSH ("SHA256:<base64-no-padding>").
func (id *Identity) Fingerprint() string {
	return fingerprint(id.pub)
}

// sign produces the challenge-response signature over the server nonce.
func (id *Identity) sign(nonce []byte) []byte {
	return ed25519.Sign(id.priv, append(append([]byte{}, authContext...), nonce...))
}

// verifyAuth checks a client signature over nonce with the given public key.
func verifyAuth(pub ed25519.PublicKey, nonce, sig []byte) bool {
	return ed25519.Verify(pub, append(append([]byte{}, authContext...), nonce...), sig)
}

func fingerprint(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
}

// decodeKey parses "ed25519:<base64>" (surrounding whitespace and a trailing
// comment are ignored) and enforces the expected byte length.
func decodeKey(s string, wantLen int) ([]byte, error) {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, " \t"); i >= 0 {
		s = s[:i]
	}
	if !strings.HasPrefix(s, keyPrefix) {
		return nil, fmt.Errorf("key must start with %q", keyPrefix)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(s, keyPrefix))
	if err != nil {
		return nil, fmt.Errorf("invalid base64: %w", err)
	}
	if len(raw) != wantLen {
		return nil, fmt.Errorf("key must be %d bytes, got %d", wantLen, len(raw))
	}
	return raw, nil
}

// ParsePublicKey parses a serialized public key ("ed25519:<base64>").
func ParsePublicKey(s string) (ed25519.PublicKey, error) {
	raw, err := decodeKey(s, ed25519.PublicKeySize)
	if err != nil {
		return nil, err
	}
	return ed25519.PublicKey(raw), nil
}

// AuthorizedKey is one allowed public key with its optional comment.
type AuthorizedKey struct {
	Key         string `json:"key"`
	Comment     string `json:"comment"`
	Fingerprint string `json:"fingerprint"`
}

// AuthorizedKeys manages the file listing public keys allowed to open remote
// terminals on this instance. The file is re-read on every operation so
// out-of-band edits take effect without a restart. Safe for concurrent use.
type AuthorizedKeys struct {
	mu   sync.Mutex
	path string
}

// NewAuthorizedKeys returns a store backed by the given file path. A missing
// file is treated as an empty list (remote access disabled).
func NewAuthorizedKeys(path string) *AuthorizedKeys {
	return &AuthorizedKeys{path: path}
}

// load reads and parses the file, skipping blank lines, comment lines and
// entries that fail to parse (a malformed line must not grant access).
func (a *AuthorizedKeys) load() []AuthorizedKey {
	data, err := os.ReadFile(a.path)
	if err != nil {
		return nil
	}
	var keys []AuthorizedKey
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.SplitN(line, " ", 2)
		pub, err := ParsePublicKey(fields[0])
		if err != nil {
			continue
		}
		comment := ""
		if len(fields) == 2 {
			comment = strings.TrimSpace(fields[1])
		}
		keys = append(keys, AuthorizedKey{
			Key:         fields[0],
			Comment:     comment,
			Fingerprint: fingerprint(pub),
		})
	}
	return keys
}

// save writes the full key list back to the file with private permissions.
func (a *AuthorizedKeys) save(keys []AuthorizedKey) error {
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k.Key)
		if k.Comment != "" {
			b.WriteString(" " + k.Comment)
		}
		b.WriteString("\n")
	}
	if err := os.MkdirAll(filepath.Dir(a.path), 0o700); err != nil {
		return fmt.Errorf("authorized keys: create dir: %w", err)
	}
	if err := os.WriteFile(a.path, []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("authorized keys: write: %w", err)
	}
	return nil
}

// List returns all valid authorized keys.
func (a *AuthorizedKeys) List() []AuthorizedKey {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.load()
}

// Add validates and appends a public key. Adding an already-present key only
// updates its comment.
func (a *AuthorizedKeys) Add(key, comment string) error {
	pub, err := ParsePublicKey(key)
	if err != nil {
		return fmt.Errorf("authorized keys: %w", err)
	}
	norm := keyPrefix + base64.StdEncoding.EncodeToString(pub)

	a.mu.Lock()
	defer a.mu.Unlock()
	keys := a.load()
	for i, k := range keys {
		if k.Key == norm {
			keys[i].Comment = strings.TrimSpace(comment)
			return a.save(keys)
		}
	}
	keys = append(keys, AuthorizedKey{Key: norm, Comment: strings.TrimSpace(comment)})
	return a.save(keys)
}

// Remove deletes a public key. Removing an absent key is a no-op.
func (a *AuthorizedKeys) Remove(key string) error {
	pub, err := ParsePublicKey(key)
	if err != nil {
		return fmt.Errorf("authorized keys: %w", err)
	}
	norm := keyPrefix + base64.StdEncoding.EncodeToString(pub)

	a.mu.Lock()
	defer a.mu.Unlock()
	keys := a.load()
	kept := keys[:0]
	for _, k := range keys {
		if k.Key != norm {
			kept = append(kept, k)
		}
	}
	return a.save(kept)
}

// IsAuthorized reports whether pub is in the authorized list.
func (a *AuthorizedKeys) IsAuthorized(pub ed25519.PublicKey) bool {
	want := keyPrefix + base64.StdEncoding.EncodeToString(pub)
	for _, k := range a.List() {
		if k.Key == want {
			return true
		}
	}
	return false
}

// Enabled reports whether remote access is enabled — i.e. at least one key is
// authorized. With an empty list the endpoint rejects everything.
func (a *AuthorizedKeys) Enabled() bool {
	return len(a.List()) > 0
}
