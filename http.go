package manet

import (
	"net"
	"net/http"

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

func (p HTTP) Materialize(m ma.Multiaddr, side int) ContextMutator {
	return func(ctx Context) error {
		mctx := ctx.Misc()
		sctx := ctx.Special()
		mctx.HTTPMux = p.Server(sctx.NetListener)
		sctx.Reuse = true
		return nil
	}
}

func (p HTTP) Server(ln net.Listener) *http.ServeMux {
	mux := http.NewServeMux()
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	return mux
}
