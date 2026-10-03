package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Display version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Taronja Gateway v%s\n", a.build.Version)
			fmt.Printf("  Commit: %s\n", a.build.Commit)
			fmt.Printf("  Built: %s\n", a.build.Date)
			fmt.Printf("  OS/Arch: %s/%s\n", a.build.OS, a.build.Arch)
		},
	}
}
