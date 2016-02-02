package manet

import (
	"fmt"
	"net"
	"strings"
	"sync"

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

	matchers.Lock()
	defer matchers.Unlock()

	chain, split, err := matchers.buildChain(remote, S_Client)
	if err != nil {
		return nil, err
	}

	ctx := Context{}
	sctx := ctx.Special()

	// apply context mutators
	for i, mch := range chain {

		err := mch.Apply(split[i], S_Client, ctx)
		if err != nil {
			if sctx.CloseFn != nil {
				sctx.CloseFn()
			}
			return nil, err
		}

		if sctx.PreAddr == nil {
			sctx.PreAddr = split[i]
		} else {
			sctx.PreAddr = sctx.PreAddr.Encapsulate(split[i])
		}

	}

	if sctx.NetConn == nil {
		if sctx.CloseFn != nil {
			sctx.CloseFn()
		}
		return nil, fmt.Errorf("insufficient address for a connection: %s", remote)
	}

	return &conn{
		Conn:    sctx.NetConn,
		raddr:   remote,
		closeFn: sctx.CloseFn,
	}, nil
}

// matchreg is a Matcher registry used by Dial and Listen functions
type matchreg struct {
	sync.Mutex

	// standard Matchers for both dialing and listening
	protocols []MatchApplier

	// running listeners available for reuse (e.g. with a ServeMux)
	reusable []MatchApplier
}

var matchers = &matchreg{
	protocols: []MatchApplier{
		IP{},
		DNS{},
		TCP{},
		HTTP{},
		WS{},
	},
}

// Listen announces on the local network address local.
func Listen(local ma.Multiaddr) (Listener, error) {
	matchers.Lock()
	defer matchers.Unlock()

	// resolve a chain of applicable MatchAppliers
	chain, split, err := matchers.buildChain(local, S_Server)
	if err != nil {
		return nil, err
	}

	ctx := Context{}
	sctx := ctx.Special()

	// apply chain to empty context
	for i, mch := range chain {

		err := mch.Apply(split[i], S_Server, ctx)

		if err != nil {
			// we reset sctx to ctx.Special() again, since it may have been overriden
			// by the reusableContext using the ctx1.CopyTo(ctx2) method
			sctx := ctx.Special()

			if sctx.CloseFn != nil {
				go sctx.CloseFn()
			}
			return nil, err
		}

		if sctx.PreAddr == nil {
			sctx.PreAddr = split[i]
		} else {
			sctx.PreAddr = sctx.PreAddr.Encapsulate(split[i])
		}

	}

	// we reset sctx to ctx.Special() again, since it may have been overriden
	// by the reusableContext using the ctx1.CopyTo(ctx2) method
	sctx = ctx.Special()

	if sctx.NetListener == nil {
		if sctx.CloseFn != nil {
			go sctx.CloseFn()
		}
		return nil, fmt.Errorf("insufficient address for a listener: %s", local)
	}

	ln := &listener{
		Listener: sctx.NetListener,
		maddr:    local,
		closeFn:  sctx.CloseFn,
	}

	return ln, nil
}

type listener struct {
	net.Listener
	maddr   ma.Multiaddr
	closeFn func() error
}

func (l listener) Accept() (Conn, error) {
	netcon, err := l.Listener.Accept()
	if err != nil {
		return nil, fmt.Errorf("listener %s is closed", l.maddr.String())
	}

	return &conn{
		Conn:    netcon,
		laddr:   l.maddr,
		closeFn: netcon.Close,
	}, nil
}

func (l listener) Close() error {
	return l.closeFn()
}

func (l listener) Multiaddr() ma.Multiaddr { return l.maddr }

type conn struct {
	net.Conn
	laddr   ma.Multiaddr
	raddr   ma.Multiaddr
	closeFn func() error
}

func (c conn) Close() error {
	return c.closeFn()
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
	if r := recover(); r != nil {
		if err != nil {
			*maybeErr = err
		} else {
			*maybeErr = fmt.Errorf("%s", r)
		}
	}
}
