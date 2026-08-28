package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dyike/monux/internal/monitor"
)

type failingController struct{}

func (failingController) CurrentInput() (monitor.Input, error) { return 0, errors.New("inactive") }
func (failingController) SetInput(monitor.Input) error         { return errors.New("inactive") }

func TestPeerControllerFallsBackForCurrentAndSet(t *testing.T) {
	current := monitor.Input(0x11)
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer secret" {
			http.Error(writer, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/local/status":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"value":17}`))
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/local/set/0x0f":
			current = 0x0f
			writer.WriteHeader(http.StatusOK)
		default:
			http.NotFound(writer, request)
		}
	})
	client := handlerHTTPClient(handler)

	controller := NewPeerController(failingController{}, []Peer{{Name: "mac", URL: "http://mac", Token: "secret"}}, client)
	input, err := controller.CurrentInput()
	if err != nil || input != 0x11 {
		t.Fatalf("CurrentInput() = %s, %v", input, err)
	}
	if err := controller.SetInput(0x0f); err != nil {
		t.Fatalf("SetInput() error = %v", err)
	}
	if current != 0x0f {
		t.Fatalf("peer current = %s, want 0x0f", current)
	}
}

func TestPeerControllerUsesLocalBeforePeer(t *testing.T) {
	peerCalled := false
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		peerCalled = true
		return nil, errors.New("peer should not be called")
	})}

	local := &fakeController{current: 0x11}
	controller := NewPeerController(local, []Peer{{Name: "mac", URL: "http://mac"}}, client)
	if input, err := controller.CurrentInput(); err != nil || input != 0x11 {
		t.Fatalf("CurrentInput() = %s, %v", input, err)
	}
	if err := controller.SetInput(0x0f); err != nil {
		t.Fatalf("SetInput() error = %v", err)
	}
	if peerCalled {
		t.Fatal("peer was called despite local success")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func handlerHTTPClient(handler http.Handler) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response.Result(), nil
	})}
}
