package http

import (
	"crypto/subtle"
	stdhttp "net/http"
)

// CSRFHeaderName is the request header that carries the CSRF token in the
// double-submit pattern.
const CSRFHeaderName = "X-CSRF-Token"

var safeHTTPMethods = map[string]bool{
	stdhttp.MethodGet:     true,
	stdhttp.MethodHead:    true,
	stdhttp.MethodOptions: true,
}

// csrfMiddleware enforces double-submit CSRF protection for cookie-based
// sessions.
//
// Safe HTTP methods bypass the check. Requests not authenticated as a user
// (API-key bearer flows) also bypass because they do not rely on
// browser-attached cookies. For protected requests, the X-CSRF-Token header
// must match the oai_csrf cookie value using a constant-time comparison so
// the check does not leak through timing.
func csrfMiddleware() func(stdhttp.Handler) stdhttp.Handler {
	return func(next stdhttp.Handler) stdhttp.Handler {
		return stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
			if safeHTTPMethods[request.Method] {
				next.ServeHTTP(writer, request)
				return
			}
			principal, ok := PrincipalFromContext(request.Context())
			if !ok || principal.Source != AuthSourceUser {
				next.ServeHTTP(writer, request)
				return
			}
			cookie, err := request.Cookie(CSRFCookieName)
			header := request.Header.Get(CSRFHeaderName)
			if err != nil || cookie.Value == "" || header == "" {
				writeCSRFFailure(writer)
				return
			}
			if subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(header)) != 1 {
				writeCSRFFailure(writer)
				return
			}
			next.ServeHTTP(writer, request)
		})
	}
}

func writeCSRFFailure(writer stdhttp.ResponseWriter) {
	writeAuthFailure(writer, stdhttp.StatusForbidden, "csrf_token_invalid", "csrf token missing or invalid")
}
