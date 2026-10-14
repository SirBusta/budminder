// Package homeassistant reads sensor states from a Home Assistant server
// through its REST API (GET /api/ and GET /api/states, authenticated with a
// long-lived access token). It is read-only: BudMinder never changes
// anything in Home Assistant.
//
// The package is a leaf (standard library only) so both the Manage sensors
// handlers (test connection, scan) and the watcher (polling) can use it.
package homeassistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// UserAgent identifies BudMinder to Home Assistant. main sets the full
// "BudMinder/<version> (+…)" string at startup.
var UserAgent = "BudMinder"

const (
	// Timeout bounds one request; Home Assistant answers /api/states in
	// well under a second even with hundreds of entities.
	Timeout = 10 * time.Second
	// MaxResponseBytes caps a response body. /api/states carries every
	// entity with its attributes, so it is larger than the other sensor
	// APIs; 8 MB is far beyond a big real installation.
	MaxResponseBytes = 8 * 1024 * 1024
)

// Errors a caller can tell apart to explain what went wrong.
var (
	ErrBadURL           = errors.New("home assistant: the address must be an http:// or https:// URL")
	ErrNoToken          = errors.New("home assistant: no access token")
	ErrUnauthorized     = errors.New("home assistant: the access token was refused")
	ErrUnreachable      = errors.New("home assistant: could not connect")
	ErrNotHomeAssistant = errors.New("home assistant: the address did not answer like Home Assistant")
	ErrTooLarge         = errors.New("home assistant: the response was too large")
)

// Doer is the part of *http.Client the client needs; tests can pass any
// implementation.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// DefaultHTTP is a client that never follows redirects: a redirect would
// send the access token's request somewhere the user didn't enter.
var DefaultHTTP Doer = &http.Client{
	Timeout: Timeout,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// Entity is one Home Assistant state as /api/states returns it (only the
// fields BudMinder uses).
type Entity struct {
	EntityID   string `json:"entity_id"`
	State      string `json:"state"`
	Attributes struct {
		Unit         string `json:"unit_of_measurement"`
		DeviceClass  string `json:"device_class"`
		FriendlyName string `json:"friendly_name"`
	} `json:"attributes"`
}

// Name is the entity's friendly name, or its entity ID when it has none.
func (e Entity) Name() string {
	if n := strings.TrimSpace(e.Attributes.FriendlyName); n != "" {
		return n
	}
	return e.EntityID
}

// Value parses the state as a number. Home Assistant reports
// "unavailable" or "unknown" when a sensor isn't reporting; those, and any
// other non-number, return ok=false.
func (e Entity) Value() (float64, bool) {
	v, err := strconv.ParseFloat(strings.TrimSpace(e.State), 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// NormalizeURL checks a Home Assistant address and returns it without a
// trailing slash. It must be http or https with a host; a path is kept
// (Home Assistant behind a reverse proxy at /ha), a query or fragment is
// not allowed.
func NormalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
		u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return "", ErrBadURL
	}
	return strings.TrimRight(raw, "/"), nil
}

// Client talks to one Home Assistant server.
type Client struct {
	BaseURL string
	Token   string
	HTTP    Doer
}

// New returns a client for baseURL (checked with NormalizeURL) and token.
func New(baseURL, token string) (*Client, error) {
	u, err := NormalizeURL(baseURL)
	if err != nil {
		return nil, err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrNoToken
	}
	return &Client{BaseURL: u, Token: token, HTTP: DefaultHTTP}, nil
}

// Check confirms the address is Home Assistant and the token is accepted
// (GET /api/ answers {"message": "API running."}).
func (c *Client) Check(ctx context.Context) error {
	body, err := c.get(ctx, "/api/")
	if err != nil {
		return err
	}
	var r struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &r) != nil || r.Message == "" {
		return ErrNotHomeAssistant
	}
	return nil
}

// States returns every entity's current state (GET /api/states).
func (c *Client) States(ctx context.Context) ([]Entity, error) {
	body, err := c.get(ctx, "/api/states")
	if err != nil {
		return nil, err
	}
	var out []Entity
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, ErrNotHomeAssistant
	}
	return out, nil
}

func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return nil, ErrBadURL
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", UserAgent)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		// Deliberately not wrapping err: it can echo the URL, never the
		// token, but the caller only needs to know the server wasn't reached.
		return nil, ErrUnreachable
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, ErrUnauthorized
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("%w (HTTP %d)", ErrNotHomeAssistant, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes+1))
	if err != nil {
		return nil, ErrUnreachable
	}
	if len(body) > MaxResponseBytes {
		return nil, ErrTooLarge
	}
	return body, nil
}
