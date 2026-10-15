package secretbox_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/secretbox"
)

func TestSealOpenRoundTrip(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "data", secretbox.KeyFile)
	key, err := secretbox.LoadOrCreateKey(path)
	require.NoError(t, err)

	sealed, err := secretbox.Seal(key, "my AC Infinity password")
	require.NoError(t, err)
	assert.NotContains(t, sealed, "password", "stored scrambled")

	again, err := secretbox.LoadOrCreateKey(path)
	require.NoError(t, err)
	assert.Equal(t, key, again, "the key is kept, not remade")
	plain, err := secretbox.Open(again, sealed)
	require.NoError(t, err)
	assert.Equal(t, "my AC Infinity password", plain)

	other, err := secretbox.Seal(key, "my AC Infinity password")
	require.NoError(t, err)
	assert.NotEqual(t, sealed, other, "a fresh nonce each time")

	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "only the owner can read the key")
	}
}

func TestOpen_WrongKeyOrNoKey(t *testing.T) {
	t.Parallel()
	k1, err := secretbox.LoadOrCreateKey(filepath.Join(t.TempDir(), secretbox.KeyFile))
	require.NoError(t, err)
	k2, err := secretbox.LoadOrCreateKey(filepath.Join(t.TempDir(), secretbox.KeyFile))
	require.NoError(t, err)
	sealed, err := secretbox.Seal(k1, "secret")
	require.NoError(t, err)

	_, err = secretbox.Open(k2, sealed)
	assert.Error(t, err, "another install's key can't read it")
	_, err = secretbox.Open(k1, strings.ToUpper(sealed))
	assert.Error(t, err, "altered text fails")

	_, err = secretbox.LoadKey(filepath.Join(t.TempDir(), "missing.key"))
	assert.ErrorIs(t, err, secretbox.ErrNoKey)
}
