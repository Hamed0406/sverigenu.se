package fetcher

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// newTestClient returns a Client whose URL fields point at baseURL so tests
// can use httptest.Server without hitting the real internet.
func newTestClient(baseURL string) *Client {
	return &Client{
		http:                  &http.Client{Timeout: 5 * time.Second},
		smhiURL:               baseURL,
		luftURL:               baseURL,
		policeURL:             baseURL,
		scbURL:                baseURL,
		koladaURL:             baseURL,
		braURL:                baseURL,
		trafikURL:             baseURL,
		skolverketRegistryURL: baseURL,
		skolverketPlanURL:     baseURL,
		ssURL:                 baseURL,
		contactEmail:          "test@example.com",
	}
}

// testCtx returns a context that expires when the test ends.
func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}
