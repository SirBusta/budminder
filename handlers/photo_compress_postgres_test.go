//go:build integration_postgres

// Photo compression on Postgres: migration 042's columns, the candidate
// query (LOWER/LIKE, the kept-at-a-higher-quality rule) and a full run that
// writes compress_quality/compress_result back.
//
// Local invocation:
//
//	go test -tags=integration_postgres -run Postgres ./handlers/...

package handlers_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
	"isley/utils"
)

func TestPhotoCompress_Postgres(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestPostgresDB(t)
	plant := testutil.SeedPlant(t, db, "PG Plant", testutil.SeedStrain(t, db, testutil.SeedBreeder(t, db, "PG B"), "PG S"), testutil.SeedZone(t, db, "PG Z"))
	uploads := t.TempDir()
	big := testutil.DetailedPhotoJPEG(t)
	small, err := utils.CompressJPEG(big, 50)
	require.NoError(t, err)
	id1 := testutil.SeedPlantImage(t, db, plant, filepath.Join(uploads, "plants", "A.JPG"), big)
	id2 := testutil.SeedPlantImage(t, db, plant, filepath.Join(uploads, "plants", "b.jpeg"), small)
	testutil.SeedPlantImage(t, db, plant, filepath.Join(uploads, "plants", "c.png"), []byte("png"))

	svc := handlers.NewPhotoCompressService(db, t.TempDir(), uploads)
	est, err := svc.Estimate(db, 75)
	require.NoError(t, err)
	assert.Equal(t, 2, est.Count, "JPEGs only, any case")

	require.NoError(t, svc.Start(75, false))
	svc.Wait()
	j := svc.Job()
	assert.Equal(t, handlers.PhotoJobFinished, j.State)
	assert.Equal(t, 1, j.Compressed)
	assert.Equal(t, 1, j.Kept)

	var q int
	var r string
	require.NoError(t, db.QueryRow(`SELECT compress_quality, compress_result FROM plant_images WHERE id = $1`, id1).Scan(&q, &r))
	assert.Equal(t, 75, q)
	assert.Equal(t, "compressed", r)
	require.NoError(t, db.QueryRow(`SELECT compress_result FROM plant_images WHERE id = $1`, id2).Scan(&r))
	assert.Equal(t, "kept", r)

	est, err = svc.Estimate(db, 60)
	require.NoError(t, err)
	assert.Equal(t, 1, est.Count, "only the kept photo is tried again at a lower quality")
}
