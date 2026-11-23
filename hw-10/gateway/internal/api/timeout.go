package api

import (
	"bytes"
	"context"
	"net/http"
	"time"
)

const timeoutMessage = "request canceled or timed out"

// Только middleware пишет в клиентское соединение: частичный или поздний
// ответ обработчика не может попасть в ответ 504.
func RequestTimeout(timeout time.Duration, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		if ctx.Err() != nil {
			writeError(w, http.StatusGatewayTimeout, timeoutMessage)
			return
		}
		response := &bufferedResponse{header: make(http.Header)}
		done := make(chan any, 1)
		go func() {
			defer func() { done <- recover() }()
			next.ServeHTTP(response, r.WithContext(ctx))
		}()
		select {
		case p := <-done:
			if p != nil {
				panic(p)
			}
			if ctx.Err() == nil {
				for k, values := range response.header {
					w.Header()[k] = values
				}
				if response.status != 0 {
					w.WriteHeader(response.status)
				}
				_, _ = w.Write(response.body.Bytes())
				return
			}
		case <-ctx.Done():
		}
		// Прерываем также чтение тела медленного POST-запроса.
		if r.Body != nil && r.Body != http.NoBody {
			_ = http.NewResponseController(w).SetReadDeadline(time.Now())
		}
		writeError(w, http.StatusGatewayTimeout, timeoutMessage)
	})
}

type bufferedResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (b *bufferedResponse) Header() http.Header { return b.header }
func (b *bufferedResponse) WriteHeader(status int) {
	if b.status == 0 {
		b.status = status
	}
}
func (b *bufferedResponse) Write(data []byte) (int, error) {
	b.WriteHeader(http.StatusOK)
	return b.body.Write(data)
}
