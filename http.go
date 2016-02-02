package manet

import (
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"

	ma "github.com/jbenet/go-multiaddr"
)

type HTTP struct{}

func (p HTTP) Match(m ma.Multiaddr, side int) (int, bool) {
	if side != S_Server {
		return 0, false
	}

	ms := m.Protocols()
	if len(ms) >= 1 && ms[0].Name == "http" {
		return 1, true
	}

	return 0, false
}

func (p HTTP) Apply(m ma.Multiaddr, side int, ctx Context) error {
	mctx := ctx.Misc()
	sctx := ctx.Special()

	mctx.HTTPMux = p.Server(sctx.NetListener)

	r := &httpreuser{ctx.Special().PreAddr}
	ctx.Reuse(r)
	return nil
}

func (p HTTP) Server(ln net.Listener) *ServeMux {
	mux := NewServeMux()
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	return mux
}

type httpreuser struct {
	prefix ma.Multiaddr
}

func (h httpreuser) Match(m ma.Multiaddr, side int) (int, bool) {
	if side != S_Server {
		return 0, false
	}

	// check if h.prefix is a prefix of m
	ms := ma.Split(m)
	ps := ma.Split(h.prefix)

	if len(ms) < len(ps) {
		return 0, false
	}

	for i, p := range ps {
		if !p.Equal(ms[i]) {
			return 0, false
		}
	}

	// match an additional http protocol if it's there
	if len(ms) > len(ps) && ms[len(ps)].String() == "/http" {
		return len(ps) + 1, true
	}

	return len(ps), true
}

// ServeMux is copied from the standard "net/http" package at this commit:
// https://github.com/golang/go/blob/2c12b81739ec2cb85073e125748fcbf5d2febb2c/src/net/http/server.go
//
// With the addition of the DeHandle method.
type ServeMux struct {
	mu    sync.RWMutex
	m     map[string]muxEntry
	hosts bool // whether any patterns contain hostnames
}

type muxEntry struct {
	explicit bool
	h        http.Handler
	pattern  string
}

// NewServeMux allocates and returns a new ServeMux.
func NewServeMux() *ServeMux { return &ServeMux{m: make(map[string]muxEntry)} }

// Does path match pattern?
func pathMatch(pattern, path string) bool {
	if len(pattern) == 0 {
		// should not happen
		return false
	}
	n := len(pattern)
	if pattern[n-1] != '/' {
		return pattern == path
	}
	return len(path) >= n && path[0:n] == pattern
}

// Return the canonical path for p, eliminating . and .. elements.
func cleanPath(p string) string {
	if p == "" {
		return "/"
	}
	if p[0] != '/' {
		p = "/" + p
	}
	np := path.Clean(p)
	// path.Clean removes trailing slash except for root;
	// put the trailing slash back if necessary.
	if p[len(p)-1] == '/' && np != "/" {
		np += "/"
	}
	return np
}

// Find a handler on a handler map given a path string
// Most-specific (longest) pattern wins
func (mux *ServeMux) match(path string) (h http.Handler, pattern string) {
	var n = 0
	for k, v := range mux.m {
		if !pathMatch(k, path) {
			continue
		}
		if h == nil || len(k) > n {
			n = len(k)
			h = v.h
			pattern = v.pattern
		}
	}
	return
}

// Handler returns the handler to use for the given request,
// consulting r.Method, r.Host, and r.URL.Path. It always returns
// a non-nil handler. If the path is not in its canonical form, the
// handler will be an internally-generated handler that redirects
// to the canonical path.
//
// Handler also returns the registered pattern that matches the
// request or, in the case of internally-generated redirects,
// the pattern that will match after following the redirect.
//
// If there is no registered handler that applies to the request,
// Handler returns a ``page not found'' handler and an empty pattern.
func (mux *ServeMux) Handler(r *http.Request) (h http.Handler, pattern string) {
	if r.Method != "CONNECT" {
		if p := cleanPath(r.URL.Path); p != r.URL.Path {
			_, pattern = mux.handler(r.Host, p)
			url := *r.URL
			url.Path = p
			return http.RedirectHandler(url.String(), http.StatusMovedPermanently), pattern
		}
	}

	return mux.handler(r.Host, r.URL.Path)
}

// handler is the main implementation of Handler.
// The path is known to be in canonical form, except for CONNECT methods.
func (mux *ServeMux) handler(host, path string) (h http.Handler, pattern string) {
	mux.mu.RLock()
	defer mux.mu.RUnlock()

	// Host-specific pattern takes precedence over generic ones
	if mux.hosts {
		h, pattern = mux.match(host + path)
	}
	if h == nil {
		h, pattern = mux.match(path)
	}
	if h == nil {
		h, pattern = http.NotFoundHandler(), ""
	}
	return
}

// ServeHTTP dispatches the request to the handler whose
// pattern most closely matches the request URL.
func (mux *ServeMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.RequestURI == "*" {
		if r.ProtoAtLeast(1, 1) {
			w.Header().Set("Connection", "close")
		}
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	h, _ := mux.Handler(r)
	h.ServeHTTP(w, r)
}

// Handle registers the handler for the given pattern.
// If a handler already exists for pattern, Handle panics.
func (mux *ServeMux) Handle(pattern string, handler http.Handler) {
	mux.mu.Lock()
	defer mux.mu.Unlock()

	if pattern == "" {
		panic("http: invalid pattern " + pattern)
	}
	if handler == nil {
		panic("http: nil handler")
	}
	if mux.m[pattern].explicit {
		panic("http: multiple registrations for " + pattern)
	}

	mux.m[pattern] = muxEntry{explicit: true, h: handler, pattern: pattern}

	if pattern[0] != '/' {
		mux.hosts = true
	}

	// Helpful behavior:
	// If pattern is /tree/, insert an implicit permanent redirect for /tree.
	// It can be overridden by an explicit registration.
	n := len(pattern)
	if n > 0 && pattern[n-1] == '/' && !mux.m[pattern[0:n-1]].explicit {
		// If pattern contains a host name, strip it and use remaining
		// path for redirect.
		path := pattern
		if pattern[0] != '/' {
			// In pattern, at least the last character is a '/', so
			// strings.Index can't be -1.
			path = pattern[strings.Index(pattern, "/"):]
		}
		url := &url.URL{Path: path}
		mux.m[pattern[0:n-1]] = muxEntry{h: http.RedirectHandler(url.String(), http.StatusMovedPermanently), pattern: pattern}
	}
}

// HandleFunc registers the handler function for the given pattern.
func (mux *ServeMux) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	mux.Handle(pattern, http.HandlerFunc(handler))
}

// DeHandle unregisters the handler for the given pattern
func (mux *ServeMux) DeHandle(pattern string) {
	mux.mu.Lock()
	defer mux.mu.Unlock()

	if pattern == "" {
		panic("http: invalid pattern " + pattern)
	}

	delete(mux.m, pattern)

	n := len(pattern)
	if pattern[n-1] == '/' && !mux.m[pattern[:n-1]].explicit {
		delete(mux.m, pattern[:n-1])
	}
}
