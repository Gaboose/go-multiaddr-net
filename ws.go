package manet

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"

	"golang.org/x/net/websocket"

	ma "github.com/jbenet/go-multiaddr"
)

var wsnilconf, _ = websocket.NewConfig("/", "/")

type WS struct{}

func (p WS) Match(m ma.Multiaddr, side int) (int, bool) {
	ps := m.Protocols()

	if len(ps) >= 1 && ps[0].Name == "ws" {
		return 1, true
	}

	// If we're a client, also match "/http/ws".
	if side == S_Client && len(ps) >= 2 &&
		ps[0].Name == "http" && ps[1].Name == "ws" {

		return 2, true
	}

	return 0, false
}

func (p WS) Materialize(m ma.Multiaddr, side int) ContextMutator {
	switch side {

	case S_Client:

		return func(ctx Context) error {
			sctx := ctx.Special()
			wcon, err := p.Select(sctx.NetConn)
			if wcon != nil {
				sctx.NetConn = wcon
			}
			return err
		}

	case S_Server:

		return func(ctx Context) error {
			sctx := ctx.Special()
			mctx := ctx.Misc()
			ln := p.Handle(mctx.HTTPMux, "/")
			sctx.NetListener = ln
			return nil
		}

	}

	return nil
}

func (p WS) Select(netcon net.Conn) (*websocket.Conn, error) {
	wcon, err := websocket.NewClient(wsnilconf, netcon)
	if err != nil {
		return nil, err
	}

	return wcon, nil
}

func (p WS) Handle(mux *http.ServeMux, pattern string) net.Listener {
	ln := &wslistener{
		make(chan net.Conn),
		make(chan struct{}),
	}

	mux.Handle(pattern, ln)

	return ln
}

type wslistener struct {
	acceptCh chan net.Conn
	closeCh  chan struct{}
}

func (ln wslistener) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	websocket.Handler(func(wcon *websocket.Conn) {
		// It appears we mustn't pass wcon to external users as is.
		// We'll pass a pipe instead, because the only way to know if a wcon
		// was closed remotely is to read from it until EOF.
		ch := make(chan struct{})
		p1, p2 := net.Pipe()

		go func() {
			io.Copy(wcon, p1)
			wcon.Close()
		}()
		go func() {
			io.Copy(p1, wcon)
			p1.Close()

			close(ch)
		}()

		select {
		case ln.acceptCh <- p2:
		case <-ln.closeCh:
		}

		// As soon as we return from this function, websocket library will
		// close wcon. So we'll wait until p2 or wcon is closed.
		<-ch

	}).ServeHTTP(w, r)
}

func (ln wslistener) Accept() (net.Conn, error) {
	select {
	case c := <-ln.acceptCh:
		return c, nil
	case <-ln.closeCh:
		return nil, errors.New("listener is closed")
	}
}

func (ln wslistener) Close() error {
	var err error
	func() {
		defer recoverToError(
			&err,
			fmt.Errorf("listener is already closed"),
		)
		close(ln.closeCh)
	}()
	return err
}

func (ln wslistener) Addr() net.Addr { return nil }
