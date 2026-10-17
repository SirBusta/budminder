package utils

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// With no logo ever uploaded there's no uploads/logos folder (there isn't one
// under utils/, where this test runs): an empty list, not a server error.
func TestListLogosHandler_NoFolderIsEmpty(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/listLogos", nil)
	ListLogosHandler(c)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"success":true,"logos":[]}`, w.Body.String())
}
