package types

import (
	"fmt"
	"math"
)

// StrainPack is one pack of seeds added to a strain (the first one or a
// restock): how many seeds, what it cost in cents (nil = not entered), and
// whether it was a freebie (counted in Value, not in Cost).
type StrainPack struct {
	ID         int    `json:"id"`
	Seeds      int    `json:"seeds"`
	PriceCents *int   `json:"price_cents"`
	Free       bool   `json:"free"`
	AddedOn    string `json:"added_on"` // "2006-01-02"
}

// LatestPack is the strain's newest pack, or nil when it has none.
func (s Strain) LatestPack() *StrainPack {
	if len(s.Packs) == 0 {
		return nil
	}
	return &s.Packs[len(s.Packs)-1]
}

// EarlierPacks are the packs before the newest one.
func (s Strain) EarlierPacks() []StrainPack {
	if len(s.Packs) < 2 {
		return nil
	}
	return s.Packs[:len(s.Packs)-1]
}

// PackPriceInput is the newest pack's price for a form's number field: ""
// when not entered, "150" or "12.50" otherwise.
func (s Strain) PackPriceInput() string {
	p := s.LatestPack()
	if p == nil || p.PriceCents == nil {
		return ""
	}
	c := *p.PriceCents
	if c%100 == 0 {
		return fmt.Sprintf("%d", c/100)
	}
	return fmt.Sprintf("%d.%02d", c/100, c%100)
}

// PackFree reports whether the newest pack was a freebie.
func (s Strain) PackFree() bool {
	p := s.LatestPack()
	return p != nil && p.Free
}

// Tagged reports whether the pack gets a tag (a price or Freebie).
func (p StrainPack) Tagged() bool { return p.Free || p.PriceCents != nil }

// PackTotals is what the seeds on hand cost and are worth, in cents
// (rounded): each pack's price scaled by the share of the packs' seeds
// still on hand (at most all of it). Cost leaves freebies out. Priced is
// false when no pack has a price; the totals are then not shown. Same
// rules as packTotals in strains-page.js.
type PackTotals struct {
	Priced bool
	Cost   int
	Value  int
}

// PackTotals works out Cost and Value for the strain's seeds on hand.
func (s Strain) PackTotals() PackTotals {
	var t PackTotals
	if s.SeedCount <= 0 {
		return t
	}
	bought, cost, value := 0, 0, 0
	for _, p := range s.Packs {
		bought += p.Seeds
		if p.PriceCents == nil {
			continue
		}
		t.Priced = true
		value += *p.PriceCents
		if !p.Free {
			cost += *p.PriceCents
		}
	}
	if bought <= 0 {
		return PackTotals{}
	}
	left := math.Min(1, float64(s.SeedCount)/float64(bought))
	t.Cost = int(math.Round(left * float64(cost)))
	t.Value = int(math.Round(left * float64(value)))
	return t
}
