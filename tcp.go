package manet

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	ma "github.com/jbenet/go-multiaddr"
)

type TCP struct{}

func (t TCP) Match(m ma.Multiaddr, side int) (int, bool) {
	ps := m.Protocols()

	if len(ps) < 1 {
		return 0, false
	}

	if ps[0].Name == "tcp" {
		return 1, true
	}

	return 0, false
}

func (t TCP) Materialize(m ma.Multiaddr, side int) ContextMutator {
	p := m.Protocols()[0]
	port, _ := m.ValueForProtocol(p.Code)

	switch side {

	case S_Client:

		return func(ctx Context) error {
			mctx := ctx.Misc()
			sctx := ctx.Special()
			netcon, err := t.Dial(mctx.IP, port)
			if netcon != nil {
				sctx.NetConn = netcon
			}
			return err
		}

	case S_Server:

		return func(ctx Context) error {
			mctx := ctx.Misc()
			sctx := ctx.Special()
			netln, err := t.Listen(mctx.IP, port)
			if netln != nil {
				sctx.NetListener = netln
			}
			return err
		}

	}
	return nil
}

func (t TCP) Dial(ip net.IP, portstr string) (*net.TCPConn, error) {
	port, err := strconv.Atoi(portstr)
	if err != nil {
		return nil, err
	}

	addr := &net.TCPAddr{IP: ip, Port: port}

	con, err := net.DialTCP("tcp", nil, addr)
	if err != nil {
		return nil, err
	}

	return con, nil
}

func (t TCP) Listen(ip net.IP, portstr string) (*net.TCPListener, error) {
	port, err := strconv.Atoi(portstr)
	if err != nil {
		return nil, err
	}

	addr := &net.TCPAddr{IP: ip, Port: port}

	ln, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return nil, err
	}

	return ln, nil
}

func ResolveTCPAddr(m ma.Multiaddr) (*net.TCPAddr, error) {
	nnet, naddr, err := DialArgs(m)
	if err != nil {
		return nil, err
	}

	return net.ResolveTCPAddr(nnet, naddr)
}

func DialArgs(m ma.Multiaddr) (string, string, error) {
	parts := strings.Split(m.String(), "/")[1:]

	switch parts[0] {
	case "ip4":
		return "tcp4", fmt.Sprintf("%s:%s", parts[1], parts[3]), nil
	case "ip6":
		return "tcp6", fmt.Sprintf("[%s]:%s", parts[1], parts[3]), nil
	default:
		return "", "", fmt.Errorf("unsupported protocol %s under tcp", parts[0])
	}
}

// FromTCPAddr converts a *net.TCPAddr type to a Multiaddr.
func FromTCPAddr(addr *net.TCPAddr) (ma.Multiaddr, error) {
	ipm, err := FromIP(addr.IP)
	if err != nil {
		return nil, err
	}

	tcpm, err := ma.NewMultiaddr(fmt.Sprintf("/tcp/%d", addr.Port))
	if err != nil {
		return nil, err
	}

	return ipm.Encapsulate(tcpm), nil
}
