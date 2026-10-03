package cli

import (
	"fmt"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/spf13/cobra"
)

func newOpenAPICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "openapi",
		Short: "Print the OpenAPI specification file",
		Long: `Prints the OpenAPI specification embedded in this binary. Needs no config
file; redirect stdout to save it:

    tg openapi > taronja-gateway-api.yaml`,
		Run: func(cmd *cobra.Command, args []string) { printOpenAPISpec() },
	}
}

// printOpenAPISpec writes only the spec to stdout so redirecting it yields a valid file.
func printOpenAPISpec() {
	fmt.Print(string(api.OpenApiSpecYaml))
}
