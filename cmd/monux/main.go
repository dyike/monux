package main

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/dyike/monux/internal/config"
	"github.com/dyike/monux/internal/monitor"
	"github.com/dyike/monux/internal/service"
	"github.com/spf13/cobra"
)

var newNativeBackend = monitor.NewNativeBackend

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
		newInputsCommand(&configPath),
		newStatusCommand(&configPath),
		newSwitchCommand(&configPath),
		newSetCommand(&configPath),
	)
	return root
}

func newInputsCommand(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "inputs",
		Short: "List monitor-reported input sources",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, backend, err := loadInputsBackend(*configPath)
			if err != nil {
				return err
			}

			supported, supportedErr := backend.SupportedInputs()
			current, currentErr := backend.CurrentInput()
			if supportedErr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not read monitor input capabilities: %v; showing configured inputs\n", supportedErr)
			}
			if currentErr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not read current input: %v\n", currentErr)
			}

			values := make(map[monitor.Input]bool)
			reported := make(map[monitor.Input]bool)
			names := make(map[monitor.Input][]string)
			for _, input := range supported {
				values[input] = true
				reported[input] = true
			}
			for name, input := range cfg.Inputs {
				values[input] = true
				names[input] = append(names[input], name)
			}
			if currentErr == nil {
				values[current] = true
			}

			ordered := make([]monitor.Input, 0, len(values))
			for input := range values {
				ordered = append(ordered, input)
			}
			sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })

			writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(writer, "VALUE\tCONNECTOR\tREPORTED\tCURRENT\tNAME")
			for _, input := range ordered {
				reportStatus := "unknown"
				if supportedErr == nil {
					reportStatus = "no"
					if reported[input] {
						reportStatus = "yes"
					}
				}
				currentStatus := "no"
				if currentErr != nil {
					currentStatus = "unknown"
				} else if input == current {
					currentStatus = "yes"
				}
				sort.Strings(names[input])
				fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n", input, connectorName(input), reportStatus, currentStatus, strings.Join(names[input], ","))
			}
			return writer.Flush()
		},
	}
}

func loadInputsBackend(path string) (config.Config, monitor.Backend, error) {
	cfg, err := config.Load(path)
	if err == nil {
		backend, err := newBackend(cfg)
		return cfg, backend, err
	}
	if !errors.Is(err, os.ErrNotExist) {
		return config.Config{}, nil, err
	}

	detector, err := newNativeBackend("")
	if err != nil {
		return config.Config{}, nil, err
	}
	displays, err := detector.Detect()
	if err != nil {
		return config.Config{}, nil, fmt.Errorf("configuration %q does not exist and monitor detection failed: %w", path, err)
	}
	if len(displays) == 0 {
		return config.Config{}, nil, fmt.Errorf("configuration %q does not exist and no monitor was detected", path)
	}
	if len(displays) > 1 {
		return config.Config{}, nil, fmt.Errorf("configuration %q does not exist and %d monitors were detected; run monux detect and configure monitor.id", path, len(displays))
	}
	backend, err := newNativeBackend(displays[0].ID)
	if err != nil {
		return config.Config{}, nil, err
	}
	return config.Config{Inputs: map[string]monitor.Input{}}, backend, nil
}

func newDetectCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "detect",
		Short: "Detect DDC/CI-capable monitors",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			backend, err := newNativeBackend("")
			if err != nil {
				return err
			}
			displays, err := backend.Detect()
			if err != nil {
				return err
			}
			for _, display := range displays {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", display.ID, display.Name)
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
			fmt.Fprintf(cmd.OutOrStdout(), "switched to %s (%s)\n", args[0], cfg.Inputs[args[0]])
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
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}
			input, err := parseInput(args[0])
			if err != nil {
				return err
			}
			backend, err := newBackend(cfg)
			if err != nil {
				return err
			}
			if err := backend.SetInput(input); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "set input to %s\n", input)
			return nil
		},
	}
}

func loadSwitcher(path string) (config.Config, *service.Switcher, error) {
	cfg, err := config.Load(path)
	if err != nil {
		return config.Config{}, nil, err
	}
	controller, err := newBackend(cfg)
	if err != nil {
		return config.Config{}, nil, err
	}
	return cfg, service.NewSwitcher(controller, cfg.Inputs), nil
}

func newBackend(cfg config.Config) (monitor.Backend, error) {
	return newNativeBackend(cfg.Monitor.ID)
}

func parseInput(value string) (monitor.Input, error) {
	parsed, err := strconv.ParseUint(strings.TrimSpace(value), 0, 16)
	if err != nil {
		return 0, fmt.Errorf("invalid input value %q: use decimal or 0x-prefixed hexadecimal", value)
	}
	return monitor.Input(parsed), nil
}

func connectorName(input monitor.Input) string {
	names := map[monitor.Input]string{
		0x01: "VGA 1",
		0x02: "VGA 2",
		0x03: "DVI 1",
		0x04: "DVI 2",
		0x05: "Composite 1",
		0x06: "Composite 2",
		0x07: "S-Video 1",
		0x08: "S-Video 2",
		0x09: "Tuner 1",
		0x0a: "Tuner 2",
		0x0b: "Tuner 3",
		0x0c: "Component 1",
		0x0d: "Component 2",
		0x0e: "Component 3",
		0x0f: "DisplayPort 1",
		0x10: "DisplayPort 2",
		0x11: "HDMI 1",
		0x12: "HDMI 2",
		0x1b: "USB-C",
	}
	if name, ok := names[input]; ok {
		return name
	}
	return "Unknown"
}
