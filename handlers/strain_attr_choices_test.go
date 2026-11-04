package handlers

// The click lists on Add/Edit strain (strain_attr_choices.go): every listed
// name has a label in every language, and saved names show translated.

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"isley/model/types"
	"isley/utils"
)

func TestStrainAttrChoices_LabelInEveryLanguage(t *testing.T) {
	t.Parallel()
	for _, lang := range utils.AvailableLanguages {
		lcl := utils.TranslationService.GetTranslations(lang)
		for group, list := range strainAttrChoices {
			seen := map[string]bool{}
			for _, ch := range list {
				assert.NotEmpty(t, lcl[ch.Key], "%s: %s %s (%s)", lang, group, ch.Name, ch.Key)
				assert.False(t, seen[ch.Name], "%s listed twice in %s", ch.Name, group)
				seen[ch.Name] = true
			}
		}
		assert.Contains(t, lcl["strain_attr_level_from"], "{level}", lang)
	}
	// Saved names are English: the English labels must be the names.
	en := utils.TranslationService.GetTranslations("en")
	for group, list := range strainAttrChoices {
		for _, ch := range list {
			assert.Equal(t, ch.Name, en[ch.Key], "%s %s", group, ch.Key)
		}
	}
}

func TestStrainAttrLabel(t *testing.T) {
	t.Parallel()
	es := map[string]string{"sr_w_relaxed": "Relajado", "strain_tp_myrcene": "Mirceno"}
	assert.Equal(t, "Relajado", StrainAttrLabel("effects", "Relaxed", es))
	assert.Equal(t, "Relajado", StrainAttrLabel("effects", "relaxed", es), "any case")
	assert.Equal(t, "Mirceno", StrainAttrLabel("terpenes", "MYRCENE", es))
	assert.Equal(t, "Blueberry", StrainAttrLabel("flavors", "Blueberry", es), "not listed: as saved")
	assert.Equal(t, "Happy", StrainAttrLabel("effects", "Happy", es), "no translation: the listed name")
	assert.Equal(t, "Myrcene", StrainAttrLabel("effects", "Myrcene", es), "only the group's own list")
}

func TestStrainAttrPicker(t *testing.T) {
	t.Parallel()
	lcl := utils.TranslationService.GetTranslations("en")

	empty := StrainAttrPicker(nil, lcl)
	assert.Len(t, empty, 4)
	for i, g := range empty {
		assert.Equal(t, StrainAttrGroups[i], g.Group)
		assert.NotEmpty(t, g.Label)
		assert.NotEmpty(t, g.Options)
		assert.NotNil(t, g.Items, "an empty list, not null, for the page script")
	}

	level := "high"
	s := &types.Strain{Terpenes: []types.StrainAttribute{{Name: "Myrcene", Level: &level}}}
	got := StrainAttrPicker(s, lcl)
	assert.Equal(t, "terpenes", got[2].Group)
	assert.Equal(t, s.Terpenes, got[2].Items)
	assert.Equal(t, "May help with", got[3].Label)
}
