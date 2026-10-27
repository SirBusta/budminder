package handlers_test

// +parallel:serial — config.RestoreInProgress is process-wide

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"isley/config"
	"isley/handlers"
)

func TestPhotoCompress_RefusedDuringRestore(t *testing.T) {
	e := newPhotoEnv(t)
	config.RestoreInProgress.Store(true)
	defer config.RestoreInProgress.Store(false)
	assert.ErrorIs(t, e.svc.Start(75, false), handlers.ErrPhotoJobRestore)
	assert.Nil(t, e.svc.Job())
}
