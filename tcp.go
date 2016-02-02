package manet

import (
	"fmt"
	"net"
	"strconv"

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

func (t TCP) Apply(m ma.Multiaddr, side int, ctx Context) error {
	p := m.Protocols()[0]
	port, _ := m.ValueForProtocol(p.Code)

	mctx := ctx.Misc()
	sctx := ctx.Special()

	switch side {

	case S_Client:
		netcon, err := t.Dial(mctx.IP, port)
		if err != nil {
			return err
		}
		sctx.NetConn = netcon
		sctx.CloseFn = netcon.Close
		return nil

	case S_Server:
		netln, err := t.Listen(mctx.IP, port)
		if err != nil {
			return err
		}
		sctx.NetListener = netln
		sctx.CloseFn = netln.Close
		return nil

	}

	return fmt.Errorf("incorrect side constant")
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
