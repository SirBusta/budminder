package utils

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHideURLSecrets_RealClientError(t *testing.T) {
	t.Parallel()

	client := &http.Client{Timeout: time.Second}
	_, err := client.Post("http://user:camerapass@127.0.0.1:1/api/user/devInfoListAll?userId=SECRET-TOKEN", "text/plain", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "SECRET-TOKEN", "Go's own error carries the query string")

	got := HideURLSecrets(err).Error()
	assert.NotContains(t, got, "SECRET-TOKEN")
	assert.NotContains(t, got, "camerapass")
	assert.True(t, strings.Contains(got, "127.0.0.1:1/api/user/devInfoListAll"), got)
}

func TestHideURLSecrets_OtherErrorsUnchanged(t *testing.T) {
	t.Parallel()

	plain := errors.New("plain")
	assert.Same(t, plain, HideURLSecrets(plain))
	assert.Nil(t, HideURLSecrets(nil))
}
