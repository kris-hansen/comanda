package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/spf13/cobra"
)

var verbose bool
var debug bool

var rootCmd = &cobra.Command{
	Use:   "comanda",
	Short: "A DSL processor for handling model interactions",
	Long: `comanda is a command line tool that processes DSL configurations 
for model interactions and executes the specified actions.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		config.Verbose = verbose
		config.Debug = debug
	},
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "enable debug logging")
}

func Execute() {
	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true

	err := rootCmd.Execute()
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "unknown command") {
			cmd := strings.Trim(strings.TrimPrefix(errMsg, "unknown command"), `"`+` for "comanda"`)
			fmt.Printf("To process a file, use the 'process' command:\n\n   comanda process %s\n\n", cmd)
		} else {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}
