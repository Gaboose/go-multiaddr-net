package main

import (
	"flag"
	"fmt"
	manet "github.com/Gaboose/go-multiaddr-net"
	ma "github.com/jbenet/go-multiaddr"
	"io"
	lg "log"
	"os"
)

var log = lg.New(os.Stdout, "", lg.Lshortfile)

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [-l] <multiaddr>\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr, "Examples:")
		fmt.Fprintf(os.Stderr, "	%s -l /ip4/0.0.0.0/tcp/4324\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "	%s /dns/localhost/tcp/4324\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "	%s /dns/echo-gaboose.rhcloud.com/tcp/8000/ws/echo\n", os.Args[0])
	}
}

func main() {
	listen := flag.Bool("l", false, "listen mode, for inbound connections")
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		flag.Usage()
		os.Exit(1)
	}

	m, err := ma.NewMultiaddr(args[0])
	if err != nil {
		log.Fatal(err)
	}

	var c manet.Conn
	if *listen {
		ln, err := manet.Listen(m)
		if err != nil {
			log.Fatal(err)
		}

		c, err = ln.Accept()
		if err != nil {
			log.Fatal(err)
		}
	} else {
		c, err = manet.Dial(m)
		if err != nil {
			log.Fatal(err)
		}
	}

	go io.Copy(c, os.Stdin)
	io.Copy(os.Stdout, c)
}
