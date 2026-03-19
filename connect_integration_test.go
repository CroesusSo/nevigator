package main

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yourorg/proxy-pac/gfw"
	"github.com/yourorg/proxy-pac/pac"
	"github.com/yourorg/proxy-pac/proxy"
	"github.com/yourorg/proxy-pac/router"
)

func sendConnect(conn net.Conn, host string, port int) (int, string, error) {
	req := fmt.Sprintf("CONNECT %s:%d HTTP/1.1\r\nHost: %s:%d\r\n\r\n", host, port, host, port)
	if _, err := conn.Write([]byte(req)); err != nil {
		return 0, "", err
	}
	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		return 0, "", err
	}
	return resp.StatusCode, resp.Status, nil
}

func TestConnectRouting(t *testing.T) {
	tmpDir := t.TempDir()
	pacPath := filepath.Join(tmpDir, "test.pac")
	pacContent := gfw.GeneratePAC(nil, nil, gfw.FWMeta{})
	if err := os.WriteFile(pacPath, []byte(pacContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Use a high, likely free port
	proxyCfg := pac.ProxyConfig{
		HTTPListen:   "127.0.0.1:28080",
		SOCKS5Listen: "",
		PACPath:      pacPath,
		TUNDevice:    "", // disabled
	}

	evalCfg := pac.EvaluatorConfig{
		PACPath:        pacPath,
		LocalIP:        "127.0.0.1",
		DNSTimeout:     50 * time.Millisecond,
		EvalTimeout:    50 * time.Millisecond,
		EnableDNSCache: false,
		FallbackRoute:  pac.Route{Strategy: "DIRECT"},
	}
	evaluator, err := pac.NewEvaluator(evalCfg)
	if err != nil {
		t.Fatalf("NewEvaluator: %v", err)
	}
	defer evaluator.Close()

	routerCfg := pac.RouterConfig{
		TUNIP:       nil,
		DialTimeout: 5 * time.Second,
	}
	r, err := router.NewRouter(routerCfg)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	serv := proxy.NewServer(proxyCfg, evaluator, r)
	if err := serv.Start(); err != nil {
		t.Fatalf("Server Start: %v", err)
	}
	// Give the server a moment to start listening
	time.Sleep(100 * time.Millisecond)
	defer serv.Stop()

	status := serv.Status()
	httpAddr := status.HTTPListen
	if httpAddr == "" {
		t.Fatal("HTTP listener not started")
	}

	t.Run("HTTP CONNECT", func(t *testing.T) {
		conn, err := net.Dial("tcp", httpAddr)
		if err != nil {
			t.Fatalf("Dial: %v", err)
		}
		defer conn.Close()
		code, respStatus, err := sendConnect(conn, "www.google.com", 443)
		if err != nil {
			t.Fatalf("sendConnect: %v", err)
		}
		if code == 404 {
			t.Fatalf("got 404 for CONNECT, wanted non-404. status=%s", respStatus)
		}
		t.Logf("HTTP CONNECT response: %d %s", code, respStatus)
	})
}
