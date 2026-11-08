package watcher

import (
	"context"
	"errors"
	"time"

	"isley/vivosun"
)

const (
	// vsWindow is how far back each poll asks for readings.
	vsWindow = 10 * time.Minute
	// vsFreshFor: a newest row older than this isn't saved, so a device that
	// went offline shows "No new readings" (Vivosun doesn't say whether a
	// VCure is online, and keeps answering with its last rows).
	vsFreshFor = 10 * time.Minute
	// vsDevicesEvery is how often the device list is fetched again.
	vsDevicesEvery = 10 * time.Minute
	// vsReloginEvery limits automatic sign-ins, like AC Infinity's.
	vsReloginEvery = time.Hour
)

// vsState is what the polling keeps between cycles (Run's goroutine only).
type vsState struct {
	devices   []vivosun.Device
	devicesAt time.Time
	listedFor map[string]bool      // device ids with sensors when the list was fetched
	lastRow   map[string]time.Time // device id -> newest row saved
	relogin   time.Time
}

func (w *Watcher) vivosunClient() *vivosun.Client {
	return &vivosun.Client{HTTP: w.HTTP, BaseURL: w.VSBaseURL}
}

// pollVivosunStayingSignedIn polls Vivosun; if it refuses the tokens and a
// password is saved, it signs in again (at most once an hour) and polls once
// more.
func (w *Watcher) pollVivosunStayingSignedIn(ctx context.Context) (bool, error) {
	contacted, err := w.PollVivosun(ctx)
	var pe *PollError
	if !errors.As(err, &pe) || pe.Kind != "auth" || w.VSRelogin == nil {
		return contacted, err
	}
	if !w.vs.relogin.IsZero() && w.Now().Sub(w.vs.relogin) < vsReloginEvery {
		return contacted, err
	}
	w.vs.relogin = w.Now()
	attempted, rerr := w.VSRelogin(ctx)
	switch {
	case !attempted:
		return contacted, err
	case rerr != nil:
		w.Logger.WithError(rerr).Warn("Automatic Vivosun sign-in failed; sign in again in Settings")
		return contacted, &PollError{Kind: "auth", Detail: "automatic sign-in failed: " + rerr.Error(), Err: rerr}
	}
	return w.PollVivosun(ctx)
}

// PollVivosun saves the newest one-minute reading of each Vivosun device
// that has sensors added in Manage sensors. contacted is false when nothing
// was asked of Vivosun (no sensors added, or not signed in).
func (w *Watcher) PollVivosun(ctx context.Context) (contacted bool, err error) {
	log := w.Logger.WithField("func", "PollVivosun")
	// device id -> reading name -> sensor ids
	tracked := map[string]map[string][]int{}
	rows, err := w.DB.Query("SELECT id, device, type FROM sensors WHERE source = $1", vivosun.Source)
	if err != nil {
		log.WithError(err).Error("Error loading Vivosun sensors")
		return false, nil
	}
	for rows.Next() {
		var id int
		var device, typ string
		if rows.Scan(&id, &device, &typ) != nil {
			continue
		}
		key := vivosun.KeyOf(typ)
		if key == "" {
			continue
		}
		if tracked[device] == nil {
			tracked[device] = map[string][]int{}
		}
		tracked[device][key] = append(tracked[device][key], id)
	}
	if err := rows.Err(); err != nil {
		log.WithError(err).Error("Error loading Vivosun sensors")
	}
	rows.Close()
	tk := vivosun.Tokens{}
	if w.VSTokens != nil {
		tk = w.VSTokens()
	}
	if len(tracked) == 0 || !tk.Set() {
		return false, nil
	}

	client := w.vivosunClient()
	now := w.Now()
	if err := w.vivosunDevices(ctx, client, tk, tracked, now); err != nil {
		return true, pollErr(err)
	}
	if w.vs.lastRow == nil {
		w.vs.lastRow = map[string]time.Time{}
	}
	var firstErr error
	saved := 0
	for _, d := range w.vs.devices {
		keys, ok := tracked[d.ID]
		if !ok {
			continue
		}
		list, err := client.Readings(ctx, tk, d, now.Add(-vsWindow), now)
		if err != nil {
			if errors.Is(err, vivosun.ErrAuth) {
				return true, pollErr(err)
			}
			log.WithError(err).WithField("device", d.Name).Warn("Error reading a Vivosun device")
			if firstErr == nil {
				firstErr = pollErr(err)
			}
			continue
		}
		if len(list) == 0 {
			continue
		}
		newest := list[len(list)-1]
		if !newest.At.After(w.vs.lastRow[d.ID]) || now.Sub(newest.At) > vsFreshFor {
			continue // already saved, or old: the device stopped sending
		}
		w.vs.lastRow[d.ID] = newest.At
		for key, ids := range keys {
			v, ok := newest.Values[key]
			if !ok {
				continue
			}
			v = vivosun.Value(d.Model, key, v)
			for _, id := range ids {
				if _, err := w.DB.Exec("INSERT INTO sensor_data (sensor_id, value) VALUES ($1, $2)", id, v); err != nil {
					log.WithError(err).WithField("sensorID", id).Error("Error writing sensor data to database")
					continue
				}
				saved++
			}
		}
	}
	log.WithField("readings", saved).Debug("Vivosun poll done")
	return true, firstErr
}

// vivosunDevices refreshes the device list every vsDevicesEvery, or sooner
// when sensors were added for a device that isn't in it. A device that was
// already missing at the last fetch (taken off the account) waits for the
// regular refresh, so it doesn't cost a request every poll.
func (w *Watcher) vivosunDevices(ctx context.Context, client *vivosun.Client, tk vivosun.Tokens, tracked map[string]map[string][]int, now time.Time) error {
	missing := false
	known := map[string]bool{}
	for _, d := range w.vs.devices {
		known[d.ID] = true
	}
	for id := range tracked {
		if !known[id] && !w.vs.listedFor[id] {
			missing = true
			break
		}
	}
	if !missing && !w.vs.devicesAt.IsZero() && now.Sub(w.vs.devicesAt) < vsDevicesEvery {
		return nil
	}
	devs, err := client.Devices(ctx, tk)
	if err != nil {
		w.Logger.WithError(err).Warn("Error listing Vivosun devices")
		return err
	}
	w.vs.devices, w.vs.devicesAt = devs, now
	w.vs.listedFor = make(map[string]bool, len(tracked))
	for id := range tracked {
		w.vs.listedFor[id] = true
	}
	if w.VSDeviceNames != nil {
		names := make(map[string]string, len(devs))
		for _, d := range devs {
			names[d.ID] = d.Name
		}
		w.VSDeviceNames(names)
	}
	return nil
}

// pollErr turns a Vivosun error into the poll's outcome.
func pollErr(err error) error {
	var refused *vivosun.RefusedError
	switch {
	case errors.Is(err, vivosun.ErrAuth):
		detail := ""
		if errors.As(err, &refused) {
			detail = refused.Msg
		}
		return &PollError{Kind: "auth", Detail: detail, Err: err}
	case errors.Is(err, vivosun.ErrUnreachable):
		return &PollError{Kind: "unreachable", Err: err}
	case errors.As(err, &refused):
		return &PollError{Kind: "error", Detail: refused.Msg, Err: err}
	}
	return &PollError{Kind: "error", Err: err}
}
