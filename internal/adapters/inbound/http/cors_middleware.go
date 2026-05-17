package http

import (
	stdhttp "net/http"

	"github.com/go-chi/cors"
)

const corsPreflightMaxAgeSeconds = 300

// corsMiddleware returns a CORS handler that allows the configured origins to
// invoke the API from the browser using cookie-based authentication.
//
// When allowedOrigins is empty the middleware is a no-op, preserving the
// bundled same-origin deployment where the Next.js proxy reaches the API over
// the internal Docker network and the browser never issues a cross-origin
// request.
//
// Allowed credentials, methods and headers match what the bundled Next.js
// proxy forwards (authorization, cookie, x-csrf-token, x-request-id) so split
// deployments that talk to the API directly preserve the same auth surface.
func corsMiddleware(allowedOrigins []string) func(stdhttp.Handler) stdhttp.Handler {
	if len(allowedOrigins) == 0 {
		return func(next stdhttp.Handler) stdhttp.Handler { return next }
	}
	return cors.Handler(cors.Options{
		AllowedOrigins: allowedOrigins,
		AllowedMethods: []string{
			stdhttp.MethodGet,
			stdhttp.MethodPost,
			stdhttp.MethodPut,
			stdhttp.MethodPatch,
			stdhttp.MethodDelete,
			stdhttp.MethodOptions,
		},
		AllowedHeaders: []string{
			"Accept",
			"Accept-Language",
			"Authorization",
			"Content-Type",
			"Cookie",
			"X-CSRF-Token",
			"X-Request-Id",
		},
		ExposedHeaders:   []string{"X-Request-Id"},
		AllowCredentials: true,
		MaxAge:           corsPreflightMaxAgeSeconds,
	})
}
