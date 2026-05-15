// CoworkAgent — Your autonomous night-shift coding partner.
// Copyright (C) 2026  Raymond M
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

// Package main is the entry point for CoworkAgent. It registers the root,
// cowork, and index sub-commands via Cobra and delegates execution to the
// TUI layer.
package main

import (
	"fmt"
	"os"
	"strings"

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
		var task strings.Builder
		for i, a := range args {
			if i > 0 {
				task.WriteString(" ")
			}
			task.WriteString(a)
		}
		app := tui.NewWithTask(cfg, task.String())
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
