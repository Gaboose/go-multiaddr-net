package manet

import (
	"fmt"
	"net"
	"reflect"

	ma "github.com/jbenet/go-multiaddr"
)

type Matcher interface {
	Match(m ma.Multiaddr, side int) (n int, ok bool)
}

type MatchApplier interface {
	Matcher
	Apply(m ma.Multiaddr, side int, ctx Context) error
}

// Passed as an arg named side to Matchers.
// Specifies whether we're dialing or listening.
const (
	S_Client = iota
	S_Server
)

type Context map[string]interface{}

type reusableContext struct {
	Matcher
	Context
	underClose func() error
	usecount   int
}

func (rc *reusableContext) Apply(m ma.Multiaddr, side int, ctx Context) error {
	rc.Context.CopyTo(ctx)
	ctx.Special().CloseFn = rc.Close
	rc.usecount++
	return nil
}

func (rc *reusableContext) Close() error {
	matchers.Lock()
	defer matchers.Unlock()

	rc.usecount--
	if rc.usecount == 0 {

		// remove rc from matchers.reusable
		mr := matchers.reusable
		for i, mch := range mr {
			if mch == rc {
				mr[i] = mr[len(mr)-1] // override with the last element
				mr[len(mr)-1] = nil   // remove duplicate ref
				mr = mr[:len(mr)-1]   // decrease length by one
				break
			}
		}
		matchers.reusable = mr

		return rc.underClose()
	}
	return nil
}

// A MatchApplier can offer its current context to be reused by another
// Listen() call later by invoking this function.
func (ctx Context) Reuse(mch Matcher) {
	// a snapshot of current context to be reused
	ctxcopy := Context{}
	ctx.CopyTo(ctxcopy)

	// replace sctx.CloseFn with one that manages usecount - the number of
	// Listener instances rc serves
	sctx := ctx.Special()
	rc := &reusableContext{mch, ctxcopy, sctx.CloseFn, 1}
	sctx.CloseFn = rc.Close

	matchers.reusable = append(matchers.reusable, rc)
}

func (ctx Context) CopyTo(target Context) {
	for k, val := range ctx {

		// "misc" and "special" keys will hold pointers to structs, so instead
		// of copying the pointer, we must reflect and copy what it points to.
		rval := reflect.ValueOf(val)
		if rval.Kind() == reflect.Ptr {

			rv := reflect.New(rval.Type().Elem())
			rv.Elem().Set(rval.Elem())
			target[k] = rv.Interface()

		} else {
			target[k] = val
		}
	}
}

// MiscContext holds things produced by some MatchAppliers and required by others
type MiscContext struct {
	IPs     []net.IP
	Host    string
	HTTPMux *ServeMux
}

// SpecialContext holds values that are used or written outside
// MatchApplier objects in between and after Apply() method calls.
type SpecialContext struct {

	// Dial() embedds NetConn into its returned Conn
	NetConn net.Conn

	// Listen() embedds NetListener into its returned Listener
	NetListener net.Listener

	// chain[i] MatchApplier will find PreAddr to hold the first part of the full
	// Multiaddr, which has already been executed by the chain[:i] MatchAppliers
	//
	// E.g. if the full Multiaddr is /ip4/127.0.0.1/tcp/80/http/ws,
	// during "http" Apply() execution PreAddr will be /ip4/127.0.0.1/tcp/80
	PreAddr ma.Multiaddr

	// CloseFn overrides the Close function of embedded NetConn and NetListener.
	//
	// If a MatchApplier needs to override this function, it should take the
	// responsibility of calling the one written by a previous MatchApplier
	// in chain.
	CloseFn func() error
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

func ConcatClose(f1, f2 func() error) func() error {
	return func() error {
		err := f1()
		err = f2()
		return err
	}
}

// buildChain returns two parralel slices. One holds a sequence of Matchers,
// which is capable to handle the given Multiaddr with the side constant.
// The second: full Multiaddr split into one or more protocols a piece, which
// is what each Matcher.Apply expects as its m argument.
//
// Allowed values for side are S_Server and S_Client.
func (mr matchreg) buildChain(m ma.Multiaddr, side int) ([]MatchApplier, []ma.Multiaddr, error) {
	tail := m
	chain := []MatchApplier{}
	split := []ma.Multiaddr{}

	for tail.String() != "" {
		mch, n, err := mr.matchPrefix(tail, side)
		if err != nil {
			return nil, nil, err
		}

		spl := ma.Split(tail)
		head := ma.Join(spl[:n]...)
		tail = ma.Join(spl[n:]...)

		split = append(split, head)
		chain = append(chain, mch)
	}

	return chain, split, nil
}

// matchPrefix finds a MatchApplier (in mr.protocols or mr.reusable) for one or
// more first m.Protocols() and also returns an int of how many protocols it can
// consume.
//
// matchPrefix returns an error if it can't find any or finds more
// than one Matcher.
func (mr matchreg) matchPrefix(m ma.Multiaddr, side int) (MatchApplier, int, error) {
	ret := []MatchApplier{}

	for _, mch := range mr.reusable {
		if _, ok := mch.Match(m, side); ok {
			ret = append(ret, mch)
		}
	}

	if len(ret) == 1 {
		n, _ := ret[0].Match(m, side)
		return ret[0], n, nil
	} else if len(ret) > 1 {
		return nil, 0, fmt.Errorf("found more than one reusable for %s", m.String())
	}

	for _, mch := range mr.protocols {
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
