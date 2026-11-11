package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"hw-9/gateway/internal/api"
)

func TestRequestTimeoutDiscardsPartialResponse(t *testing.T) {
	for _, tc := range []struct {
		name         string
		timeout      time.Duration
		cancelClient bool
	}{
		{"deadline", 40 * time.Millisecond, false},
		{"client_cancel", 3 * time.Second, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ended := make(chan error, 1)
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Partial", "yes")
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte("partial"))
				if tc.cancelClient {
					cancel()
				}
				<-r.Context().Done()
				_, _ = w.Write([]byte("late"))
				ended <- r.Context().Err()
			})
			req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
			res := httptest.NewRecorder()
			api.JSONContentType(api.RequestTimeout(tc.timeout, handler)).ServeHTTP(res, req)
			select {
			case err := <-ended:
				if tc.cancelClient {
					require.ErrorIs(t, err, context.Canceled)
				} else {
					require.ErrorIs(t, err, context.DeadlineExceeded)
				}
			case <-time.After(time.Second):
				t.Fatal("handler did not observe cancellation")
			}
			assert.Empty(t, res.Header().Get("X-Partial"))
			assert.Equal(t, "request canceled or timed out", assertJSON[api.ErrorResponse](t, res, http.StatusGatewayTimeout).Error)
		})
	}
}
