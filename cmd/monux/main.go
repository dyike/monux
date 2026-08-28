package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"
	"unicode"

	"github.com/dyike/monux/internal/config"
	"github.com/dyike/monux/internal/httpapi"
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
		newInitCommand(&configPath),
		newDetectCommand(),
		newInputsCommand(&configPath),
		newStatusCommand(&configPath),
		newSwitchCommand(&configPath),
		newSetCommand(&configPath),
		newServeCommand(&configPath),
	)
	return root
}

func newInitCommand(configPath *string) *cobra.Command {
	var monitorID string
	currentName := defaultLocalInputName()
	var configuredInputs []string
	var configuredPeers []string
	command := &cobra.Command{
		Use:   "init",
		Short: "Detect the monitor and generate or refresh the configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := config.Config{Inputs: make(map[string]monitor.Input)}
			existed := false
			loaded, err := config.Load(*configPath)
			switch {
			case err == nil:
				cfg = loaded
				existed = true
			case errors.Is(err, os.ErrNotExist):
			case err != nil:
				return err
			}

			detector, err := newNativeBackend("")
			if err != nil {
				return err
			}
			displays, err := detector.Detect()
			if err != nil {
				return fmt.Errorf("detect monitors: %w", err)
			}
			display, err := selectDisplay(displays, monitorID, cfg.Monitor.ID)
			if err != nil {
				return err
			}

			backend, err := newNativeBackend(display.ID)
			if err != nil {
				return err
			}
			supported, supportedErr := backend.SupportedInputs()
			current, currentErr := backend.CurrentInput()

			for _, mapping := range configuredInputs {
				name, input, err := parseConfiguredInput(mapping)
				if err != nil {
					return err
				}
				cfg.Inputs[name] = input
			}
			for _, mapping := range configuredPeers {
				name, peerURL, err := parseConfiguredPeer(mapping)
				if err != nil {
					return err
				}
				upsertPeer(&cfg, name, peerURL)
			}
			if currentErr == nil && !hasInputValue(cfg.Inputs, current) {
				name := uniqueInputName(currentName, cfg.Inputs)
				cfg.Inputs[name] = current
			}
			sort.Slice(supported, func(i, j int) bool { return supported[i] < supported[j] })
			for _, input := range supported {
				if hasInputValue(cfg.Inputs, input) {
					continue
				}
				name := uniqueInputName(inputName(input), cfg.Inputs)
				cfg.Inputs[name] = input
			}
			if len(cfg.Inputs) == 0 {
				return fmt.Errorf("monitor %s did not provide a current or supported input: current: %v; capabilities: %v", display.ID, currentErr, supportedErr)
			}

			if cfg.Node.Name == "" {
				cfg.Node.Name = defaultLocalInputName()
			}
			cfg.Monitor.ID = display.ID
			if err := config.Save(*configPath, cfg); err != nil {
				return err
			}
			if supportedErr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not read monitor input capabilities: %v\n", supportedErr)
			}
			if currentErr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not read current monitor input: %v\n", currentErr)
			}

			action := "created"
			if existed {
				action = "updated"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", action, *configPath)
			fmt.Fprintf(cmd.OutOrStdout(), "monitor: %s (%s)\n", display.ID, display.Name)
			printConfiguredInputs(cmd, cfg.Inputs, current, currentErr)
			for _, peer := range cfg.Peers {
				fmt.Fprintf(cmd.OutOrStdout(), "peer: %s=%s\n", peer.Name, peer.URL)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "note: a monitor reports connector values, not the operating system connected to each port")
			return nil
		},
	}
	command.Flags().StringVar(&monitorID, "monitor", "", "monitor ID to use when more than one is detected")
	command.Flags().StringVar(&currentName, "current-name", currentName, "name assigned to the current input when it is not configured")
	command.Flags().StringArrayVar(&configuredInputs, "input", nil, "named input mapping in name=value form (repeatable)")
	command.Flags().StringArrayVar(&configuredPeers, "peer", nil, "peer mapping in name=url form (repeatable)")
	return command
}

func defaultLocalInputName() string {
	switch runtime.GOOS {
	case "darwin":
		return "mac"
	case "windows":
		return "windows"
	default:
		return runtime.GOOS
	}
}

func selectDisplay(displays []monitor.Display, requested, configured string) (monitor.Display, error) {
	if len(displays) == 0 {
		return monitor.Display{}, errors.New("no monitor was detected")
	}
	selectedID := strings.TrimSpace(requested)
	if selectedID == "" && len(displays) == 1 {
		return displays[0], nil
	}
	if selectedID == "" {
		selectedID = configured
	}
	for _, display := range displays {
		if display.ID == selectedID {
			return display, nil
		}
	}
	available := make([]string, 0, len(displays))
	for _, display := range displays {
		available = append(available, fmt.Sprintf("%s (%s)", display.ID, display.Name))
	}
	if selectedID != "" {
		return monitor.Display{}, fmt.Errorf("monitor %q was not detected; available: %s", selectedID, strings.Join(available, ", "))
	}
	return monitor.Display{}, fmt.Errorf("%d monitors were detected; rerun with --monitor <id>; available: %s", len(displays), strings.Join(available, ", "))
}

func parseConfiguredInput(mapping string) (string, monitor.Input, error) {
	name, value, found := strings.Cut(mapping, "=")
	name = strings.TrimSpace(name)
	if !found || name == "" || strings.TrimSpace(value) == "" {
		return "", 0, fmt.Errorf("invalid --input %q: use name=value (for example mac=hdmi-1)", mapping)
	}
	input, err := monitor.ParseInput(value)
	if err != nil {
		return "", 0, fmt.Errorf("invalid --input %q: %w", mapping, err)
	}
	return name, input, nil
}

func parseConfiguredPeer(mapping string) (string, string, error) {
	name, peerURL, found := strings.Cut(mapping, "=")
	name = strings.TrimSpace(name)
	peerURL = strings.TrimSpace(peerURL)
	if !found || name == "" || peerURL == "" {
		return "", "", fmt.Errorf("invalid --peer %q: use name=url (for example mac=http://192.168.5.82:8765)", mapping)
	}
	return name, peerURL, nil
}

func upsertPeer(cfg *config.Config, name, peerURL string) {
	for index := range cfg.Peers {
		if cfg.Peers[index].Name == name {
			cfg.Peers[index].URL = peerURL
			return
		}
	}
	cfg.Peers = append(cfg.Peers, config.PeerConfig{Name: name, URL: peerURL})
}

func hasInputValue(inputs map[string]monitor.Input, wanted monitor.Input) bool {
	for _, input := range inputs {
		if input == wanted {
			return true
		}
	}
	return false
}

func uniqueInputName(base string, inputs map[string]monitor.Input) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "input"
	}
	if _, exists := inputs[base]; !exists {
		return base
	}
	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s-%d", base, suffix)
		if _, exists := inputs[candidate]; !exists {
			return candidate
		}
	}
}

func inputName(input monitor.Input) string {
	name := strings.ToLower(input.ConnectorName())
	var result strings.Builder
	separator := false
	for _, character := range name {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			if separator && result.Len() > 0 {
				result.WriteByte('-')
			}
			result.WriteRune(character)
			separator = false
		} else {
			separator = true
		}
	}
	if result.Len() == 0 || name == "unknown" {
		return "input-" + input.String()
	}
	return result.String()
}

func printConfiguredInputs(cmd *cobra.Command, inputs map[string]monitor.Input, current monitor.Input, currentErr error) {
	names := make([]string, 0, len(inputs))
	for name := range inputs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		marker := ""
		if currentErr == nil && inputs[name] == current {
			marker = " (current)"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "input: %s=%s (%s)%s\n", name, inputs[name], inputs[name].ConnectorName(), marker)
	}
}

func newServeCommand(configPath *string) *cobra.Command {
	listenAddress := envOrDefault("MONUX_HTTP_LISTEN", "127.0.0.1:8765")
	token := os.Getenv("MONUX_HTTP_TOKEN")
	command := &cobra.Command{
		Use:   "serve",
		Short: "Run the monitor control HTTP API",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, backend, switcher, err := loadControl(*configPath)
			if err != nil {
				return err
			}
			listener, err := net.Listen("tcp", listenAddress)
			if err != nil {
				return fmt.Errorf("listen on %s: %w", listenAddress, err)
			}
			defer listener.Close()

			if token == "" && !isLoopbackAddress(listener.Addr().String()) {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: HTTP API is listening beyond localhost without authentication; set MONUX_HTTP_TOKEN")
			}
			server := &http.Server{
				Handler:           httpapi.New(backend, switcher, token),
				ReadHeaderTimeout: 5 * time.Second,
				IdleTimeout:       60 * time.Second,
				MaxHeaderBytes:    8 * 1024,
			}
			fmt.Fprintf(cmd.OutOrStdout(), "monux HTTP API listening on http://%s\n", listener.Addr())

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			serveErr := make(chan error, 1)
			go func() { serveErr <- server.Serve(listener) }()

			select {
			case err := <-serveErr:
				if errors.Is(err, http.ErrServerClosed) {
					return nil
				}
				return err
			case <-ctx.Done():
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := server.Shutdown(shutdownCtx); err != nil {
					return fmt.Errorf("shut down HTTP server: %w", err)
				}
				return nil
			}
		},
	}
	command.Flags().StringVar(&listenAddress, "listen", listenAddress, "HTTP listen address")
	command.Flags().StringVar(&token, "token", token, "Bearer token (prefer MONUX_HTTP_TOKEN)")
	return command
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func isLoopbackAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
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
				fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n", input, input.ConnectorName(), reportStatus, currentStatus, strings.Join(names[input], ","))
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
		Use:   "set <connector-or-vcp-value>",
		Short: "Set an input by connector name or raw VCP value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			input, err := monitor.ParseInput(args[0])
			if err != nil {
				return err
			}
			_, _, switcher, err := loadControl(*configPath)
			if err != nil {
				return err
			}
			if err := switcher.Set(input); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "set input to %s\n", input)
			return nil
		},
	}
}

func loadSwitcher(path string) (config.Config, *service.Switcher, error) {
	cfg, _, switcher, err := loadControl(path)
	return cfg, switcher, err
}

func loadControl(path string) (config.Config, monitor.Backend, *service.Switcher, error) {
	cfg, err := config.Load(path)
	if err != nil {
		return config.Config{}, nil, nil, err
	}
	rawBackend, err := newBackend(cfg)
	if err != nil {
		return config.Config{}, nil, nil, err
	}
	controller := service.NewSynchronizedBackend(rawBackend)
	peers := make([]service.Peer, 0, len(cfg.Peers))
	for _, peer := range cfg.Peers {
		peers = append(peers, service.Peer{Name: peer.Name, URL: peer.URL, Token: peer.Token})
	}
	routedController := service.NewPeerController(controller, peers, nil)
	return cfg, controller, service.NewSwitcher(routedController, cfg.Inputs), nil
}

func newBackend(cfg config.Config) (monitor.Backend, error) {
	return newNativeBackend(cfg.Monitor.ID)
}
