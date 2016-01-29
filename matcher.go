package manet

import (
	"fmt"
	"net"
	"net/http"

	ma "github.com/jbenet/go-multiaddr"
)

type Matcher interface {
	Match(ma.Multiaddr, int) (int, bool)
	Materialize(ma.Multiaddr, int) ContextMutator
}

var Matchers = []Matcher{
	IP{},
	TCP{},
	HTTP{},
	WS{},
}

// Passed as an arg named side to Matchers.
// Specifies whether we're dialing or listening.
const (
	S_Client = iota
	S_Server
)

type Context map[string]interface{}

type ContextMutator func(Context) error

// MiscContext holds things produced by some Matchers and requierd by others
type MiscContext struct {
	IP      net.IP
	HTTPMux *http.ServeMux
}

// SpecialContext holds values that are read and used outside Matcher objects
// during and after ContextMutators are applied.
type SpecialContext struct {

	// Embedded into Conn that's returned by Dial()
	NetConn net.Conn

	// Embedded into Listener that's returned by Listen()
	NetListener net.Listener

	// Can be set to true by Matcher to signal that this point in Listener
	// creation can be reused by later Listen() calls.
	Reuse bool
}

func (ctx Context) Misc() *MiscContext {
	if mctx, ok := ctx["misc"]; ok {
		return mctx.(*MiscContext)
	} else {
		mctx := &MiscContext{}
		ctx["misc"] = mctx
		return mctx
	}
}

func (ctx Context) Special() *SpecialContext {
	if sctx, ok := ctx["special"]; ok {
		return sctx.(*SpecialContext)
	} else {
		sctx := &SpecialContext{}
		ctx["special"] = sctx
		return sctx
	}
}

// buildChain returns a sequence of ContextMutators, which is capable to handle
// the given multiaddress and side constant.
//
// Allowed values for side are S_Server and S_Client.
func buildChain(m ma.Multiaddr, side int) ([]ContextMutator, []ma.Multiaddr, error) {
	tail := m
	chain := []ContextMutator{}
	split := []ma.Multiaddr{}

	for tail.String() != "" {
		mch, n, err := matchPrefix(tail, side)
		if err != nil {
			return nil, nil, err
		}

		spl := ma.Split(tail)
		head := ma.Join(spl[:n]...)
		tail = ma.Join(spl[n:]...)

		split = append(split, head)
		chain = append(chain, mch.Materialize(head, side))
	}

	return chain, split, nil
}

// matchPrefix finds a Matcher for one or more first m.Protocols() and
// also returns an int of how many protocols it can handle.
//
// matchPrefix returns an error if it can't find any or finds more
// than one Matcher.
func matchPrefix(m ma.Multiaddr, side int) (Matcher, int, error) {
	ret := []Matcher{}

	for _, mch := range Matchers {
		if _, ok := mch.Match(m, side); ok {
			ret = append(ret, mch)
		}
	}

	if len(ret) == 0 {
		return nil, 0, fmt.Errorf("no matchers found for %s", m.String())
	} else if len(ret) > 1 {
		return nil, 0, fmt.Errorf("found more than one matcher for %s", m.String())
	}

	n, _ := ret[0].Match(m, side)
	return ret[0], n, nil
}
