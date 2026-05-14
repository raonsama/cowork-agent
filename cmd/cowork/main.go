package main

import (
	"fmt"
	"os"

	"github.com/raonsama/cowork-agent/internal/config"
	"github.com/raonsama/cowork-agent/internal/tui"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "cowork",
	Short: "CoworkAgent — Your autonomous night-shift coding partner",
	Long: `
  ██████╗ ██████╗ ██╗    ██╗ ██████╗ ██████╗ ██╗  ██╗
 ██╔════╝██╔═══██╗██║    ██║██╔═══██╗██╔══██╗██║ ██╔╝
 ██║     ██║   ██║██║ █╗ ██║██║   ██║██████╔╝█████╔╝
 ██║     ██║   ██║██║███╗██║██║   ██║██╔══██╗██╔═██╗
 ╚██████╗╚██████╔╝╚███╔███╔╝╚██████╔╝██║  ██║██║  ██╗
  ╚═════╝ ╚═════╝  ╚══╝╚══╝  ╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝

  Senior Ghost Developer — Always on shift.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("config load failed: %w", err)
		}
		app := tui.New(cfg)
		return app.Run()
	},
}

var coworkCmd = &cobra.Command{
	Use:   "cowork [task description]",
	Short: "Start autonomous cowork mode",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("config load failed: %w", err)
		}
		task := ""
		for i, a := range args {
			if i > 0 {
				task += " "
			}
			task += a
		}
		app := tui.NewWithTask(cfg, task)
		return app.Run()
	},
}

var indexCmd = &cobra.Command{
	Use:   "index [path]",
	Short: "Index a project directory",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := "."
		if len(args) > 0 {
			path = args[0]
		}
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("config load failed: %w", err)
		}
		app := tui.NewIndexer(cfg, path)
		return app.Run()
	},
}

func init() {
	rootCmd.AddCommand(coworkCmd)
	rootCmd.AddCommand(indexCmd)
	rootCmd.PersistentFlags().StringP("model", "m", "", "Override Ollama model")
	rootCmd.PersistentFlags().StringP("project", "p", ".", "Project root directory")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
