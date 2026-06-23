package sdk

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/HanshalDabbiru/feature-flag-engine/pkg/domain"
)

// Client connects to a feature flag engine server, maintains a local copy of
// all flags, and evaluates them in memory without additional network round-trips.
type Client struct {
	serverURL      string
	mu             sync.RWMutex
	flags          map[string]domain.FeatureFlag
	reconnectDelay time.Duration
}

// New returns a Client that will connect to the given serverURL.
func New(serverURL string) *Client {
	return &Client{
		serverURL:      serverURL,
		flags:          make(map[string]domain.FeatureFlag),
		reconnectDelay: 2 * time.Second,
	}
}

// Connect opens a persistent SSE connection to serverURL/stream and keeps the
// local flag cache up to date as events arrive. If the connection drops, Connect
// waits reconnectDelay then reconnects automatically. It blocks until ctx is
// cancelled, at which point it stops reconnecting and returns nil.
func (c *Client) Connect(ctx context.Context) error {
	for {
		resp, err := http.Get(c.serverURL + "/stream")
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return fmt.Errorf("unexpected status %s", resp.Status)
		}

		connCtx, connCancel := context.WithCancel(ctx)
		go func() {
			<-connCtx.Done()
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

		_ = scanner.Err()
		connCancel()

		timer := time.NewTimer(c.reconnectDelay)
		select {
		case <-timer.C:
			continue
		case <-ctx.Done():
			timer.Stop()
			return nil
		}
	}
}

// SetReconnectDelay configures how long Connect waits between reconnect attempts.
// The default is 2 seconds. Useful in tests to speed up reconnection.
func (c *Client) SetReconnectDelay(d time.Duration) {
	c.reconnectDelay = d
}

// Get returns the locally cached FeatureFlag for the given key.
// If the key does not exist, the zero value of domain.FeatureFlag is returned.
func (c *Client) Get(key string) domain.FeatureFlag {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.flags[key]
}

// Evaluate walks the flag's rules in order and returns the first matching rule's
// Value. Returns DefaultValue if the flag is not found, disabled, or no rules match.
func (c *Client) Evaluate(key string, ctx domain.UserContext) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	flag, ok := c.flags[key]
	if !ok {
		return flag.DefaultValue
	}
	if !flag.Enabled {
		return flag.DefaultValue
	}
	for _, rule := range flag.Rules {
		matched := true
		for _, pred := range rule.Predicates {
			if !matchesPredicate(pred, ctx) {
				matched = false
				break
			}
		}
		if matched {
			return rule.Value
		}
	}
	return flag.DefaultValue
}

// matchesPredicate reports whether a single predicate holds for the given user context.
func matchesPredicate(p domain.Predicate, uctx domain.UserContext) bool {
	if len(p.Values) == 0 {
		return false
	}
	val := uctx[p.Attribute]
	switch p.Operator {
	case domain.EQUALS:
		for _, v := range p.Values {
			if val == v {
				return true
			}
		}
		return false
	case domain.NOT_EQUALS:
		for _, v := range p.Values {
			if val == v {
				return false
			}
		}
		return true
	case domain.CONTAINS:
		for _, v := range p.Values {
			if strings.Contains(val, v) {
				return true
			}
		}
		return false
	case domain.STARTS_WITH:
		for _, v := range p.Values {
			if strings.HasPrefix(val, v) {
				return true
			}
		}
		return false
	default:
		return false
	}
}
