package match

import (
	"net"

	ma "github.com/jbenet/go-multiaddr"
)

type Matcher interface {
	Match(m ma.Multiaddr, side int) (n int, ok bool)
}

type MatchApplier interface {
	Matcher
	Apply(m ma.Multiaddr, side int, ctx Context) error
}

// Passed as an arg named "side" to MatchAppliers.
// Specifies whether we're dialing or listening.
const (
	S_Client = iota
	S_Server
)

type Context interface {
	Map() map[string]interface{}
	Misc() *MiscContext
	Special() *SpecialContext
	CopyTo(Context)

	// A MatchApplier can offer its current context to be reused by another
	// Listen() call later by invoking this function.
	// E.g. /http does this to share a single ServeMux with several /ws listeners
	//
	// Specifically, Reuse copies and stores ctx, then applies it to
	// multiaddresses that are matched by the given mch.
	Reuse(mch Matcher)
}

// MiscContext holds things produced by some MatchAppliers and required by others
type MiscContext struct {
	IPs     []net.IP
	Host    string
	HTTPMux *ServeMux
}

// SpecialContext holds values that are used or written outside MatchApplier
// objects by the library in between and after Apply() method calls.
type SpecialContext struct {

	// Dial() embedds NetConn into its returned Conn
	NetConn net.Conn

	// Listen() embedds NetListener into its returned Listener
	NetListener net.Listener

	// chain[i] MatchApplier will find PreAddr to hold the left part of the full
	// Multiaddr, which has already been executed by the chain[:i] MatchAppliers
	//
	// E.g. if the full Multiaddr is /ip4/127.0.0.1/tcp/80/http/ws,
	// during "http" Apply() execution PreAddr will be /ip4/127.0.0.1/tcp/80
	PreAddr ma.Multiaddr

	// CloseFn overrides the Close function of embedded NetConn and NetListener.
	//
	// If a MatchApplier needs to override this function, it should take the
	// responsibility of calling the one written by a previous MatchApplier
	// in the chain.
	CloseFn func() error
}
