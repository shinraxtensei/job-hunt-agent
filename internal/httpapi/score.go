package httpapi

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/shinraxtensei/job-hunt-agent/internal/store"
)

// FitLevels are the score bands, best first (M3-06); "unscored" is the filter value for
// leads without a score.
var FitLevels = []string{"very_strong", "strong", "good", "stretch", "no"}

// factorOrder is the breakdown's order: the stored factors, then freshness (added when
// read, never stored: ADR-0021).
var factorOrder = []string{"required", "title", "seniority", "location", "optional"}

// Factor is one line of a score's breakdown.
type Factor struct {
	Factor string  `json:"factor"`
	Weight float64 `json:"weight"`
	Value  float64 `json:"value"`  // 0..1
	Points float64 `json:"points"` // weight × value, rounded to 0.1
	Detail string  `json:"detail"` // how code decided it; from the profile and the extraction
}

// freshnessParams and bands are stored with each score: what it was scored with.
type freshnessParams struct {
	Weight    float64 `json:"weight"`
	FreshDays float64 `json:"fresh_days"`
	StaleDays float64 `json:"stale_days"`
	Unknown   float64 `json:"unknown"`
}

type bands struct {
	VeryStrong float64 `json:"very_strong"`
	Strong     float64 `json:"strong"`
	Good       float64 `json:"good"`
	Stretch    float64 `json:"stretch"`
	// M3-09: location/remote/visa points below this cap the level at stretch.
	LocationGate float64 `json:"location_gate"`
}

// freshness mirrors score.py: 1 up to fresh_days, 0 from stale_days, linear between, in
// whole days since published (so the score order holds within a day).
func freshness(published *time.Time, now time.Time, p freshnessParams) float64 {
	if published == nil {
		return p.Unknown
	}
	age := math.Floor(now.Sub(*published).Hours() / 24)
	if age <= p.FreshDays {
		return 1
	}
	return math.Max(0, (p.StaleDays-age)/(p.StaleDays-p.FreshDays))
}

// fitLevel is the band of total, capped at stretch when the location points are below
// the gate (M3-09, as score.py's fit_level). The total itself is never changed.
func fitLevel(total, location float64, b bands) string {
	for i, cut := range []float64{b.VeryStrong, b.Strong, b.Good, b.Stretch} {
		if total >= cut {
			if location < b.LocationGate && i < 3 {
				return "stretch"
			}
			return FitLevels[i]
		}
	}
	return "no"
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
func round2(v float64) float64 { return math.Round(v*100) / 100 }

// applyScore sets a lead's score fields from its row, as of now. Model output (reason,
// gaps) is copied as data: the API only ever JSON-encodes it.
func applyScore(l *Lead, r store.ListLeadScoresRow, now time.Time) error {
	l.ScoreStatus = scoreStatus(r)
	if !r.Points.Valid {
		return nil
	}
	var factors map[string]struct {
		Value  float64 `json:"value"`
		Detail string  `json:"detail"`
		Weight float64 `json:"weight"`
	}
	var fp freshnessParams
	var b bands
	for _, j := range []struct {
		raw []byte
		v   any
	}{{r.Factors, &factors}, {r.Freshness, &fp}, {r.Bands, &b}} {
		if err := json.Unmarshal(j.raw, j.v); err != nil {
			return fmt.Errorf("decoding score of posting %d: %w", l.ID, err)
		}
	}
	f := freshness(l.PublishedAt, now, fp)
	total := r.Points.Float64 + fp.Weight*f
	l.Score = new(round1(total))
	loc := factors["location"]
	l.FitLevel = new(fitLevel(round1(total), loc.Weight*loc.Value, b)) // the level of the score shown
	l.NotEnoughInfo = r.NotEnoughInfo.Bool
	l.Breakdown = []Factor{}
	for _, k := range factorOrder {
		x := factors[k]
		l.Breakdown = append(l.Breakdown, Factor{k, x.Weight, round2(x.Value), round1(x.Weight * x.Value), x.Detail})
	}
	l.Breakdown = append(l.Breakdown, Factor{"freshness", fp.Weight, round2(f), round1(fp.Weight * f), ""})
	switch { // M4-06: the variant chosen with the score; the reason's for older rows
	case r.ScoreVariant.Valid:
		l.CVVariant = &r.ScoreVariant.String
	case r.CvVariant.Valid:
		l.CVVariant = &r.CvVariant.String
	}
	if r.Reason.Valid {
		l.Reason = &r.Reason.String
		if err := json.Unmarshal(r.Gaps, &l.Gaps); err != nil {
			return fmt.Errorf("decoding gaps of posting %d: %w", l.ID, err)
		}
	}
	l.ReasonCurrent = r.ReasonCurrent
	return nil
}

// scoreStatus: needs_review when the extraction or the reason failed (the owner looks),
// pending while either is still to come, scored when the score and a reason exist.
func scoreStatus(r store.ListLeadScoresRow) string {
	switch {
	case r.ExtractState == "failed" || r.ReasonState.String == "failed":
		return "needs_review"
	case !r.Points.Valid || !r.Reason.Valid:
		return "pending"
	}
	return "scored"
}
