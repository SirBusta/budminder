// Package secretbox encrypts small secrets (such as a saved AC Infinity
// password) so they are stored scrambled. The key lives in its own file in
// BudMinder's data folder, never in the database or in backups: a copied
// backup or database only holds unreadable text. Someone with full access to
// the server's files could still find both; that is the honest limit of
// encrypting a secret a program must be able to use unattended.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// KeyFile is the key's file name inside the data folder.
const KeyFile = "budminder.key"

// ErrNoKey means the key file is missing (e.g. a backup restored on another
// computer): the secret can't be read and has to be entered again.
var ErrNoKey = errors.New("secretbox: no key file")

// LoadKey reads the 32-byte key from path.
func LoadKey(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoKey
	}
	if err != nil {
		return nil, err
	}
	key, err := hex.DecodeString(strings.TrimSpace(string(b)))
	if err != nil || len(key) != 32 {
		return nil, errors.New("secretbox: key file is not a 32-byte hex key")
	}
	return key, nil
}

// LoadOrCreateKey reads the key, making a new random one (readable only by
// its owner) the first time.
func LoadOrCreateKey(path string) ([]byte, error) {
	key, err := LoadKey(path)
	if !errors.Is(err, ErrNoKey) {
		return key, err
	}
	key = make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(hex.EncodeToString(key)+"\n"), 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

// Seal encrypts plaintext with AES-256-GCM; the result is base64 text
// (nonce followed by ciphertext).
func Seal(key []byte, plaintext string) (string, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(plaintext), nil)), nil
}

// Open decrypts what Seal produced. A wrong key or altered text fails.
func Open(key []byte, sealed string) (string, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	raw, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil || len(raw) < gcm.NonceSize() {
		return "", errors.New("secretbox: not sealed text")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", errors.New("secretbox: can't decrypt (wrong key?)")
	}
	return string(plain), nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
