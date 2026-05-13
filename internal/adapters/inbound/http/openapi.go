package http

import (
	_ "embed"
	stdhttp "net/http"
)

//go:embed openapi.yaml
var openAPIDocument []byte

// OpenAPIDocument returns the embedded OpenAPI 3.1 specification bytes.
func OpenAPIDocument() []byte {
	return openAPIDocument
}

func (router *Router) handleOpenAPI(writer stdhttp.ResponseWriter, _ *stdhttp.Request) {
	writer.Header().Set("Content-Type", "application/yaml")
	writer.Header().Set("Cache-Control", "public, max-age=300")
	writer.WriteHeader(stdhttp.StatusOK)
	_, _ = writer.Write(openAPIDocument)
}
