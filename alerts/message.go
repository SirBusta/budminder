package alerts

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"isley/climate"
)

// Pending notices on an event.
const (
	NoticeFired    = "fired"
	NoticeRepeat   = "repeat"
	NoticeResolved = "resolved"
)

// Delivery states on an event.
const (
	DeliveryDryRun  = "dry_run"
	DeliveryPending = "pending"
	DeliverySent    = "sent"
	DeliveryFailed  = "failed"
	DeliveryOff     = "off" // Send email is chosen, but the Email card is off or incomplete
)

// Event is one alert_event row.
type Event struct {
	ID            int
	Kind          string
	ZoneID        int
	Zone          string
	SensorID      *int
	Sensor        string
	Measure       string
	Side          string
	Value         *float64
	Limit         *float64
	Detail        string // clock alerts: seen on|seen off|schedule on|schedule off, as HH:MM; sensor alerts: the unit
	Started       time.Time
	Notified      time.Time
	Resolved      *time.Time
	Delivery      string
	DeliveryError string
	Pending       string
	Emailed       bool
}

// Words builds an event's text: T translates a key, Units is the Settings
// measurement units, Loc the Settings time zone.
type Words struct {
	T     func(key string) string
	Units string
	Loc   *time.Location
}

func (w Words) fill(key string, vals map[string]string) string {
	s := w.T(key)
	for k, v := range vals {
		s = strings.ReplaceAll(s, "{"+k+"}", v)
	}
	return s
}

// Amount formats a measure's value with its unit; temperatures are stored in
// °C and shown in the Settings units.
func (w Words) Amount(measure string, v *float64) string {
	if v == nil {
		return ""
	}
	switch measure {
	case climate.Temp:
		if w.Units == "metric" {
			return fmt.Sprintf("%.1f °C", *v)
		}
		return fmt.Sprintf("%.0f °F", *v*9/5+32)
	case climate.RH:
		return fmt.Sprintf("%.0f%%", *v)
	case climate.VPD:
		return fmt.Sprintf("%.2f kPa", *v)
	}
	return strconv.FormatFloat(*v, 'f', -1, 64)
}

func (w Words) measure(m string) string { return w.T("al_measure_" + m) }

// Clock formats a time in the Settings zone.
func (w Words) Clock(t time.Time) string {
	if w.Loc != nil {
		t = t.In(w.Loc)
	}
	return t.Format("3:04 PM")
}

func clockText(hhmm string) string {
	t, err := time.Parse("15:04", hhmm)
	if err != nil {
		return hhmm
	}
	return t.Format("3:04 PM")
}

// sensorAmount formats a sensor alert's number in the sensor's own unit.
func sensorAmount(v *float64, unit string) string {
	if v == nil {
		return ""
	}
	s := strconv.FormatFloat(math.Round(*v*100)/100, 'f', -1, 64)
	switch unit {
	case "":
		return s
	case "%":
		return s + unit
	}
	return s + " " + unit
}

// sensorWhere names a sensor alert's sensor, with its zone if it has one.
func sensorWhere(e Event) string {
	if e.Zone == "" {
		return e.Sensor
	}
	return e.Sensor + " (" + e.Zone + ")"
}

// Title is an event's short name; resolved says it's the back-to-normal one.
func (w Words) Title(e Event, resolved bool) string {
	vals := map[string]string{"measure": w.measure(e.Measure), "zone": e.Zone}
	if e.Kind == KindSensor {
		vals = map[string]string{"sensor": sensorWhere(e)}
		switch {
		case resolved:
			return w.fill("al_ev_sensor_ok", vals)
		case e.Side == SideLow:
			return w.fill("al_ev_sensor_low", vals)
		}
		return w.fill("al_ev_sensor_high", vals)
	}
	switch {
	case e.Kind == KindLight && resolved:
		return w.fill("al_ev_light_ok", vals)
	case e.Kind == KindLight:
		return w.fill("al_ev_light", vals)
	case e.Kind == KindClock:
		return w.fill("al_ev_clock", vals)
	case resolved:
		return w.fill("al_ev_ok", vals)
	case e.Kind == KindDrift:
		return w.fill("al_ev_drift", vals)
	case e.Side == SideLow:
		return w.fill("al_ev_low", vals)
	}
	return w.fill("al_ev_high", vals)
}

// Line is the sentence under an event's title.
func (w Words) Line(e Event, resolved bool) string {
	if e.Kind == KindSensor {
		key := "al_line_sensor_" + e.Side
		if resolved {
			key = "al_line_sensor_ok"
		}
		return w.fill(key, map[string]string{"sensor": e.Sensor, "value": sensorAmount(e.Value, e.Detail), "limit": sensorAmount(e.Limit, e.Detail)})
	}
	vals := map[string]string{"measure": w.measure(e.Measure), "zone": e.Zone,
		"value": w.Amount(e.Measure, e.Value), "limit": w.Amount(e.Measure, e.Limit)}
	key := ""
	switch e.Kind {
	case KindLight:
		switch {
		case resolved:
			key = "al_line_light_ok"
		case e.Side == SideOn:
			key = "al_line_light_on"
		default:
			key = "al_line_light_off"
		}
	case KindClock:
		key = "al_line_clock_late"
		if e.Side == SideEarly {
			key = "al_line_clock_early"
		}
		if e.Limit != nil {
			vals["minutes"] = strconv.Itoa(int(*e.Limit))
		}
		if p := strings.Split(e.Detail, "|"); len(p) == 4 {
			vals["seen_on"], vals["seen_off"] = clockText(p[0]), clockText(p[1])
			vals["sched_on"], vals["sched_off"] = clockText(p[2]), clockText(p[3])
		}
	default:
		switch {
		case resolved:
			key = "al_line_ok"
		default:
			key = "al_line_" + e.Kind + "_" + e.Side
		}
	}
	return w.fill(key, vals)
}

// severityRank sorts the most serious first.
func severityRank(kind string) int {
	switch Severity(kind) {
	case Critical:
		return 0
	case Warning:
		return 1
	}
	return 2
}

// BuildMessage is one email for every pending notice in events. The subject
// names the most serious.
func BuildMessage(w Words, events []Event, link string) Message {
	sorted := append([]Event(nil), events...)
	sort.SliceStable(sorted, func(i, j int) bool {
		ri, rj := sorted[i].Pending == NoticeResolved, sorted[j].Pending == NoticeResolved
		if ri != rj {
			return !ri // alerts before back-to-normal notices
		}
		return severityRank(sorted[i].Kind) < severityRank(sorted[j].Kind)
	})
	var paras []string
	for _, e := range sorted {
		resolved := e.Pending == NoticeResolved
		line := w.Title(e, resolved) + ". " + w.Line(e, resolved)
		if !resolved {
			line += " " + w.fill("al_since", map[string]string{"time": w.Clock(e.Started)})
			if e.Pending == NoticeRepeat {
				line += " " + w.T("al_still")
			}
		}
		paras = append(paras, line)
	}
	first := ""
	if len(sorted) > 0 {
		first = w.Title(sorted[0], sorted[0].Pending == NoticeResolved)
	}
	subject := w.fill("al_subject", map[string]string{"title": first})
	if len(sorted) > 1 {
		subject = w.fill("al_subject_more", map[string]string{"title": first, "n": strconv.Itoa(len(sorted) - 1)})
	}
	return Simple(subject, paras, link, w.T("al_open_budminder"))
}
