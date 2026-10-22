package ingest

import (
	"bufio"
	"context"
	"net/url"
	"strings"
	"sync"
)

// RobotsRule is one parsed Allow/Disallow line.
type RobotsRule struct {
	Allow   bool
	Pattern string
}

// RobotsGroup is a User-agent block.
type RobotsGroup struct {
	Agents []string
	Rules  []RobotsRule
}

// Robots is the parsed robots.txt for one host.
type Robots struct {
	Groups []RobotsGroup
	Blank  bool
}

// RobotsCache fetches and caches robots.txt per scheme+host.
type RobotsCache struct {
	mu       sync.Mutex
	cache    map[string]*Robots
	inflight map[string]*sync.WaitGroup
	client   *Client
}

func NewRobotsCache() *RobotsCache {
	return &RobotsCache{cache: map[string]*Robots{}, inflight: map[string]*sync.WaitGroup{}}
}

// SetClient wires the cache to the client that fetches robots.txt. Avoids the
// constructor cycle.
func (r *RobotsCache) SetClient(c *Client) { r.client = c }

// Allowed reports whether the client user agent may fetch absURL per robots.
// A missing or unparseable robots.txt allows everything. Errors fetching
// robots.txt allow everything (fail open) so a network blip does not silence a
// source.
func (r *RobotsCache) Allowed(ctx context.Context, absURL string) bool {
	u, err := url.Parse(absURL)
	if err != nil {
		return true
	}
	key := u.Scheme + "://" + u.Host
	robots, err := r.forHost(ctx, key)
	if err != nil || robots == nil || robots.Blank {
		return true
	}
	agent := ""
	if r.client != nil {
		agent = r.client.userAgent
	}
	return robots.Allowed(agent, u.Path)
}

func (r *RobotsCache) forHost(ctx context.Context, base string) (*Robots, error) {
	r.mu.Lock()
	if rb, ok := r.cache[base]; ok {
		r.mu.Unlock()
		return rb, nil
	}
	if wg, ok := r.inflight[base]; ok {
		r.mu.Unlock()
		wg.Wait()
		r.mu.Lock()
		rb := r.cache[base]
		r.mu.Unlock()
		return rb, nil
	}
	wg := &sync.WaitGroup{}
	wg.Add(1)
	r.inflight[base] = wg
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		delete(r.inflight, base)
		r.mu.Unlock()
		wg.Done()
	}()

	client := r.client
	if client == nil {
		rb := &Robots{Blank: true}
		r.mu.Lock()
		r.cache[base] = rb
		r.mu.Unlock()
		return rb, nil
	}
	resp, err := client.Get(ctx, base+"/robots.txt")
	rb := &Robots{Blank: true}
	if err == nil {
		rb = ParseRobots(string(resp.Body))
	} else if se := statusOf(err); se == 404 {
		rb = &Robots{Blank: true}
	}
	r.mu.Lock()
	r.cache[base] = rb
	r.mu.Unlock()
	return rb, nil
}

// ParseRobots parses robots.txt content.
func ParseRobots(text string) *Robots {
	rb := &Robots{Blank: true}
	var cur *RobotsGroup
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		line := strings.TrimSpace(strings.TrimSuffix(sc.Text(), "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		val = strings.TrimSpace(val)
		switch key {
		case "user-agent":
			if cur == nil || len(cur.Rules) > 0 {
				rb.Groups = append(rb.Groups, RobotsGroup{})
				cur = &rb.Groups[len(rb.Groups)-1]
			}
			cur.Agents = append(cur.Agents, strings.ToLower(val))
			rb.Blank = false
		case "allow", "disallow":
			if cur == nil {
				continue
			}
			if val == "" {
				continue
			}
			cur.Rules = append(cur.Rules, RobotsRule{Allow: key == "allow", Pattern: val})
		}
	}
	return rb
}

// Allowed applies the most specific matching rule for the user agent. Longest
// match wins; Allow wins ties.
func (r *Robots) Allowed(userAgent, path string) bool {
	ua := strings.ToLower(userAgent)
	var groups []RobotsGroup
	for _, g := range r.Groups {
		for _, a := range g.Agents {
			if a == "*" || (ua != "" && strings.Contains(ua, a)) {
				groups = append(groups, g)
				break
			}
		}
	}
	if len(groups) == 0 {
		return true
	}
	best := -1
	allow := true
	for _, g := range groups {
		for _, rule := range g.Rules {
			n := robotsMatch(rule.Pattern, path)
			if n < 0 {
				continue
			}
			if n > best || (n == best && rule.Allow) {
				best = n
				allow = rule.Allow
			}
		}
	}
	if best < 0 {
		return true
	}
	return allow
}

// robotsMatch returns the pattern length when it matches, else -1.
func robotsMatch(pattern, path string) int {
	anchor := !strings.HasSuffix(pattern, "$")
	pattern = strings.TrimSuffix(pattern, "$")
	if pattern == "" {
		return -1
	}
	if anchor {
		if strings.HasPrefix(path, pattern) {
			return len(pattern)
		}
		return -1
	}
	if strings.HasPrefix(path, pattern) || path == pattern {
		return len(pattern)
	}
	return -1
}
