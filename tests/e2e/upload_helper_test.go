package e2e

import (
	"io"
	"net/http"
	"testing"
)

// newUploadRequest builds an authenticated multipart request.
func newUploadRequest(t *testing.T, e *env, path, contentType string, body io.Reader) *http.Request {
	t.Helper()
	req, err := http.NewRequest("POST", e.server.URL+path, body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+e.token)
	return req
}
