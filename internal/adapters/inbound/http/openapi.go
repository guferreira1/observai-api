package http

import (
	_ "embed"
	stdhttp "net/http"
)

const (
	openAPIYAMLRoutePath   = "/v1/openapi.yaml"
	swaggerUIRoutePath     = "/docs"
	swaggerUIBasePath      = swaggerUIRoutePath + "/"
	swaggerUIAliasPath     = "/swagger"
	swaggerUIAliasBasePath = swaggerUIAliasPath + "/"
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
