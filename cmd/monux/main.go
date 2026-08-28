package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/dyike/monux/internal/config"
	"github.com/dyike/monux/internal/monitor"
	"github.com/dyike/monux/internal/service"
	"github.com/spf13/cobra"
)

func main() {
	if err := newRootCommand().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	configPath := config.DefaultPath()
	root := &cobra.Command{
		Use:           "monux",
		Short:         "Switch a monitor between named input sources",
		SilenceUsage:  true,
		SilenceErrors: false,
	}
	root.PersistentFlags().StringVarP(&configPath, "config", "c", configPath, "configuration file")

	root.AddCommand(
		newDetectCommand(),
		newStatusCommand(&configPath),
		newSwitchCommand(&configPath),
		newSetCommand(&configPath),
	)
	return root
}

func newDetectCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "detect",
		Short: "Detect DDC/CI-capable monitors",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			output, err := monitor.NewDDCUtil(0).Detect()
			if err != nil {
				return err
			}
			if output != "" {
				fmt.Fprintln(cmd.OutOrStdout(), output)
			}
			return nil
		},
	}
}

func newStatusCommand(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the monitor's current input",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, switcher, err := loadSwitcher(*configPath)
			if err != nil {
				return err
			}
			input, err := switcher.Current()
			if err != nil {
				return err
			}
			if name, ok := switcher.Name(input); ok {
				fmt.Fprintf(cmd.OutOrStdout(), "%s (%s)\n", name, input)
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), input)
			}
			return nil
		},
	}
}

func newSwitchCommand(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "switch <name>",
		Short: "Switch to a named input from the configuration",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, switcher, err := loadSwitcher(*configPath)
			if err != nil {
				return err
			}
			if err := switcher.Switch(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "switched to %s (%s) on bus %d\n", args[0], cfg.Inputs[args[0]], cfg.Monitor.Bus)
			return nil
		},
	}
}

func newSetCommand(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "set <vcp-value>",
		Short: "Set a raw VCP input value (for example 0x0f)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, err := loadSwitcher(*configPath)
			if err != nil {
				return err
			}
			input, err := parseInput(args[0])
			if err != nil {
				return err
			}
			if err := monitor.NewDDCUtil(cfg.Monitor.Bus).SetInput(input); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "set input to %s on bus %d\n", input, cfg.Monitor.Bus)
			return nil
		},
	}
}

func loadSwitcher(path string) (config.Config, *service.Switcher, error) {
	cfg, err := config.Load(path)
	if err != nil {
		return config.Config{}, nil, err
	}
	controller := monitor.NewDDCUtil(cfg.Monitor.Bus)
	return cfg, service.NewSwitcher(controller, cfg.Inputs), nil
}

func parseInput(value string) (monitor.Input, error) {
	parsed, err := strconv.ParseUint(strings.TrimSpace(value), 0, 16)
	if err != nil {
		return 0, fmt.Errorf("invalid input value %q: use decimal or 0x-prefixed hexadecimal", value)
	}
	return monitor.Input(parsed), nil
}
