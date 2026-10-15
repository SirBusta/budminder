// Package acinfinity signs in to AC Infinity's cloud API and returns the
// token BudMinder polls with. Shared by the Settings sign-in window and the
// watcher's automatic sign-in ("Stay signed in").
package acinfinity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// BaseURL is AC Infinity's API host; tests point it elsewhere per call.
const BaseURL = "https://www.acinfinityserver.com"

// MaxPasswordLength: AC Infinity's sign-in only uses the first 25 characters.
const MaxPasswordLength = 25

const successCode = 200

// ErrUnreachable means AC Infinity couldn't be reached or answered oddly.
var ErrUnreachable = errors.New("acinfinity: could not reach AC Infinity")

// RefusedError is AC Infinity saying no (wrong email or password, ...),
// with its own message.
type RefusedError struct {
	Code int
	Msg  string
}

func (e *RefusedError) Error() string {
	return strings.TrimSpace(fmt.Sprintf("AC Infinity refused the sign-in: %d %s", e.Code, e.Msg))
}

// Doer is the part of *http.Client Login needs.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

var defaultHTTP Doer = &http.Client{Timeout: 15 * time.Second}

// Login signs in and returns the token. baseURL "" means AC Infinity's own
// host; client nil means a default client.
func Login(ctx context.Context, client Doer, baseURL, email, password string) (string, error) {
	if client == nil {
		client = defaultHTTP
	}
	if baseURL == "" {
		baseURL = BaseURL
	}
	if len(password) > MaxPasswordLength {
		password = password[:MaxPasswordLength]
	}
	form := url.Values{}
	form.Set("appEmail", email)
	form.Set("appPasswordl", password) // sic: AC Infinity's own field name
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/user/appUserLogin", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")
	req.Header.Set("User-Agent", "ACController/1.8.2 (com.acinfinity.humiture; build:489; iOS 16.5.1) Alamofire/5.4.4")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w (HTTP %d)", ErrUnreachable, resp.StatusCode)
	}
	var body struct {
		Msg  string `json:"msg"`
		Code int    `json:"code"`
		Data struct {
			AppID string `json:"appId"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return "", fmt.Errorf("%w: unreadable reply", ErrUnreachable)
	}
	if body.Code != successCode || body.Data.AppID == "" {
		return "", &RefusedError{Code: body.Code, Msg: body.Msg}
	}
	return body.Data.AppID, nil
}
