package handlers

// Settings > Alerts > Sensor alerts: any sensor at or past a chosen value
// (rules in alerts/rule.go; the check is in alert_engine.go).

import (
	"database/sql"
	"errors"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"isley/alerts"
	"isley/logger"
)

// alertRuleRow is a sensor alert as the page and the save routes see it.
type alertRuleRow struct {
	ID          int     `json:"id"`
	SensorID    int     `json:"sensor_id"`
	Side        string  `json:"side"`
	Limit       float64 `json:"limit"`
	HoldMinutes int     `json:"hold_minutes"`
	Enabled     bool    `json:"enabled"`
}

// alertRuleSensor is a sensor the form can pick.
type alertRuleSensor struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Zone string `json:"zone"`
	Unit string `json:"unit"`
}

// alertRulesPage is the Sensor alerts card's data.
type alertRulesPage struct {
	Rules   []alertRuleRow    `json:"rules"`
	Sensors []alertRuleSensor `json:"sensors"`
	Hold    [2]int            `json:"hold"`
}

func loadAlertRules(db *sql.DB) ([]alerts.Rule, error) {
	rows, err := db.Query(`SELECT id, sensor_id, side, limit_value, hold_minutes, enabled FROM alert_rule ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []alerts.Rule
	for rows.Next() {
		var r alerts.Rule
		var hold, enabled int
		if err := rows.Scan(&r.ID, &r.SensorID, &r.Side, &r.Limit, &hold, &enabled); err != nil {
			return nil, err
		}
		r.Hold, r.Enabled = time.Duration(hold)*time.Minute, enabled != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

// alertRulesPageData lists the rules whose sensor still exists, and the sensors.
func alertRulesPageData(c *gin.Context) alertRulesPage {
	db := DBFromContext(c)
	page := alertRulesPage{Rules: []alertRuleRow{}, Sensors: []alertRuleSensor{}, Hold: alerts.RuleHoldRange}
	known := map[int]bool{}
	for _, s := range ListManagedSensors(db, ConfigStoreFromContext(c).PollingInterval()) {
		known[s.ID] = true
		page.Sensors = append(page.Sensors, alertRuleSensor{ID: s.ID, Name: s.Name, Zone: s.Zone, Unit: s.Unit})
	}
	rules, err := loadAlertRules(db)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to read sensor alerts")
	}
	for _, r := range rules {
		if known[r.SensorID] {
			page.Rules = append(page.Rules, ruleRow(r))
		}
	}
	return page
}

func ruleRow(r alerts.Rule) alertRuleRow {
	return alertRuleRow{ID: r.ID, SensorID: r.SensorID, Side: r.Side, Limit: r.Limit, HoldMinutes: int(r.Hold.Minutes()), Enabled: r.Enabled}
}

// alertRuleRequest is the form. Enabled and Limit are pointers so that leaving
// them out is an error, not off and zero.
type alertRuleRequest struct {
	SensorID    int      `json:"sensor_id"`
	Side        string   `json:"side"`
	Limit       *float64 `json:"limit"`
	HoldMinutes int      `json:"hold_minutes"`
	Enabled     *bool    `json:"enabled"`
}

// bindAlertRule reads and checks the form, and answers the request if it's wrong.
func bindAlertRule(c *gin.Context) (alertRuleRequest, bool) {
	var req alertRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Enabled == nil {
		apiBadRequest(c, "api_invalid_request_payload")
		return req, false
	}
	if req.Side != alerts.SideLow && req.Side != alerts.SideHigh {
		apiBadRequest(c, "api_alert_bad_rule")
		return req, false
	}
	if req.Limit == nil || math.IsNaN(*req.Limit) || math.Abs(*req.Limit) > alerts.RuleLimitMax {
		apiBadRequest(c, "api_alert_bad_number")
		return req, false
	}
	if req.HoldMinutes < alerts.RuleHoldRange[0] || req.HoldMinutes > alerts.RuleHoldRange[1] {
		apiBadRequest(c, "api_alert_bad_number")
		return req, false
	}
	var one int
	err := DBFromContext(c).QueryRow(`SELECT 1 FROM sensors WHERE id = $1`, req.SensorID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		apiBadRequest(c, "api_alert_bad_rule")
		return req, false
	}
	if err != nil {
		apiInternalError(c, "api_database_error")
		return req, false
	}
	return req, true
}

func alertRuleID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		apiBadRequest(c, "api_alert_bad_rule")
		return 0, false
	}
	return id, true
}

func (r alertRuleRequest) row(id int) alertRuleRow {
	return alertRuleRow{ID: id, SensorID: r.SensorID, Side: r.Side, Limit: *r.Limit, HoldMinutes: r.HoldMinutes, Enabled: *r.Enabled}
}

// AddAlertRuleHandler: POST /settings/alerts/rules.
func AddAlertRuleHandler(c *gin.Context) {
	req, ok := bindAlertRule(c)
	if !ok {
		return
	}
	db := DBFromContext(c)
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM alert_rule`).Scan(&n); err != nil {
		apiInternalError(c, "api_database_error")
		return
	}
	if n >= alerts.MaxRules {
		apiBadRequest(c, "api_alert_too_many_rules")
		return
	}
	var id int
	err := db.QueryRow(`INSERT INTO alert_rule (sensor_id, side, limit_value, hold_minutes, enabled) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		req.SensorID, req.Side, *req.Limit, req.HoldMinutes, boolInt(*req.Enabled)).Scan(&id)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to save a sensor alert")
		apiInternalError(c, "api_database_error")
		return
	}
	c.JSON(http.StatusOK, req.row(id))
}

// UpdateAlertRuleHandler: PUT /settings/alerts/rules/:id. A changed rule starts
// over: an open alert ends quietly, and the next check starts one if it still
// applies, with the new limit and sensor.
func UpdateAlertRuleHandler(c *gin.Context) {
	id, ok := alertRuleID(c)
	if !ok {
		return
	}
	req, ok := bindAlertRule(c)
	if !ok {
		return
	}
	db := DBFromContext(c)
	res, err := db.Exec(`UPDATE alert_rule SET sensor_id = $1, side = $2, limit_value = $3, hold_minutes = $4, enabled = $5 WHERE id = $6`,
		req.SensorID, req.Side, *req.Limit, req.HoldMinutes, boolInt(*req.Enabled), id)
	if err == nil {
		var n int64
		if n, err = res.RowsAffected(); err == nil && n == 0 {
			apiBadRequest(c, "api_alert_bad_rule")
			return
		}
	}
	if err == nil {
		err = closeRuleAlerts(db, id)
	}
	if err != nil {
		logger.Log.WithError(err).Error("Failed to save a sensor alert")
		apiInternalError(c, "api_database_error")
		return
	}
	c.JSON(http.StatusOK, req.row(id))
}

// DeleteAlertRuleHandler: DELETE /settings/alerts/rules/:id.
func DeleteAlertRuleHandler(c *gin.Context) {
	id, ok := alertRuleID(c)
	if !ok {
		return
	}
	db := DBFromContext(c)
	_, err := db.Exec(`DELETE FROM alert_rule WHERE id = $1`, id)
	if err == nil {
		err = closeRuleAlerts(db, id)
	}
	if err != nil {
		logger.Log.WithError(err).Error("Failed to delete a sensor alert")
		apiInternalError(c, "api_database_error")
		return
	}
	apiOK(c, "api_settings_saved")
}

// closeRuleAlerts ends a rule's open alert with no notice.
func closeRuleAlerts(db *sql.DB, id int) error {
	_, err := db.Exec(`UPDATE alert_event SET resolved_at = $1, pending = '', retry_at = NULL,
		delivery = CASE WHEN delivery = $2 THEN '' ELSE delivery END
		WHERE kind = $3 AND measure = $4 AND resolved_at IS NULL`,
		dbTime(time.Now()), alerts.DeliveryPending, alerts.KindSensor, alerts.RuleMeasure(id))
	return err
}
