package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dyike/monux/internal/monitor"
	"github.com/dyike/monux/internal/service"
)

type fakeController struct {
	current   monitor.Input
	err       error
	supported []monitor.Input
}

func (f *fakeController) CurrentInput() (monitor.Input, error) { return f.current, f.err }
func (f *fakeController) SetInput(input monitor.Input) error {
	if f.err != nil {
		return f.err
	}
	f.current = input
	return nil
}
func (f *fakeController) Detect() ([]monitor.Display, error) { return nil, f.err }
func (f *fakeController) SupportedInputs() ([]monitor.Input, error) {
	return f.supported, f.err
}

func TestStatus(t *testing.T) {
	controller := &fakeController{current: 0x11}
	server := newTestServer(controller, "")
	response := request(t, server, http.MethodGet, "/api/v1/status", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body inputResponse
	decode(t, response, &body)
	if body.Name != "mac" || body.Input != "0x11" || body.Value != 17 {
		t.Fatalf("response = %#v", body)
	}
}

func TestSwitch(t *testing.T) {
	controller := &fakeController{current: 0x11}
	server := newTestServer(controller, "")
	response := request(t, server, http.MethodPost, "/api/v1/switch/linux", "")
	if response.Code != http.StatusOK || controller.current != 0x0f {
		t.Fatalf("status = %d, current = %s, body = %s", response.Code, controller.current, response.Body.String())
	}
	response = request(t, server, http.MethodPost, "/api/v1/switch/unknown", "")
	if response.Code != http.StatusNotFound {
		t.Fatalf("unknown input status = %d", response.Code)
	}
	response = request(t, server, http.MethodGet, "/api/v1/switch/linux", "")
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("wrong method status = %d", response.Code)
	}
}

func TestInputs(t *testing.T) {
	server := newTestServer(&fakeController{}, "")
	response := request(t, server, http.MethodGet, "/api/v1/inputs", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	var body struct {
		Inputs []inputResponse `json:"inputs"`
	}
	decode(t, response, &body)
	if len(body.Inputs) != 2 || body.Inputs[0].Name != "linux" || body.Inputs[1].Name != "mac" {
		t.Fatalf("inputs = %#v", body.Inputs)
	}
}

func TestCapabilities(t *testing.T) {
	backend := &fakeController{current: 0x11, supported: []monitor.Input{0x0f, 0x10, 0x11}}
	server := newTestServer(backend, "")
	response := request(t, server, http.MethodGet, "/api/v1/capabilities", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Inputs []capabilityResponse `json:"inputs"`
	}
	decode(t, response, &body)
	if len(body.Inputs) != 3 || body.Inputs[0].Reported == nil || !*body.Inputs[0].Reported || body.Inputs[0].Connector != "DisplayPort 1" {
		t.Fatalf("inputs = %#v", body.Inputs)
	}
	if body.Inputs[2].Current == nil || !*body.Inputs[2].Current || body.Inputs[2].Input != "0x11" {
		t.Fatalf("current input = %#v", body.Inputs[2])
	}
}

func TestCapabilitiesUseNullForFailedQueries(t *testing.T) {
	backend := &fakeController{err: errors.New("DDC failed")}
	server := newTestServer(backend, "")
	response := request(t, server, http.MethodGet, "/api/v1/capabilities", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Inputs   []capabilityResponse `json:"inputs"`
		Warnings []string             `json:"warnings"`
	}
	decode(t, response, &body)
	if len(body.Inputs) != 2 || body.Inputs[0].Reported != nil || body.Inputs[0].Current != nil {
		t.Fatalf("inputs = %#v", body.Inputs)
	}
	if len(body.Warnings) != 2 {
		t.Fatalf("warnings = %#v", body.Warnings)
	}
}

func TestSetRawInput(t *testing.T) {
	backend := &fakeController{current: 0x11}
	server := newTestServer(backend, "")
	response := request(t, server, http.MethodPost, "/api/v1/set/0x0f", "")
	if response.Code != http.StatusOK || backend.current != 0x0f {
		t.Fatalf("status = %d, current = %s, body = %s", response.Code, backend.current, response.Body.String())
	}
	response = request(t, server, http.MethodPost, "/api/v1/set/invalid", "")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid value status = %d", response.Code)
	}
}

func TestAuthorization(t *testing.T) {
	server := newTestServer(&fakeController{current: 0x11}, "secret")
	response := request(t, server, http.MethodGet, "/api/v1/status", "")
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status without token = %d", response.Code)
	}
	requestWithWrongScheme := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	requestWithWrongScheme.Header.Set("Authorization", "Basic secret")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, requestWithWrongScheme)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status with wrong auth scheme = %d", response.Code)
	}
	response = request(t, server, http.MethodGet, "/api/v1/status", "secret")
	if response.Code != http.StatusOK {
		t.Fatalf("status with token = %d, body = %s", response.Code, response.Body.String())
	}
	response = request(t, server, http.MethodGet, "/healthz", "")
	if response.Code != http.StatusOK {
		t.Fatalf("health status = %d", response.Code)
	}
}

func TestControllerError(t *testing.T) {
	server := newTestServer(&fakeController{err: errors.New("DDC failed")}, "")
	response := request(t, server, http.MethodGet, "/api/v1/status", "")
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func newTestServer(backend *fakeController, token string) *Server {
	switcher := service.NewSwitcher(backend, map[string]monitor.Input{"mac": 0x11, "linux": 0x0f})
	return New(backend, switcher, token)
}

func request(t *testing.T, handler http.Handler, method, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decode(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response: %v; body = %s", err, response.Body.String())
	}
}
