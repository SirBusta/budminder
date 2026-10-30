package climate

import (
	"math"
	"time"
)

// Reading is one sensor reading.
type Reading struct {
	At    time.Time
	Value float64
}

// LightRun is a stretch of readings with the lights on or off.
type LightRun struct {
	On       bool
	From, To time.Time
}

// LightRuns groups readings (oldest first) into on and off stretches. The
// lights count as on when a reading is above above.
func LightRuns(readings []Reading, above float64) []LightRun {
	var out []LightRun
	for _, r := range readings {
		on := r.Value > above
		if n := len(out); n > 0 && out[n-1].On == on {
			out[n-1].To = r.At
			continue
		}
		out = append(out, LightRun{On: on, From: r.At, To: r.At})
	}
	return out
}

// LightMismatch is a light sensor disagreeing with the schedule.
type LightMismatch struct {
	SeenOn bool
	Since  time.Time // the scheduled switch it missed, or the first reading that disagreed after one that agreed
}

// CheckLights compares a light sensor's readings over the last hold with the
// schedule. It finds a mismatch only when the schedule hasn't switched during
// hold and at least two readings, all disagreeing, cover it. agree is true
// when the latest two readings match the schedule; with fewer readings than
// that, neither result is set and the caller should wait.
func CheckLights(readings []Reading, above float64, s Schedule, stage string, now time.Time, loc *time.Location, hold time.Duration) (m *LightMismatch, agree bool) {
	want, scheduled := LightsOn(s, stage, now, loc)
	if !scheduled {
		return nil, false
	}
	var recent []Reading
	for _, r := range readings {
		if !r.At.Before(now.Add(-hold)) && !r.At.After(now) {
			recent = append(recent, r)
		}
	}
	if len(recent) < 2 {
		return nil, false
	}
	last := recent[len(recent)-2:]
	if (last[0].Value > above) == want && (last[1].Value > above) == want {
		return nil, true
	}
	sw, switched := LastSwitch(s, stage, now, loc)
	if switched && now.Sub(sw) < hold {
		return nil, false
	}
	for _, r := range recent {
		if (r.Value > above) == want {
			return nil, false
		}
	}
	// Readings must span most of hold, so a sensor that just came back
	// isn't taken for 15 minutes of disagreement.
	if recent[0].At.After(now.Add(-hold * 3 / 4)) {
		return nil, false
	}
	since := recent[0].At
	if switched {
		since = sw
	}
	// Lights that followed the switch and failed later disagree only from then.
	for i := len(readings) - 1; i >= 0 && !readings[i].At.Before(since); i-- {
		if (readings[i].Value > above) == want {
			if i+1 < len(readings) {
				since = readings[i+1].At
			}
			break
		}
	}
	return &LightMismatch{SeenOn: !want, Since: since}, false
}

// The clock-offset check: three days of switches, each about an hour off.
const (
	clockOffsetDays     = 3
	clockOffsetMin      = 50 * time.Minute
	clockOffsetMax      = 70 * time.Minute
	clockOffsetSearch   = 90 * time.Minute
	clockOffsetLookback = (clockOffsetDays + 1) * 24 * time.Hour
)

// ClockOffsetLookback is how much history ClockOffset needs.
const ClockOffsetLookback = clockOffsetLookback

// ClockShift says the lights switch steadily off the schedule.
type ClockShift struct {
	Early           bool      // the lights switch before the schedule
	Minutes         int       // about how far off, on average
	SeenOn, SeenOff time.Time // the latest switches the sensor saw
}

// ClockOffset looks for lights that switch about an hour before or after the
// schedule on each of the last three days, on and off alike: usually a
// controller's clock or a wrong time zone. The readings must run oldest first.
func ClockOffset(readings []Reading, above float64, s Schedule, stage string, now time.Time, loc *time.Location) *ClockShift {
	runs := LightRuns(readings, above)
	if len(runs) < 2 {
		return nil
	}
	var sum time.Duration
	var n, ons, offs int
	var shift ClockShift
	all := switches(s, stage, now, loc, clockOffsetDays+1)
	for i := len(all) - 1; i >= 0; i-- {
		sw := all[i]
		// A switch still within reach of a late change hasn't been seen yet.
		if sw.At.After(now.Add(-clockOffsetSearch)) {
			continue
		}
		if sw.On && ons == clockOffsetDays || !sw.On && offs == clockOffsetDays {
			continue
		}
		seen, ok := nearestChange(runs, sw)
		if !ok {
			return nil
		}
		d := seen.Sub(sw.At)
		abs := time.Duration(math.Abs(float64(d)))
		if abs < clockOffsetMin || abs > clockOffsetMax {
			return nil
		}
		if n > 0 && (d < 0) != shift.Early {
			return nil
		}
		shift.Early = d < 0
		if sw.On {
			if ons == 0 {
				shift.SeenOn = seen
			}
			ons++
		} else {
			if offs == 0 {
				shift.SeenOff = seen
			}
			offs++
		}
		sum += abs
		n++
	}
	if ons < clockOffsetDays || offs < clockOffsetDays {
		return nil
	}
	shift.Minutes = int((sum / time.Duration(n)).Round(time.Minute).Minutes())
	return &shift
}

// nearestChange finds the sensor's change in sw's direction closest to sw.
func nearestChange(runs []LightRun, sw Switch) (time.Time, bool) {
	var best time.Time
	bestGap := clockOffsetSearch + 1
	for _, r := range runs[1:] { // the first run's start isn't a change
		if r.On != sw.On {
			continue
		}
		gap := time.Duration(math.Abs(float64(r.From.Sub(sw.At))))
		if gap <= clockOffsetSearch && gap < bestGap {
			best, bestGap = r.From, gap
		}
	}
	return best, !best.IsZero()
}
