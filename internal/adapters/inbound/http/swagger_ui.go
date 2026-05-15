package http

import (
	stdhttp "net/http"
	"strings"

	"github.com/swaggest/swgui/v5emb"
)

func newSwaggerUIHandler() stdhttp.Handler {
	return v5emb.New("ObservAI API", openAPIYAMLRoutePath, swaggerUIBasePath)
}

func redirectSwaggerUIAlias(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	targetPath := swaggerUIRoutePath
	if strings.HasPrefix(request.URL.Path, swaggerUIAliasBasePath) {
		targetPath = swaggerUIBasePath + strings.TrimPrefix(request.URL.Path, swaggerUIAliasBasePath)
	}
	if request.URL.RawQuery != "" {
		targetPath += "?" + request.URL.RawQuery
	}
	stdhttp.Redirect(writer, request, targetPath, stdhttp.StatusTemporaryRedirect)
}
