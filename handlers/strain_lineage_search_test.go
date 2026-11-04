package handlers_test

// Lineage parents are also searched on StrainCompass as you type (when it's
// on), and strain names typed on the strain forms get capitals. The searching
// and capitalizing happen in the browser (isley-autocomplete.js); these tests
// check the pages switch them on.

import (
	"io"
	"net/http"
	"net/url"
	"strconv"
	"testing"

	"isley/tests/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (s *importServer) page(t *testing.T, path string) string {
	t.Helper()
	resp := s.client.Get(path)
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode, path)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(body)
}

func TestLineageSearch_StraincompassOnEditAndReview(t *testing.T) {
	t.Parallel()
	s := newImportServer(t)
	breeder := testutil.SeedBreeder(t, s.db, "Lineage Breeder")
	id := testutil.SeedStrain(t, s.db, breeder, "Lineage Strain")
	edit := "/strain/" + strconv.Itoa(id) + "/edit"

	page := s.page(t, edit)
	assert.Contains(t, page, `data-straincompass="1"`)
	assert.Contains(t, page, `data-group-mine="Your strains"`)
	assert.Contains(t, page, `data-sc-failed="Could not connect to StrainCompass"`)

	_, review := s.reviewPage(t, url.Values{"source": {"straincompass"}, "slug": {"haze-automatic"},
		"name": {"Haze Automatic"}, "listing_breeder": {"Dinafem Seeds"}})
	assert.Contains(t, review, `data-straincompass="1"`)

	s.server.ConfigStore.SetStraincompassEnabled(0)
	assert.Contains(t, s.page(t, edit), `data-straincompass=""`, "off in Settings: your strains only")
}

func TestStrainNames_CapitalizedOnTheForms(t *testing.T) {
	t.Parallel()
	s := newImportServer(t)
	breeder := testutil.SeedBreeder(t, s.db, "Caps Breeder")
	id := testutil.SeedStrain(t, s.db, breeder, "Caps Strain")

	assert.Regexp(t, `id="editStrainName"[^>]*data-capitalize`, s.page(t, "/strain/"+strconv.Itoa(id)+"/edit"))
	assert.Regexp(t, `id="editStrainName"[^>]*data-capitalize`, s.page(t, "/strain/new"))
	assert.Regexp(t, `id="spManualName"[^>]*data-capitalize`, s.page(t, "/strains"))
}
