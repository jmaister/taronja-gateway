package handlers

import (
	"bytes"
	"context"

	_ "embed"

	"github.com/jmaister/taronja-gateway/api"
)

// GetOpenApiYaml returns the OpenAPI specification of Taronja Gateway in YAML format.
func (s *StrictApiServer) GetOpenApiYaml(ctx context.Context, request api.GetOpenApiYamlRequestObject) (api.GetOpenApiYamlResponseObject, error) {
	return api.GetOpenApiYaml200TextyamlResponse{
		Body:          bytes.NewReader(api.OpenApiSpecYaml),
		ContentLength: int64(len(api.OpenApiSpecYaml)),
	}, nil
}
