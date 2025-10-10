package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"tailscale.com/tsnet"
)

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

type mapping struct {
	listen string
	dest   string
}

func parseMapping(s string) (mapping, error) {
	// Format: listen=dest e.g. 127.0.0.1:50010=10.0.0.10:50000
	parts := strings.SplitN(s, "=", 2)
	if len(parts) != 2 {
		return mapping{}, fmt.Errorf("invalid mapping %q, expected listen=dest", s)
	}
	return mapping{listen: strings.TrimSpace(parts[0]), dest: strings.TrimSpace(parts[1])}, nil
}

func main() {
	var maps multiFlag
	var loginServer, authKey, hostname string
	var verbose bool

	defLogin := os.Getenv("TS_LOGIN_SERVER")
	if defLogin == "" {
		defLogin = "https://login.tailscale.com"
	}
	defHost := os.Getenv("TS_HOSTNAME")
	if defHost == "" {
		defHost = fmt.Sprintf("gn-forward-%s", strings.ToLower(strings.Split(runtime.GOOS+"-"+runtime.GOARCH, "-")[0]))
	}

	flag.Var(&maps, "map", "Forward mapping in the form listen=dest (repeatable). Example: -map 127.0.0.1:50010=10.0.0.10:50000")
	flag.StringVar(&loginServer, "login-server", defLogin, "Tailscale/Headscale login server URL")
	flag.StringVar(&authKey, "authkey", os.Getenv("TS_AUTHKEY"), "Tailscale auth key (or TS_AUTHKEY env)")
	flag.StringVar(&hostname, "hostname", defHost, "tsnet hostname for this forwarder")
	flag.BoolVar(&verbose, "v", false, "verbose logging")
	flag.Parse()
	log.Printf("tsnet-forward starting; mappings=%d, hostname=%s, login=%s", len(maps), hostname, loginServer)
	for i, m := range maps {
		log.Printf("arg map[%d]: %s", i, m)
	}

	if len(maps) == 0 {
		log.Fatalf("no -map provided; specify at least one listen=dest mapping")
	}
	if authKey == "" {
		log.Fatalf("no auth key provided; set TS_AUTHKEY env or use -authkey")
	}

	srv := &tsnet.Server{
		Hostname:   hostname,
		AuthKey:    authKey,
		ControlURL: loginServer,
		Ephemeral:  true,
		Dir:        "", // in-memory state
		Logf: func(format string, args ...any) {
			if verbose {
				log.Printf("tsnet: "+format, args...)
			}
		},
	}
	defer srv.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Start listeners first so we can verify local bindings early.
	log.Printf("starting listener setup for %d mappings", len(maps))
	errs := make(chan error, len(maps))
	for i, m := range maps {
		mp, err := parseMapping(m)
		if err != nil {
			log.Fatalf("%v", err)
		}
		log.Printf("parsed map[%d]: %s => %s", i, mp.listen, mp.dest)
		go func(mp mapping) {
			log.Printf("listener init: trying %s -> %s", mp.listen, mp.dest)
			ln, err := net.Listen("tcp", mp.listen)
			if err != nil {
				log.Printf("listen failed on %s: %v", mp.listen, err)
				errs <- fmt.Errorf("listen %s: %w", mp.listen, err)
				return
			}
			log.Printf("forwarding %s -> %s (via tsnet)", mp.listen, mp.dest)
			for {
				c, err := ln.Accept()
				if err != nil {
					if ne, ok := err.(net.Error); ok && ne.Timeout() {
						time.Sleep(100 * time.Millisecond)
						continue
					}
					errs <- fmt.Errorf("accept on %s: %w", mp.listen, err)
					return
				}
				go handleConn(ctx, srv, c, mp.dest)
			}
		}(mp)
	}

	// Bring up tsnet after listeners are bound.
	// Dial attempts will block until Up succeeds, which is fine.
	if _, err := srv.Up(context.Background()); err != nil {
		log.Fatalf("tsnet up failed: %v", err)
	}

	select {
	case <-ctx.Done():
		log.Printf("shutting down")
		return
	case err := <-errs:
		log.Fatalf("error: %v", err)
	}
}

func handleConn(ctx context.Context, srv *tsnet.Server, c net.Conn, dest string) {
	defer c.Close()
	rc, err := srv.Dial(ctx, "tcp", dest)
	if err != nil {
		log.Printf("dial %s via tsnet failed: %v", dest, err)
		return
	}
	defer rc.Close()
	// Bidirectional copy
	done := make(chan struct{}, 2)
	go func() { io.Copy(rc, c); rc.(*net.TCPConn).CloseWrite(); done <- struct{}{} }()
	go func() { io.Copy(c, rc); c.(*net.TCPConn).CloseWrite(); done <- struct{}{} }()
	<-done
}
