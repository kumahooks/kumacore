package httpx

import "net/http"

// LimitRequestBody caps the number of bytes read from a request body.
func LimitRequestBody(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if limit <= 0 {
			return next
		}

		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.Body != nil {
				request.Body = http.MaxBytesReader(writer, request.Body, limit)
			}

			next.ServeHTTP(writer, request)
		})
	}
}
