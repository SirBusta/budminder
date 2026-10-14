package handlers

// Home Assistant: test the connection from Settings, and scan its sensors
// from Manage sensors. Polling lives in the watcher; the client in package
// homeassistant.

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"

	"isley/config"
	"isley/homeassistant"
	"isley/logger"
)

// maxHATokenLength bounds a pasted token (Home Assistant's long-lived
// tokens are about 180 characters).
const maxHATokenLength = 2048

// haClient builds a client from the typed address/token when given (Test
// connection before saving), otherwise from the saved settings.
func haClient(store *config.Store, url, token string) (*homeassistant.Client, error) {
	if strings.TrimSpace(url) == "" {
		url = store.HAURL()
	}
	if strings.TrimSpace(token) == "" {
		token = store.HAToken()
	}
	return homeassistant.New(url, token)
}

// haErrorKey picks the translated message (and status) for a client error.
func haErrorKey(err error) (int, string) {
	switch {
	case errors.Is(err, homeassistant.ErrBadURL):
		return http.StatusBadRequest, "ha_err_bad_url"
	case errors.Is(err, homeassistant.ErrNoToken):
		return http.StatusBadRequest, "ha_err_no_token"
	case errors.Is(err, homeassistant.ErrUnauthorized):
		return http.StatusBadGateway, "ha_err_unauthorized"
	case errors.Is(err, homeassistant.ErrUnreachable):
		return http.StatusBadGateway, "ha_err_unreachable"
	case errors.Is(err, homeassistant.ErrTooLarge):
		return http.StatusBadGateway, "ha_err_too_large"
	default:
		return http.StatusBadGateway, "ha_err_not_ha"
	}
}

// haStates fetches every state, answering the request with the right
// message on failure (ok=false).
func haStates(c *gin.Context, client *homeassistant.Client, check bool) ([]homeassistant.Entity, bool) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*homeassistant.Timeout)
	defer cancel()
	var states []homeassistant.Entity
	err := func() error {
		if check {
			if err := client.Check(ctx); err != nil {
				return err
			}
		}
		var err error
		states, err = client.States(ctx)
		return err
	}()
	if err != nil {
		logger.Log.WithField("func", "haStates").WithError(err).Warn("Home Assistant request failed")
		status, key := haErrorKey(err)
		apiError(c, status, key)
		return nil, false
	}
	return states, true
}

// TestHomeAssistantHandler checks the address and token
// (POST /settings/homeassistant/test, body {url, token}, both optional) and
// says how many sensors Home Assistant has.
func TestHomeAssistantHandler(c *gin.Context) {
	var input struct {
		URL   string `json:"url"`
		Token string `json:"token"`
	}
	_ = c.ShouldBindJSON(&input) // an empty body tests the saved settings
	if len(input.URL) > 2048 || len(input.Token) > maxHATokenLength {
		apiBadRequest(c, "api_invalid_input")
		return
	}
	client, err := haClient(ConfigStoreFromContext(c), input.URL, input.Token)
	if err != nil {
		status, key := haErrorKey(err)
		apiError(c, status, key)
		return
	}
	states, ok := haStates(c, client, true)
	if !ok {
		return
	}
	sensors, grow := 0, 0
	for _, e := range states {
		if homeassistant.IsSensor(e) {
			sensors++
			if homeassistant.IsGrowRelevant(e) {
				grow++
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "sensors": sensors, "grow": grow})
}

// ScanHomeAssistantSensors lists Home Assistant's numeric sensors that
// aren't in BudMinder yet (POST /sensors/scanHA). It only previews: each
// one is added from Manage sensors through /sensors/add.
func ScanHomeAssistantSensors(c *gin.Context) {
	client, err := haClient(ConfigStoreFromContext(c), "", "")
	if err != nil {
		status, key := haErrorKey(err)
		apiError(c, status, key)
		return
	}
	states, ok := haStates(c, client, false)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{"sensors": newSensorsOnly(DBFromContext(c), haFoundSensors(states))})
}

// haFoundSensors turns Home Assistant states into scan results: numeric
// sensor.* entities, grow-relevant ones first, then by name.
func haFoundSensors(states []homeassistant.Entity) []foundSensor {
	var found []foundSensor
	for _, e := range states {
		if !homeassistant.IsSensor(e) {
			continue
		}
		v, _ := e.Value()
		found = append(found, foundSensor{
			Source: homeassistant.Source, Device: homeassistant.Device, Type: e.EntityID,
			Name: e.Name(), Unit: e.Attributes.Unit, Value: &v, More: !homeassistant.IsGrowRelevant(e),
		})
	}
	sort.SliceStable(found, func(i, j int) bool {
		if found[i].More != found[j].More {
			return !found[i].More
		}
		return strings.ToLower(found[i].Name) < strings.ToLower(found[j].Name)
	})
	return found
}
