package handlers_test

// Settings > Phone & tablet (handlers/phone.go) through the real engine.

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPhoneHTTP_SavedAddressAndQR(t *testing.T) {
	t.Parallel()
	r := newAlertRoutes(t)
	page := r.settingsPage(t)
	assert.Contains(t, page, `data-sec="phone"`)
	assert.Contains(t, page, `href="/settings#phone"`)
	assert.Contains(t, page, `data-from-saved="The address you saved.`, "page text, not escaped as a URL")

	// The same setting as Alerts' BudMinder address.
	code, out := r.post(t, "/settings/alerts/link", map[string]any{"link": "http://192.168.1.10:8080/"})
	require.Equal(t, http.StatusOK, code, out)

	resp := r.c.Get("/settings/phone")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var d map[string]any
	require.NoError(t, json.Unmarshal(b, &d))
	assert.Equal(t, "http://192.168.1.10:8080", d["address"])
	assert.Equal(t, "saved", d["source"])
	assert.Contains(t, d["qr"], "<svg ")

	page = r.settingsPage(t)
	assert.Contains(t, page, `<code id="phAddrText">http://192.168.1.10:8080</code>`)
	assert.Contains(t, page, `<svg xmlns="http://www.w3.org/2000/svg"`, "the code is drawn on the page")
}
