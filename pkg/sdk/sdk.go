package sdk

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/HanshalDabbiru/feature-flag-engine/pkg/domain"
)

// Client connects to a feature flag engine server, maintains a local copy of
// all flags, and evaluates them in memory without additional network round-trips.
type Client struct {
	serverURL string
	mu        sync.RWMutex
	flags     map[string]domain.FeatureFlag
}

// New returns a Client that will connect to the given serverURL.
func New(serverURL string) *Client {
	return &Client{
		serverURL: serverURL,
		flags:     make(map[string]domain.FeatureFlag),
	}
}

func (c *Client) Connect(ctx context.Context) error {
	resp, err := http.Get(c.serverURL + "/stream")
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return fmt.Errorf("unexpected status %s", resp.Status)
	}

	go func() {
		<-ctx.Done()
		resp.Body.Close()
	}()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		raw := strings.TrimPrefix(line, "data: ")
		var flag domain.FeatureFlag
		if err := json.Unmarshal([]byte(raw), &flag); err != nil {
			continue
		}
		c.mu.Lock()
		c.flags[flag.Key] = flag
		c.mu.Unlock()
	}
	return scanner.Err()
}

// Get returns the locally cached FeatureFlag for the given key.
// If the key does not exist, the zero value of domain.FeatureFlag is returned.
func (c *Client) Get(key string) domain.FeatureFlag {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.flags[key]
}
