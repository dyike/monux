package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/dyike/monux/internal/monitor"
	"github.com/dyike/monux/internal/service"
)

type Server struct {
	backend   monitor.Backend
	switcher  *service.Switcher
	token     string
	operation sync.Mutex
	handler   http.Handler
}

type inputResponse struct {
	Name      string `json:"name,omitempty"`
	Input     string `json:"input"`
	Value     uint16 `json:"value"`
	Connector string `json:"connector"`
}

type capabilityResponse struct {
	inputResponse
	Names    []string `json:"names,omitempty"`
	Reported *bool    `json:"reported"`
	Current  *bool    `json:"current"`
}

func New(backend monitor.Backend, switcher *service.Switcher, token string) *Server {
	server := &Server{backend: backend, switcher: switcher, token: token}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", server.health)
	mux.Handle("GET /api/v1/status", server.authorize(http.HandlerFunc(server.status)))
	mux.Handle("GET /api/v1/inputs", server.authorize(http.HandlerFunc(server.inputs)))
	mux.Handle("GET /api/v1/capabilities", server.authorize(http.HandlerFunc(server.capabilities)))
	mux.Handle("POST /api/v1/switch/{name}", server.authorize(http.HandlerFunc(server.switchInput)))
	mux.Handle("POST /api/v1/set/{value}", server.authorize(http.HandlerFunc(server.setInput)))
	server.handler = mux
	return server
}

func (s *Server) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	s.handler.ServeHTTP(writer, request)
}

func (s *Server) health(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) status(writer http.ResponseWriter, _ *http.Request) {
	s.operation.Lock()
	defer s.operation.Unlock()

	input, err := s.switcher.Current()
	if err != nil {
		writeError(writer, http.StatusInternalServerError, fmt.Errorf("read current input: %w", err))
		return
	}
	writeJSON(writer, http.StatusOK, s.response(input))
}

func (s *Server) inputs(writer http.ResponseWriter, _ *http.Request) {
	configured := s.switcher.Inputs()
	inputs := make([]inputResponse, 0, len(configured))
	for _, input := range configured {
		inputs = append(inputs, inputResponse{
			Name:      input.Name,
			Input:     input.Input.String(),
			Value:     uint16(input.Input),
			Connector: input.Input.ConnectorName(),
		})
	}
	writeJSON(writer, http.StatusOK, map[string]any{"inputs": inputs})
}

func (s *Server) capabilities(writer http.ResponseWriter, _ *http.Request) {
	s.operation.Lock()
	defer s.operation.Unlock()

	supported, supportedErr := s.backend.SupportedInputs()
	current, currentErr := s.backend.CurrentInput()
	values := make(map[monitor.Input]bool)
	reported := make(map[monitor.Input]bool)
	names := make(map[monitor.Input][]string)
	for _, input := range supported {
		values[input] = true
		reported[input] = true
	}
	for _, configured := range s.switcher.Inputs() {
		values[configured.Input] = true
		names[configured.Input] = append(names[configured.Input], configured.Name)
	}
	if currentErr == nil {
		values[current] = true
	}

	ordered := make([]monitor.Input, 0, len(values))
	for input := range values {
		ordered = append(ordered, input)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	inputs := make([]capabilityResponse, 0, len(ordered))
	for _, input := range ordered {
		sort.Strings(names[input])
		var reportStatus, currentStatus *bool
		if supportedErr == nil {
			reportStatus = boolPointer(reported[input])
		}
		if currentErr == nil {
			currentStatus = boolPointer(input == current)
		}
		inputs = append(inputs, capabilityResponse{
			inputResponse: inputResponse{
				Input:     input.String(),
				Value:     uint16(input),
				Connector: input.ConnectorName(),
			},
			Names:    names[input],
			Reported: reportStatus,
			Current:  currentStatus,
		})
	}
	warnings := make([]string, 0, 2)
	if supportedErr != nil {
		warnings = append(warnings, "could not read monitor input capabilities: "+supportedErr.Error())
	}
	if currentErr != nil {
		warnings = append(warnings, "could not read current input: "+currentErr.Error())
	}
	response := map[string]any{"inputs": inputs}
	if len(warnings) > 0 {
		response["warnings"] = warnings
	}
	writeJSON(writer, http.StatusOK, response)
}

func boolPointer(value bool) *bool {
	return &value
}

func (s *Server) switchInput(writer http.ResponseWriter, request *http.Request) {
	name := request.PathValue("name")
	input, ok := s.switcher.Input(name)
	if !ok {
		writeError(writer, http.StatusNotFound, fmt.Errorf("unknown input %q", name))
		return
	}

	s.operation.Lock()
	defer s.operation.Unlock()
	if err := s.switcher.Switch(name); err != nil {
		writeError(writer, http.StatusInternalServerError, fmt.Errorf("switch to %q: %w", name, err))
		return
	}
	writeJSON(writer, http.StatusOK, inputResponse{Name: name, Input: input.String(), Value: uint16(input), Connector: input.ConnectorName()})
}

func (s *Server) setInput(writer http.ResponseWriter, request *http.Request) {
	input, err := monitor.ParseInput(request.PathValue("value"))
	if err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}

	s.operation.Lock()
	defer s.operation.Unlock()
	if err := s.backend.SetInput(input); err != nil {
		writeError(writer, http.StatusInternalServerError, fmt.Errorf("set input to %s: %w", input, err))
		return
	}
	writeJSON(writer, http.StatusOK, s.response(input))
}

func (s *Server) response(input monitor.Input) inputResponse {
	response := inputResponse{Input: input.String(), Value: uint16(input), Connector: input.ConnectorName()}
	if name, ok := s.switcher.Name(input); ok {
		response.Name = name
	}
	return response
}

func (s *Server) authorize(next http.Handler) http.Handler {
	if s.token == "" {
		return next
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		provided, bearer := strings.CutPrefix(request.Header.Get("Authorization"), "Bearer ")
		if !bearer || len(provided) != len(s.token) || subtle.ConstantTimeCompare([]byte(provided), []byte(s.token)) != 1 {
			writer.Header().Set("WWW-Authenticate", "Bearer")
			writeError(writer, http.StatusUnauthorized, fmt.Errorf("invalid or missing bearer token"))
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func writeError(writer http.ResponseWriter, status int, err error) {
	writeJSON(writer, status, map[string]string{"error": err.Error()})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
