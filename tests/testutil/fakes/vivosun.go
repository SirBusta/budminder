package fakes

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

// The account FakeVivosun accepts, and the tokens it hands out.
const (
	VivosunEmail    = "grower@example.com"
	VivosunPassword = "test-vivosun-pass"
	VivosunAccess   = "test-vs-access"
	VivosunLogin    = "test-vs-login"
	// VivosunDeviceID is the VCure C80 on the fake account.
	VivosunDeviceID = "1730901"
)

// FakeVS is a stand-in for Vivosun's API: sign-in (/user/login), the device
// list and the reading rows. Every call but the sign-in needs the tokens it
// hands out (else HTTP 401). The readings request is encrypted by the real
// client; the fake doesn't read it and answers with Rows.
type FakeVS struct {
	*httptest.Server

	mu     sync.Mutex
	rows   []map[string]any
	expire bool // refuse the tokens with "Token expired" until the next sign-in

	// Logins counts sign-in attempts; DeviceCalls and ReadingCalls the other requests.
	Logins, DeviceCalls, ReadingCalls atomic.Int64
}

// SetRows sets the reading rows (Vivosun's names, values x100, "time" in
// Unix seconds) every readings request answers with.
func (f *FakeVS) SetRows(rows ...map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows = rows
}

// Expire makes Vivosun refuse the current tokens until someone signs in again.
func (f *FakeVS) Expire() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.expire = true
}

// VCureRow is one VCure C80 row at unix time ts: inside 22.5 °C / 57%,
// room 21 °C / 60%, electronics 38.9 °C, coil 20 °C, plus readings BudMinder
// doesn't offer (Wi-Fi signal, compressor) and an unplugged probe (-6666).
func VCureRow(ts int64) map[string]any {
	return map[string]any{"time": ts, "pTemp": 2250, "pHumi": 5700, "pVpd": 115, "bTemp": 2100, "bHumi": 6000,
		"bVpd": 100, "coreTemp": 3890, "evapT": 2000, "rssi": -7600, "cure.comp": 0, "waterT": -6666}
}

// FakeVivosun starts a FakeVS with one VCure C80 and no rows.
func FakeVivosun(t *testing.T) *FakeVS {
	t.Helper()
	f := &FakeVS{}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.Server.Close)
	return f
}

func (f *FakeVS) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == "/user/login" {
		f.Logins.Add(1)
		var in map[string]any
		_ = json.Unmarshal(body, &in)
		if in["email"] != VivosunEmail || in["password"] != VivosunPassword {
			fmt.Fprint(w, `{"success":false,"code":20001,"message":"Wrong account or password"}`)
			return
		}
		f.mu.Lock()
		f.expire = false
		f.mu.Unlock()
		fmt.Fprintf(w, `{"success":true,"data":{"accessToken":%q,"loginToken":%q,"userId":"u-1","email":%q}}`,
			VivosunAccess, VivosunLogin, VivosunEmail)
		return
	}
	if r.Header.Get("access-token") != VivosunAccess || r.Header.Get("login-token") != VivosunLogin {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	f.mu.Lock()
	expired, rows := f.expire, f.rows
	f.mu.Unlock()
	if expired {
		fmt.Fprint(w, `{"success":false,"code":10001,"message":"Token expired, please login again"}`)
		return
	}
	switch r.URL.Path {
	case "/iot/device/getTotalList":
		f.DeviceCalls.Add(1)
		fmt.Fprintf(w, `{"success":true,"data":{"deviceGroup":{"STORE":[{"deviceId":%q,"name":"VCure C80","userId":"u-1","email":%q,"scene":null}]}}}`,
			VivosunDeviceID, VivosunEmail)
	case "/iot/data/getPointLog":
		f.ReadingCalls.Add(1)
		if rows == nil {
			rows = []map[string]any{}
		}
		b, _ := json.Marshal(map[string]any{"success": true, "data": map[string]any{"iotDataLogList": rows}})
		_, _ = w.Write(b)
	default:
		http.NotFound(w, r)
	}
}
