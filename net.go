package manet

import (
	"fmt"
	"net"
	"strings"

	ma "github.com/jbenet/go-multiaddr"
)

// Conn is the equivalent of a net.Conn object. It is the
// result of calling the Dial or Listen functions in this
// package, with associated local and remote Multiaddrs.
type Conn interface {
	net.Conn

	// LocalMultiaddr returns the local Multiaddr associated
	// with this connection
	LocalMultiaddr() ma.Multiaddr

	// RemoteMultiaddr returns the remote Multiaddr associated
	// with this connection
	RemoteMultiaddr() ma.Multiaddr
}

// A Listener is a generic network listener for stream-oriented protocols.
// it's similar to net.Listener, except it provides its Multiaddr and
// the Listener.Accept is changed to return this package's Conn.
type Listener interface {

	// Accept waits for and returns the next connection to the listener.
	// Returns a Multiaddr friendly Conn
	Accept() (Conn, error)

	// Close closes the listener.
	// Any blocked Accept operations will be unblocked and return errors.
	Close() error

	// Multiaddr returns the listener's (local) Multiaddr.
	Multiaddr() ma.Multiaddr

	// Addr returns the net.Listener's network address.
	Addr() net.Addr
}

// Dial connects to a remote address
func Dial(remote ma.Multiaddr) (Conn, error) {

	chain, _, err := buildChain(remote, S_Client)
	if err != nil {
		return nil, err
	}

	ctx := Context{}
	sctx := ctx.Special()

	// apply context mutators
	for _, cm := range chain {
		err := cm(ctx)
		if err != nil {
			if con := ctx.Special().NetConn; con != nil {
				con.Close()
			}
			return nil, err
		}
	}

	return &conn{
		Conn:  sctx.NetConn,
		raddr: remote,
	}, nil
}

// relisten holds contexts of reusable listeners
var relisten = map[ma.Multiaddr]Context{}

// Listen announces on the local network address local.
func Listen(local ma.Multiaddr) (Listener, error) {

	var chain []ContextMutator
	var split []ma.Multiaddr
	var ctx Context
	var err error

	// see if we can reuse a listener,
	// e.g. an http server for another websocket
	for m, c := range relisten {

		if tail, ok := trimPrefix(local, m); ok {
			// got lucky, build only the differing part, reuse context
			chain, split, err = buildChain(tail, S_Client)
			if err != nil {
				return nil, err
			}
			ctx = c
			break
		}
	}

	// nothing to reuse, build the whole chain, create new context
	if chain == nil {
		chain, split, err = buildChain(local, S_Client)
		if err != nil {
			if ln := ctx.Special().NetListener; ln != nil {
				ln.Close()
			}
			return nil, err
		}
		ctx = Context{}
	}

	sctx := ctx.Special()

	// apply context mutators
	for i, cm := range chain {
		err := cm(ctx)
		if err != nil {
			if ln := ctx.Special().NetListener; ln != nil {
				ln.Close()
			}
			return nil, err
		}

		// a mutator can offer the context up to itself
		// to be reused by another listener later
		if sctx.Reuse {
			relisten[ma.Join(split[:i+1]...)] = ctx
			sctx.Reuse = false
		}
	}

	ln := &listener{
		Listener: sctx.NetListener,
		maddr:    local,
	}

	return ln, nil
}

type listener struct {
	net.Listener
	maddr ma.Multiaddr
}

func (l listener) Accept() (Conn, error) {
	netcon, err := l.Listener.Accept()
	if err != nil {
		return nil, fmt.Errorf("listener %s is closed", l.maddr.String())
	}

	return &conn{
		Conn:  netcon,
		laddr: l.maddr,
	}, nil
}

func (l listener) Multiaddr() ma.Multiaddr { return l.maddr }

type conn struct {
	net.Conn
	laddr ma.Multiaddr
	raddr ma.Multiaddr
}

func (c conn) LocalMultiaddr() ma.Multiaddr {
	if c.laddr != nil {
		return c.laddr
	}
	m, _ := FromNetAddr(c.LocalAddr())
	return m
}
func (c conn) RemoteMultiaddr() ma.Multiaddr {
	if c.raddr != nil {
		return c.raddr
	}
	m, _ := FromNetAddr(c.RemoteAddr())
	return m
}

func trimPrefix(m, prem ma.Multiaddr) (ma.Multiaddr, bool) {
	s := m.String()
	pres := prem.String()

	if !strings.HasPrefix(s, pres) {
		return nil, false
	}

	return ma.StringCast(strings.TrimPrefix(s, pres)), true
}

var errIncorrectNetAddr = fmt.Errorf("incorrect network addr conversion")

// FromNetAddr converts a net.Addr type to a Multiaddr.
func FromNetAddr(naddr net.Addr) (ma.Multiaddr, error) {
	if tcpaddr, ok := naddr.(*net.TCPAddr); ok {
		return FromTCPAddr(tcpaddr)
	} else {
		return nil, fmt.Errorf("unknown net.Addr")
	}
}

func recoverToError(maybeErr *error, err error) {
	if recover() != nil {
		*maybeErr = err
	}
}
