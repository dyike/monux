package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dyike/monux/internal/monitor"
)

type Peer struct {
	Name  string
	URL   string
	Token string
}

// PeerController uses the local DDC controller first and falls back to peers.
// Peer requests target local-only HTTP endpoints, so two nodes cannot relay a
// failed request back and forth indefinitely.
type PeerController struct {
	local  monitor.Controller
	peers  []Peer
	client *http.Client
}

func NewPeerController(local monitor.Controller, peers []Peer, client *http.Client) *PeerController {
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}
	return &PeerController{local: local, peers: append([]Peer(nil), peers...), client: client}
}

func (c *PeerController) CurrentInput() (monitor.Input, error) {
	input, localErr := c.local.CurrentInput()
	if localErr == nil {
		return input, nil
	}
	if len(c.peers) == 0 {
		return 0, localErr
	}

	errorsByNode := []error{fmt.Errorf("local DDC: %w", localErr)}
	for _, peer := range c.peers {
		input, err := c.peerCurrent(peer)
		if err == nil {
			return input, nil
		}
		errorsByNode = append(errorsByNode, fmt.Errorf("peer %s: %w", peer.Name, err))
	}
	return 0, fmt.Errorf("read current input from every node: %w", errors.Join(errorsByNode...))
}

func (c *PeerController) SetInput(input monitor.Input) error {
	localErr := c.local.SetInput(input)
	if localErr == nil {
		return nil
	}
	if len(c.peers) == 0 {
		return localErr
	}

	errorsByNode := []error{fmt.Errorf("local DDC: %w", localErr)}
	for _, peer := range c.peers {
		if err := c.peerSet(peer, input); err == nil {
			return nil
		} else {
			errorsByNode = append(errorsByNode, fmt.Errorf("peer %s: %w", peer.Name, err))
		}
	}
	return fmt.Errorf("set input through every node: %w", errors.Join(errorsByNode...))
}

func (c *PeerController) peerCurrent(peer Peer) (monitor.Input, error) {
	request, err := http.NewRequest(http.MethodGet, peerEndpoint(peer, "/api/v1/local/status"), nil)
	if err != nil {
		return 0, err
	}
	setPeerAuthorization(request, peer.Token)
	response, err := c.client.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return 0, peerHTTPError(response)
	}
	var body struct {
		Value uint16 `json:"value"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64*1024)).Decode(&body); err != nil {
		return 0, fmt.Errorf("decode response: %w", err)
	}
	return monitor.Input(body.Value), nil
}

func (c *PeerController) peerSet(peer Peer, input monitor.Input) error {
	path := "/api/v1/local/set/" + url.PathEscape(input.String())
	request, err := http.NewRequest(http.MethodPost, peerEndpoint(peer, path), nil)
	if err != nil {
		return err
	}
	setPeerAuthorization(request, peer.Token)
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return peerHTTPError(response)
	}
	return nil
}

func peerEndpoint(peer Peer, path string) string {
	return strings.TrimRight(peer.URL, "/") + path
}

func setPeerAuthorization(request *http.Request, token string) {
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
}

func peerHTTPError(response *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4*1024))
	detail := strings.TrimSpace(string(body))
	if detail == "" {
		detail = http.StatusText(response.StatusCode)
	}
	return fmt.Errorf("HTTP %d: %s", response.StatusCode, detail)
}
