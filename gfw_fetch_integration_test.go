package main

import (
	"context"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yourorg/proxy-pac/gfw"
	"github.com/yourorg/proxy-pac/pac"
	"github.com/yourorg/proxy-pac/proxy"
	"github.com/yourorg/proxy-pac/router"
	"github.com/yourorg/proxy-pac/socks5"
)

// upstreamTarget records the destination a SOCKS5 client asked for.
type upstreamTarget struct {
	host string
	port uint16
}

// fakeUpstream is a SOCKS5 server standing in for the configured external
// upstream proxy. It records the destination it was asked for and relays to a
// fixed origin, so a test can see both that the request arrived and what name
// it carried.
type fakeUpstream struct {
	addr    string
	origin  string
	targets chan upstreamTarget
	wg      sync.WaitGroup
}

func startFakeUpstream(t *testing.T, origin string) *fakeUpstream {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	u := &fakeUpstream{
		addr:    ln.Addr().String(),
		origin:  origin,
		targets: make(chan upstreamTarget, 4),
	}
	u.wg.Add(1)
	go func() {
		defer u.wg.Done()
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go u.handle(c)
		}
	}()
	t.Cleanup(func() {
		ln.Close()
		u.wg.Wait()
	})
	return u
}

func (u *fakeUpstream) handle(c net.Conn) {
	defer c.Close()

	// Greeting: VER, NMETHODS, METHODS
	hdr := make([]byte, 2)
	if _, err := io.ReadFull(c, hdr); err != nil {
		return
	}
	methods := make([]byte, hdr[1])
	if _, err := io.ReadFull(c, methods); err != nil {
		return
	}
	if _, err := c.Write([]byte{socks5.Version, socks5.AuthNone}); err != nil {
		return
	}

	// Request: VER, CMD, RSV, ATYP, ADDR, PORT
	req := make([]byte, 4)
	if _, err := io.ReadFull(c, req); err != nil {
		return
	}
	if req[0] != socks5.Version || req[1] != socks5.CMDConnect {
		return
	}
	var host string
	var port uint16
	switch req[3] {
	case socks5.ATYPDomain:
		l := make([]byte, 1)
		if _, err := io.ReadFull(c, l); err != nil {
			return
		}
		name := make([]byte, l[0])
		if _, err := io.ReadFull(c, name); err != nil {
			return
		}
		host = string(name)
		pb := make([]byte, 2)
		if _, err := io.ReadFull(c, pb); err != nil {
			return
		}
		port = uint16(pb[0])<<8 | uint16(pb[1])
	case socks5.ATYPIPv4:
		rest := make([]byte, 6)
		if _, err := io.ReadFull(c, rest); err != nil {
			return
		}
		host = net.IP(rest[:4]).String()
		port = uint16(rest[4])<<8 | uint16(rest[5])
	default:
		return
	}
	u.targets <- upstreamTarget{host: host, port: port}

	out, err := net.Dial("tcp", u.origin)
	if err != nil {
		c.Write(socks5.EncodeReply(socks5.REPGeneralFailure))
		return
	}
	defer out.Close()
	c.Write(socks5.EncodeReply(socks5.REPSuccess))
	go io.Copy(out, c)
	io.Copy(c, out)
}

// gfwListBody is the payload a GFW list server returns: the base64 of a
// one-rule list.
func gfwListBody() string {
	return base64.StdEncoding.EncodeToString([]byte("||blocked.example.com\n"))
}

// TestGFWFetchGoesThroughLocalSocks5AndUpstream proves that the GFW list is
// fetched through this instance's own SOCKS5 listener and leaves through the
// configured external upstream proxy.
//
// It covers the first-start case: no GFW rule for the source host exists
// yet, so the pinned fallback route is what carries the request. Without it
// the fetch would go out directly and fail on a censored network.
func TestGFWFetchGoesThroughLocalSocks5AndUpstream(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(gfwListBody()))
	}))
	defer origin.Close()
	upstream := startFakeUpstream(t, strings.TrimPrefix(origin.URL, "http://"))

	tmpDir := t.TempDir()
	pacPath := filepath.Join(tmpDir, "proxy.pac")
	// An empty rule set: this is the state before the first fetch.
	if err := os.WriteFile(pacPath, []byte(gfw.GeneratePAC(nil, nil, gfw.FWMeta{})), 0644); err != nil {
		t.Fatal(err)
	}

	const sourceHost = "raw.githubusercontent.com"
	ev, err := pac.NewEvaluator(pac.EvaluatorConfig{
		PACPath:       pacPath,
		EvalTimeout:   time.Second,
		FallbackRoute: pac.Route{Strategy: "DIRECT"},
		HostFallbacks: map[string]pac.Route{
			sourceHost: pac.GFWSourceRoute("upstream-1"),
		},
	})
	if err != nil {
		t.Fatalf("NewEvaluator: %v", err)
	}
	defer ev.Close()

	rt, err := router.NewRouter(pac.RouterConfig{DialTimeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	rt.SetProxyRegistry([]pac.ExternalProxy{{ID: "upstream-1", Kind: "socks5", Address: upstream.addr}})
	rt.SetDefaultProxyID("upstream-1")

	srv := proxy.NewServer(pac.ProxyConfig{
		HTTPListen:   "127.0.0.1:0",
		SOCKS5Listen: "127.0.0.1:0",
		PACPath:      pacPath,
	}, ev, rt)
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Stop()

	localAddr := srv.SOCKS5DialAddr()
	if localAddr == "" {
		t.Fatal("the local SOCKS5 listener has no address")
	}

	rulesPath := filepath.Join(tmpDir, "gfw-rules.json")
	fetcher := gfw.NewFetcher("http://"+sourceHost+"/gfwlist/gfwlist/master/gfwlist.txt", rulesPath, 10*time.Second)
	transport := socks5.NewHTTPTransport(socks5.HTTPTransportOptions{
		ProxyAddr: localAddr,
		Timeout:   10 * time.Second,
	})
	defer transport.CloseIdleConnections()
	fetcher.HTTPClient = &http.Client{
		Transport: transport,
		Timeout:   15 * time.Second,
	}

	result := fetcher.Fetch(context.Background())
	if result.Error != nil {
		t.Fatalf("Fetch: %v", result.Error)
	}
	if !result.Updated || result.RuleCount != 1 {
		t.Errorf("result = %+v, want 1 updated rule", result)
	}

	// The request must have reached the external upstream, carrying the
	// unresolved source name.
	select {
	case tgt := <-upstream.targets:
		if tgt.host != sourceHost {
			t.Errorf("upstream host = %q, want %q", tgt.host, sourceHost)
		}
		if tgt.port != 80 {
			t.Errorf("upstream port = %d, want 80", tgt.port)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the external upstream never received the request; " +
			"the fetch did not leave through the configured upstream")
	}

	rules, err := gfw.LoadRules(rulesPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || rules[0].Value != ".blocked.example.com" {
		t.Errorf("saved rules = %+v, want one rule for .blocked.example.com", rules)
	}
}

// TestGFWFetchRouteDependsOnPinnedFallback proves that the pinned route is
// what carries the fetch, by showing that the same host evaluates to the
// global fallback route without it.
//
// The upstream is registered as the default proxy, so a request that reached
// the router would take it. A request that evaluates to the global fallback
// leaves the host directly instead.
func TestGFWFetchRouteDependsOnPinnedFallback(t *testing.T) {
	tmpDir := t.TempDir()
	pacPath := filepath.Join(tmpDir, "proxy.pac")
	if err := os.WriteFile(pacPath, []byte(gfw.GeneratePAC(nil, nil, gfw.FWMeta{})), 0644); err != nil {
		t.Fatal(err)
	}

	const sourceHost = "raw.githubusercontent.com"
	ev, err := pac.NewEvaluator(pac.EvaluatorConfig{
		PACPath:       pacPath,
		EvalTimeout:   time.Second,
		FallbackRoute: pac.Route{Strategy: "DIRECT"},
	})
	if err != nil {
		t.Fatalf("NewEvaluator: %v", err)
	}
	defer ev.Close()

	if got := ev.Evaluate(sourceHost); got.Strategy != "DIRECT" {
		t.Errorf("unpinned route = %q, want DIRECT", got.Strategy)
	}

	ev.SetHostFallback(sourceHost, pac.GFWSourceRoute("upstream-1"))
	if got := ev.Evaluate(sourceHost); got.Strategy != "DEFAULT" {
		t.Errorf("pinned route = %q, want DEFAULT", got.Strategy)
	}
}
