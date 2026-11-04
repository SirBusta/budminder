package handlers

import (
	"strings"

	"isley/model/types"
)

// attrChoice is one button in a strain's click lists (Add/Edit strain):
// Name is what's saved, in English the way StrainCompass sends it, so an
// imported "Relaxed" lights up the Relaxed button; Key is the label's
// translation key.
type attrChoice struct {
	Name string `json:"name"`
	Key  string `json:"-"`
}

// strainAttrChoices are the click lists per group. Effects reuse the smoke
// report's words (sr_w_*); so do flavors, except where the smoke report's
// label joins two names ("Diesel/Gas", "Spicy/Pepper").
var strainAttrChoices = map[string][]attrChoice{
	"effects": {
		{"Relaxed", "sr_w_relaxed"}, {"Happy", "sr_w_happy"}, {"Euphoric", "sr_w_euphoric"}, {"Uplifted", "sr_w_uplifted"},
		{"Creative", "sr_w_creative"}, {"Energetic", "sr_w_energetic"}, {"Focused", "sr_w_focused"}, {"Talkative", "sr_w_talkative"},
		{"Giggly", "sr_w_giggly"}, {"Tingly", "sr_w_tingly"}, {"Calm", "sr_w_calm"}, {"Sleepy", "sr_w_sleepy"},
		{"Hungry", "sr_w_hungry"}, {"Aroused", "sr_w_aroused"}, {"Body high", "sr_w_body_high"}, {"Head high", "sr_w_head_high"},
	},
	"flavors": {
		{"Sweet", "sr_w_sweet"}, {"Earthy", "sr_w_earthy"}, {"Citrus", "sr_w_citrus"}, {"Lemon", "sr_w_lemon"},
		{"Orange", "sr_w_orange"}, {"Berry", "sr_w_berry"}, {"Grape", "sr_w_grape"}, {"Tropical", "sr_w_tropical"},
		{"Pine", "sr_w_pine"}, {"Gas", "strain_fl_gas"}, {"Diesel", "strain_fl_diesel"}, {"Skunk", "sr_w_skunk"},
		{"Spicy", "strain_fl_spicy"}, {"Herbal", "sr_w_herbal"}, {"Woody", "sr_w_woody"}, {"Floral", "sr_w_floral"},
		{"Cheese", "sr_w_cheese"}, {"Creamy", "sr_w_creamy"}, {"Vanilla", "sr_w_vanilla"}, {"Mint", "sr_w_mint"},
		{"Nutty", "sr_w_nutty"}, {"Chocolate", "sr_w_chocolate"}, {"Coffee", "sr_w_coffee"}, {"Candy", "sr_w_candy"},
		{"Sour", "sr_w_sour"},
	},
	"terpenes": {
		{"Myrcene", "strain_tp_myrcene"}, {"Limonene", "strain_tp_limonene"}, {"Caryophyllene", "strain_tp_caryophyllene"},
		{"Pinene", "strain_tp_pinene"}, {"Linalool", "strain_tp_linalool"}, {"Humulene", "strain_tp_humulene"},
		{"Terpinolene", "strain_tp_terpinolene"}, {"Ocimene", "strain_tp_ocimene"}, {"Bisabolol", "strain_tp_bisabolol"},
		{"Nerolidol", "strain_tp_nerolidol"}, {"Valencene", "strain_tp_valencene"}, {"Geraniol", "strain_tp_geraniol"},
		{"Eucalyptol", "strain_tp_eucalyptol"},
	},
	"medical_uses": {
		{"Stress", "strain_md_stress"}, {"Anxiety", "strain_md_anxiety"}, {"Depression", "strain_md_depression"},
		{"Pain", "strain_md_pain"}, {"Insomnia", "strain_md_insomnia"}, {"Fatigue", "strain_md_fatigue"},
		{"Lack of appetite", "strain_md_appetite"}, {"Nausea", "strain_md_nausea"}, {"Headaches", "strain_md_headaches"},
		{"Inflammation", "strain_md_inflammation"}, {"Muscle spasms", "strain_md_spasms"},
	},
}

// StrainAttrGroups is the order the groups appear in.
var StrainAttrGroups = []string{"effects", "flavors", "terpenes", "medical_uses"}

// StrainAttrLabel is how a saved name shows in the user's language: the
// translation when it's one of the group's listed names (any case), else
// the name as saved.
func StrainAttrLabel(group, name string, translations map[string]string) string {
	for _, ch := range strainAttrChoices[group] {
		if strings.EqualFold(ch.Name, name) {
			if t := translations[ch.Key]; t != "" {
				return t
			}
			return ch.Name
		}
	}
	return name
}

// attrPickOption is a listed name with its label in the user's language.
type attrPickOption struct {
	Name  string `json:"name"`
	Label string `json:"label"`
}

// attrPickGroup is one group's data for strain-attr-pick.js.
type attrPickGroup struct {
	Group   string                  `json:"group"`
	Label   string                  `json:"label"`
	Options []attrPickOption        `json:"options"`
	Items   []types.StrainAttribute `json:"items"`
}

// StrainAttrPicker is the click lists' data for Add/Edit strain (and the
// import review): every group's listed names and the strain's saved ones.
// s may be nil (Add strain).
func StrainAttrPicker(s *types.Strain, translations map[string]string) []attrPickGroup {
	saved := map[string][]types.StrainAttribute{}
	if s != nil {
		saved = map[string][]types.StrainAttribute{
			"effects": s.Effects, "flavors": s.Flavors, "terpenes": s.Terpenes, "medical_uses": s.MedicalUses,
		}
	}
	labels := map[string]string{
		"effects": "straincompass_effects", "flavors": "straincompass_flavors",
		"terpenes": "straincompass_terpenes", "medical_uses": "straincompass_medical_uses",
	}
	out := make([]attrPickGroup, 0, len(StrainAttrGroups))
	for _, g := range StrainAttrGroups {
		opts := make([]attrPickOption, 0, len(strainAttrChoices[g]))
		for _, ch := range strainAttrChoices[g] {
			opts = append(opts, attrPickOption{Name: ch.Name, Label: StrainAttrLabel(g, ch.Name, translations)})
		}
		items := saved[g]
		if items == nil {
			items = []types.StrainAttribute{}
		}
		out = append(out, attrPickGroup{Group: g, Label: translations[labels[g]], Options: opts, Items: items})
	}
	return out
}
