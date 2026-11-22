// Package security provides HTTP security middleware.
package security

import (
	"net/http"
	"net/url"
	"strings"
)

const contentSecurityPolicy = "default-src 'self'; object-src 'none'; base-uri 'none'; " +
	"form-action 'self'; style-src 'self'; style-src-attr 'unsafe-inline'; " +
	"script-src 'self'; img-src 'self'"

// Middleware sets security-related response headers on every request.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Frame-Options", "DENY")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		writer.Header().Set("Content-Security-Policy", contentSecurityPolicy)
		writer.Header().Set(
			"Permissions-Policy",
			"camera=(), microphone=(), geolocation=(), payment=(), usb=()",
		)

		next.ServeHTTP(writer, request)
	})
}

// SameOriginMiddleware rejects unsafe requests whose Origin or Referer names a different host.
func SameOriginMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !unsafeMethod(request.Method) || sameOrigin(request) {
			next.ServeHTTP(writer, request)
			return
		}

		http.Error(writer, "cross-origin request rejected", http.StatusBadRequest)
	})
}

func unsafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return false
	default:
		return true
	}
}

func sameOrigin(request *http.Request) bool {
	// Fetch Metadata is only present on browser requests. A cross-site navigation or form post is rejected even if the
	// browser omitted both Origin and Referer.
	if request.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return false
	}

	// Only request.Host is trusted. X-Forwarded-Host is client-controlled unless a reverse proxy overwrites it, so
	// honoring it would let a directly exposed instance accept a spoofed origin.
	if origin := request.Header.Get("Origin"); origin != "" {
		return originMatchesHost(origin, request.Host)
	}

	if referer := request.Header.Get("Referer"); referer != "" {
		return originMatchesHost(referer, request.Host)
	}

	return true
}

func originMatchesHost(rawURL string, requestHost string) bool {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return false
	}

	if parsedURL.Host == "" {
		return false
	}

	return strings.EqualFold(parsedURL.Host, requestHost)
}
