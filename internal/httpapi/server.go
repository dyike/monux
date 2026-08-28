package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/dyike/monux/internal/monitor"
	"github.com/dyike/monux/internal/service"
)

type Server struct {
	switcher  *service.Switcher
	token     string
	operation sync.Mutex
	handler   http.Handler
}

type inputResponse struct {
	Name  string `json:"name,omitempty"`
	Input string `json:"input"`
	Value uint16 `json:"value"`
}

func New(switcher *service.Switcher, token string) *Server {
	server := &Server{switcher: switcher, token: token}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", server.health)
	mux.Handle("GET /api/v1/status", server.authorize(http.HandlerFunc(server.status)))
	mux.Handle("GET /api/v1/inputs", server.authorize(http.HandlerFunc(server.inputs)))
	mux.Handle("POST /api/v1/switch/{name}", server.authorize(http.HandlerFunc(server.switchInput)))
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
			Name:  input.Name,
			Input: input.Input.String(),
			Value: uint16(input.Input),
		})
	}
	writeJSON(writer, http.StatusOK, map[string]any{"inputs": inputs})
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
	writeJSON(writer, http.StatusOK, inputResponse{Name: name, Input: input.String(), Value: uint16(input)})
}

func (s *Server) response(input monitor.Input) inputResponse {
	response := inputResponse{Input: input.String(), Value: uint16(input)}
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
		provided := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
		if len(provided) != len(s.token) || subtle.ConstantTimeCompare([]byte(provided), []byte(s.token)) != 1 {
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
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
