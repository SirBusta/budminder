package handlers

// Internal tests for the export helpers in plant_activity_log.go.

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSlugify(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"basic", "Plant One", "plant-one"},
		{"mixed punctuation", "Plant!#1.A", "plant-1-a"},
		{"trims dashes", "  --foo--bar--  ", "foo-bar"},
		{"unicode is dashed", "Café", "caf"}, // non-ASCII letters dashed-out
		{"empty input → fallback", "", "plant"},
		{"only punctuation → fallback", "!!!", "plant"},
		{"underscores collapsed", "a__b__c", "a-b-c"},
		{"already a slug", "ok-name", "ok-name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, slugify(tc.in))
		})
	}
}

func TestCellRef(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "A3", cellRef("A", 3))
	assert.Equal(t, "AB12", cellRef("AB", 12))
	assert.Equal(t, "Z1", cellRef("Z", 1))
}
