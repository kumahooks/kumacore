package httpx

import (
	"net/http"
	"time"
)

// cookieAttributes holds the cookie fields shared by SetCookie and ClearCookie.
func cookieAttributes(name string, secure bool) http.Cookie {
	return http.Cookie{
		Name:     name,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	}
}

// SetCookie writes a cookie with the given name, value, and expiry. The cookie is HttpOnly, scoped to Path "/", and
// uses SameSite=Strict. The Secure flag reflects the caller's transport policy (false for plain HTTP dev, true for
// production HTTPS) so the caller threads the flag from config, not the package.
func SetCookie(writer http.ResponseWriter, name string, value string, expiresAt time.Time, secure bool) {
	cookie := cookieAttributes(name, secure)
	cookie.Value = value
	cookie.Expires = expiresAt

	http.SetCookie(writer, &cookie)
}

// ClearCookie expires the cookie with the given name so the browser drops it. The cookie is zero-valued with a past
// expiry and MaxAge -1, mirroring the shape SetCookie writes so the cleared value overwrites any prior cookie for the
// same name.
func ClearCookie(writer http.ResponseWriter, name string, secure bool) {
	cookie := cookieAttributes(name, secure)
	cookie.Value = ""
	cookie.Expires = time.Unix(0, 0).UTC()
	cookie.MaxAge = -1

	http.SetCookie(writer, &cookie)
}
