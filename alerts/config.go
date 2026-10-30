package alerts

import (
	"database/sql"
	"errors"
	"strconv"
	"time"
)

// Settings rows for the Climate alerts card.
const (
	SettingClimateMode    = "alert_climate_mode" // ModeOff, ModeDryRun or ModeEmail
	SettingSafetyReadings = "alert_safety_readings"
	SettingWindowMinutes  = "alert_window_minutes"
	SettingDriftHours     = "alert_drift_hours"
	SettingPauseMinutes   = "alert_pause_minutes"
	SettingRepeatCritical = "alert_repeat_critical_minutes"
	SettingRepeatWarning  = "alert_repeat_warning_hours"
	SettingLanguage       = "alert_language" // the language emails are written in
)

// Climate alert modes. Dry run is the default: alerts are listed, nothing is sent.
const (
	ModeOff    = "off"
	ModeDryRun = "dry"
	ModeEmail  = "email"
)

// Allowed values for the card's numbers ([min, max]; 0 repeats mean never).
var (
	SafetyReadingsRange = [2]int{1, 10}
	WindowMinutesRange  = [2]int{5, 60}
	DriftHoursRange     = [2]int{1, 24}
	PauseMinutesRange   = [2]int{0, 60}
	RepeatCriticalRange = [2]int{0, 1440}
	RepeatWarningRange  = [2]int{0, 72}
)

// ClimateConfig is the Climate alerts card.
type ClimateConfig struct {
	Mode     string
	Timing   Timing
	Language string
}

// LoadClimateConfig reads the card, with defaults for what was never saved
// and for numbers out of range.
func LoadClimateConfig(db *sql.DB) (ClimateConfig, error) {
	cfg := ClimateConfig{Mode: ModeDryRun, Timing: DefaultTiming(), Language: "en"}
	get := func(name string) (string, error) {
		var v string
		err := db.QueryRow("SELECT value FROM settings WHERE name = $1", name).Scan(&v)
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return v, err
	}
	mode, err := get(SettingClimateMode)
	if err != nil {
		return cfg, err
	}
	if mode == ModeOff || mode == ModeDryRun || mode == ModeEmail {
		cfg.Mode = mode
	}
	if lang, err := get(SettingLanguage); err != nil {
		return cfg, err
	} else if lang != "" {
		cfg.Language = lang
	}
	for _, f := range []struct {
		name string
		rng  [2]int
		set  func(int)
	}{
		{SettingSafetyReadings, SafetyReadingsRange, func(n int) { cfg.Timing.SafetyReadings = n }},
		{SettingWindowMinutes, WindowMinutesRange, func(n int) { cfg.Timing.Window = time.Duration(n) * time.Minute }},
		{SettingDriftHours, DriftHoursRange, func(n int) { cfg.Timing.Drift = time.Duration(n) * time.Hour }},
		{SettingPauseMinutes, PauseMinutesRange, func(n int) { cfg.Timing.Pause = time.Duration(n) * time.Minute }},
		{SettingRepeatCritical, RepeatCriticalRange, func(n int) { cfg.Timing.RepeatCritical = time.Duration(n) * time.Minute }},
		{SettingRepeatWarning, RepeatWarningRange, func(n int) { cfg.Timing.RepeatWarning = time.Duration(n) * time.Hour }},
	} {
		v, err := get(f.name)
		if err != nil {
			return cfg, err
		}
		if n, err := strconv.Atoi(v); err == nil && n >= f.rng[0] && n <= f.rng[1] {
			f.set(n)
		}
	}
	return cfg, nil
}
