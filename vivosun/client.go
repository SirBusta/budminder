// Package vivosun reads sensors from Vivosun's cloud: sign-in, the device
// list and each device's one-minute readings. Read-only: it never sends a
// control request.
//
// The request format comes from lientry/homeassistant-vivosun-growhub
// (MIT License, Copyright (c) 2026 lientry): api.py, encryption.py,
// const.py and redaction.py.
package vivosun

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// BaseURL is Vivosun's API host; tests point Client.BaseURL elsewhere.
const BaseURL = "https://api-prod.next.vivosun.com"

const (
	loginPath   = "/user/login"
	devicesPath = "/iot/device/getTotalList"
	readingPath = "/iot/data/getPointLog"

	// What the Android app sends; Vivosun refuses requests from an
	// "outdated app".
	appID       = "com.vivosun.android"
	apiVersion  = "1.0.5"
	appVersion  = "4.63.1"
	maxResponse = 8 << 20
)

// ErrUnreachable means Vivosun couldn't be reached or answered oddly.
var ErrUnreachable = errors.New("vivosun: could not reach Vivosun")

// ErrAuth means Vivosun refused the saved sign-in: sign in again.
var ErrAuth = errors.New("vivosun: sign-in refused")

// RefusedError is Vivosun saying no, with its own message. It wraps ErrAuth
// when the message is about the sign-in.
type RefusedError struct {
	Code any
	Msg  string
	auth bool
}

func (e *RefusedError) Error() string {
	return strings.TrimSpace(fmt.Sprintf("Vivosun refused the request: %v %s", e.Code, e.Msg))
}

func (e *RefusedError) Unwrap() error {
	if e.auth {
		return ErrAuth
	}
	return nil
}

// Tokens are what a sign-in returns and every other request sends.
type Tokens struct {
	Access, Login string
}

// Set is true when both tokens are there.
func (t Tokens) Set() bool { return t.Access != "" && t.Login != "" }

// Doer is the part of *http.Client the client needs.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Client talks to Vivosun. The zero value uses Vivosun's own host and a
// default HTTP client.
type Client struct {
	HTTP    Doer
	BaseURL string
}

var defaultHTTP Doer = &http.Client{Timeout: 20 * time.Second}

// Device is one device on the account.
type Device struct {
	ID       string
	Name     string
	SceneID  int64
	HasScene bool
}

// Row is one reading row: Vivosun's names with the values divided by 100
// (the API sends 2256 for 22.56), and At, when it was measured. A sensor
// that isn't connected (-6666) is left out.
type Row struct {
	At     time.Time
	Values map[string]float64
}

// Login signs in and returns the tokens.
func (c *Client) Login(ctx context.Context, email, password string) (Tokens, error) {
	body := map[string]any{
		"email": email, "password": password, "spAppId": appID,
		"spClientId": newUUID(), "spSessionId": newUUID(),
	}
	env, err := c.call(ctx, http.MethodPost, loginPath, Tokens{}, body)
	if err != nil {
		var refused *RefusedError
		if errors.As(err, &refused) {
			refused.auth = true // any refusal of a sign-in is about the sign-in
		}
		return Tokens{}, err
	}
	data, _ := env["data"].(map[string]any)
	tk := Tokens{Access: str(data["accessToken"]), Login: str(data["loginToken"])}
	if !tk.Set() {
		return Tokens{}, fmt.Errorf("%w: sign-in answered without tokens", ErrUnreachable)
	}
	return tk, nil
}

// Devices lists the account's devices, in a stable order.
func (c *Client) Devices(ctx context.Context, tk Tokens) ([]Device, error) {
	env, err := c.call(ctx, http.MethodGet, devicesPath, tk, nil)
	if err != nil {
		return nil, err
	}
	return parseDevices(env), nil
}

// DevicesRaw is the device list exactly as Vivosun sends it (for the
// troubleshooting download, after Hide).
func (c *Client) DevicesRaw(ctx context.Context, tk Tokens) (map[string]any, error) {
	return c.call(ctx, http.MethodGet, devicesPath, tk, nil)
}

func parseDevices(env map[string]any) []Device {
	data, _ := env["data"].(map[string]any)
	group, _ := data["deviceGroup"].(map[string]any)
	var out []Device
	for _, cat := range sortedKeys(group) {
		list, _ := group[cat].([]any)
		for _, item := range list {
			m, _ := item.(map[string]any)
			if m == nil || str(m["deviceId"]) == "" {
				continue
			}
			d := Device{ID: str(m["deviceId"]), Name: strings.TrimSpace(str(m["name"]))}
			if d.Name == "" {
				d.Name = "Vivosun"
			}
			if sc, ok := m["scene"].(map[string]any); ok {
				if f, ok := sc["sceneId"].(float64); ok {
					d.SceneID, d.HasScene = int64(f), true
				}
			}
			out = append(out, d)
		}
	}
	return out
}

// Readings returns a device's rows between from and to, oldest first.
func (c *Client) Readings(ctx context.Context, tk Tokens, d Device, from, to time.Time) ([]Row, error) {
	raw, err := c.ReadingsRaw(ctx, tk, d, from, to)
	if err != nil {
		return nil, err
	}
	rows := make([]Row, 0, len(raw))
	for _, item := range raw {
		m, _ := item.(map[string]any)
		if r, ok := parseRow(m); ok {
			rows = append(rows, r)
		}
	}
	sort.SliceStable(rows, func(a, b int) bool { return rows[a].At.Before(rows[b].At) })
	return rows, nil
}

// ReadingsRaw is the rows exactly as Vivosun sends them.
func (c *Client) ReadingsRaw(ctx context.Context, tk Tokens, d Device, from, to time.Time) ([]any, error) {
	body := map[string]any{
		"sceneId": d.SceneID, "deviceId": d.ID, "startTime": from.Unix(), "endTime": to.Unix(),
		"reportType": 0, "orderBy": "asc", "timeLevel": "ONE_MINUTE",
	}
	env, err := c.call(ctx, http.MethodPost, readingPath, tk, body)
	if err != nil && !d.HasScene && !errors.Is(err, ErrAuth) && !errors.Is(err, ErrUnreachable) {
		// Devices without a scene (the VSeed) are asked without one.
		delete(body, "sceneId")
		env, err = c.call(ctx, http.MethodPost, readingPath, tk, body)
	}
	if err != nil {
		return nil, err
	}
	data, _ := env["data"].(map[string]any)
	rows, _ := data["iotDataLogList"].([]any)
	return rows, nil
}

func parseRow(m map[string]any) (Row, bool) {
	ts, ok := m["time"].(float64)
	if !ok || ts <= 0 {
		return Row{}, false
	}
	r := Row{At: time.Unix(int64(ts), 0), Values: map[string]float64{}}
	for k, v := range m {
		f, ok := v.(float64)
		if !ok || k == "time" || f == noSensor {
			continue
		}
		r.Values[k] = f / 100
	}
	return r, true
}

// noSensor is what Vivosun sends for a sensor that isn't connected.
const noSensor = -6666

// authMarks: Vivosun answers a refused token with success false and a
// message; one with these words is about the sign-in (as api.py decides).
var authMarks = []string{"auth", "credential", "forbidden", "invalid", "login", "password", "token", "unauthorized"}

// call sends one request and returns the response when success is true.
// Every POST but the sign-in is encrypted.
func (c *Client) call(ctx context.Context, method, path string, tk Tokens, body map[string]any) (map[string]any, error) {
	base := c.BaseURL
	if base == "" {
		base = BaseURL
	}
	client := c.HTTP
	if client == nil {
		client = defaultHTTP
	}
	var payload []byte
	hdr := http.Header{}
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return nil, err
		}
		if method == http.MethodPost && path != loginPath {
			t, code, enc := encrypt(payload, time.Now().UnixMilli())
			hdr.Set("Request-Time", t)
			hdr.Set("Request-Code", code)
			payload = enc
		}
		hdr.Set("Content-Type", "application/json")
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header = hdr
	req.Header.Set("Server-Platform", "android")
	req.Header.Set("Api-Version", apiVersion)
	req.Header.Set("App-Version", appVersion)
	if tk.Set() {
		req.Header.Set("login-token", tk.Login)
		req.Header.Set("access-token", tk.Access)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, &RefusedError{Code: resp.StatusCode, Msg: http.StatusText(resp.StatusCode), auth: true}
	}
	var env map[string]any
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("%w (HTTP %d, not JSON)", ErrUnreachable, resp.StatusCode)
	}
	if ok, _ := env["success"].(bool); !ok {
		msg := str(env["message"])
		lower := strings.ToLower(msg)
		auth := false
		for _, m := range authMarks {
			if strings.Contains(lower, m) {
				auth = true
				break
			}
		}
		return nil, &RefusedError{Code: env["code"], Msg: msg, auth: auth}
	}
	return env, nil
}

// encrypt mirrors encryption.py: AES-CBC with PKCS7 padding; the key is a
// random-length slice of md5_hex(time in ms) and the IV a 16-character slice
// of a random salt, both described in the Request-Code header.
func encrypt(plain []byte, ms int64) (string, string, []byte) {
	ts := strconv.FormatInt(ms, 10)
	sum := md5.Sum([]byte(ts))
	md5hex := hex.EncodeToString(sum[:])
	keyLen := []int{16, 24, 32}[rnd(3)]
	ks := rnd(32 - keyLen + 1)
	key := []byte(md5hex[ks : ks+keyLen])
	const alpha = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	saltLen := 16 + rnd(84)
	salt := make([]byte, saltLen)
	for i := range salt {
		salt[i] = alpha[rnd(len(alpha))]
	}
	is := rnd(saltLen - 16 + 1)
	iv := salt[is : is+16]
	pad := aes.BlockSize - len(plain)%aes.BlockSize
	padded := append(append([]byte{}, plain...), bytes.Repeat([]byte{byte(pad)}, pad)...)
	blk, _ := aes.NewCipher(key) // 16, 24 or 32 bytes: never fails
	ct := make([]byte, len(padded))
	cipher.NewCBCEncrypter(blk, iv).CryptBlocks(ct, padded)
	code := fmt.Sprintf("AC5-%d-%d-%d-%d-%s", ks, ks+keyLen, is, is+16, salt)
	body, _ := json.Marshal(map[string]string{"content": hex.EncodeToString(ct)})
	return ts, code, body
}

func rnd(n int) int {
	x, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0
	}
	return int(x.Int64())
}

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func sortedKeys(m map[string]any) []string {
	k := make([]string, 0, len(m))
	for x := range m {
		k = append(k, x)
	}
	sort.Strings(k)
	return k
}

func str(v any) string { s, _ := v.(string); return s }
