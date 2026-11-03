package types

import "fmt"

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
