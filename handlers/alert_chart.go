package handlers

// What the Sensors page needs to show one alert from Recent alerts
// (sensors-page.js, opened as /sensors#alert=<id>): its zone, which charts it
// belongs to, its start and end, the limits it used and any other alert of the
// same kind in the time shown.

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"isley/alerts"
	"isley/climate"
)

// alertChartAround is how much time the Sensors page shows on each side of an
// alert: from this long before the hold that started it to this long after it ended.
const alertChartAround = time.Hour

// alertChartLimit is one limit in use. Why is what the chart names beside it
// when more than one limit is in view (alerts.LimitReasons); Stage is the
// translated stage or Flower phase for Why "stage".
type alertChartLimit struct {
	V     float64 `json:"v"`
	At    int64   `json:"at"`
	Why   string  `json:"why"`
	Stage string  `json:"stage,omitempty"`
}

type alertChartSpan struct {
	Start int64  `json:"start"`
	End   *int64 `json:"end"`
}

type alertChart struct {
	ZoneID  int    `json:"zone_id"`
	Kind    string `json:"kind"` // "limit" (Low or High limits and sensor alerts), "offline" or "light"
	Side    string `json:"side"`
	Rule    bool   `json:"rule"`    // a sensor alert: the limit is "Your limit"
	Sensors []int  `json:"sensors"` // the charts it belongs to; offline alerts: every chart of the zone
	alertChartSpan
	From    int64             `json:"from"`
	To      int64             `json:"to"`
	Limits  []alertChartLimit `json:"limits"`
	LimitIn string            `json:"limit_in"` // "c": °C; "sensor": the sensor's own unit; "": as the chart shows it
	Others  []alertChartSpan  `json:"others"`
}

func chartMs(t time.Time) int64 { return t.UnixMilli() }

func chartSpan(ev alerts.Event) alertChartSpan {
	s := alertChartSpan{Start: chartMs(ev.Started)}
	if ev.Resolved != nil {
		end := chartMs(*ev.Resolved)
		s.End = &end
	}
	return s
}

// buildAlertChart reads one alert and what goes with it.
func buildAlertChart(db *sql.DB, id int, now time.Time, w alerts.Words) (*alertChart, error) {
	rows, err := db.Query("SELECT "+alertEventColumns+" FROM alert_event WHERE id = $1", id)
	if err != nil {
		return nil, err
	}
	list, err := scanAlertEvents(rows)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 || list[0].Kind == alerts.KindClock {
		return nil, sql.ErrNoRows
	}
	ev := list[0]
	ch := &alertChart{ZoneID: ev.ZoneID, Kind: "limit", Side: ev.Side, alertChartSpan: chartSpan(ev), Sensors: []int{}, Limits: []alertChartLimit{}, Others: []alertChartSpan{}}
	switch ev.Kind {
	case alerts.KindOffline, alerts.KindLight:
		ch.Kind = ev.Kind
	case alerts.KindSensor:
		ch.Rule, ch.LimitIn = true, "sensor"
		// A sensor alert follows its sensor, wherever it is now.
		if ev.SensorID != nil {
			var zone int
			if err := db.QueryRow(`SELECT COALESCE(zone_id, 0) FROM sensors WHERE id = $1`, *ev.SensorID).Scan(&zone); err == nil {
				ch.ZoneID = zone
			}
		}
	case alerts.KindStage:
		if ev.Measure == climate.Temp {
			ch.LimitIn = "c"
		}
	}
	if ev.Kind != alerts.KindOffline && ev.SensorID != nil {
		ch.Sensors = append(ch.Sensors, *ev.SensorID)
	}
	if alerts.HasReadings(ev.Kind) {
		// A sensor alert's limit changes only by an edit, and the chart calls it "Your limit" throughout.
		var why []string
		if ev.Kind != alerts.KindSensor {
			why = alerts.LimitReasons(ev.Limits)
		}
		for i, l := range ev.Limits {
			lim := alertChartLimit{V: l.V, At: chartMs(l.At)}
			if why != nil {
				lim.Why = why[i]
			}
			if lim.Why == "stage" {
				lim.Stage = w.StageName(l)
			}
			ch.Limits = append(ch.Limits, lim)
		}
		if len(ch.Limits) == 0 && ev.Limit != nil {
			ch.Limits = append(ch.Limits, alertChartLimit{V: *ev.Limit, At: chartMs(ev.Started)})
		}
	}
	from := ev.Started.Add(-alerts.Hold - alertChartAround)
	to := now
	if ev.Resolved != nil && ev.Resolved.Add(alertChartAround).Before(now) {
		to = ev.Resolved.Add(alertChartAround)
	}
	ch.From, ch.To = chartMs(from), chartMs(to)

	rows, err = db.Query(`SELECT `+alertEventColumns+` FROM alert_event
		WHERE kind = $1 AND zone_id = $2 AND measure = $3 AND id <> $4 AND started_at < $5 AND (resolved_at IS NULL OR resolved_at > $6)
		ORDER BY started_at`, ev.Kind, ev.ZoneID, ev.Measure, ev.ID, dbTime(to), dbTime(from))
	if err != nil {
		return nil, err
	}
	others, err := scanAlertEvents(rows)
	if err != nil {
		return nil, err
	}
	for _, o := range others {
		ch.Others = append(ch.Others, chartSpan(o))
	}
	return ch, nil
}

// AlertChartHandler: GET /settings/alerts/events/:id/chart.
func AlertChartHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		apiBadRequest(c, "api_invalid_request_payload")
		return
	}
	ch, err := buildAlertChart(DBFromContext(c), id, time.Now(), alertWordsFor(c))
	switch {
	case errors.Is(err, sql.ErrNoRows):
		apiNotFound(c, "api_alert_not_found")
		return
	case err != nil:
		apiInternalError(c, "api_database_error")
		return
	}
	c.JSON(http.StatusOK, ch)
}
