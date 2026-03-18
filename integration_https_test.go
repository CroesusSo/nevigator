package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yourorg/proxy-pac/pac"
	"github.com/yourorg/proxy-pac/proxy"
	"github.com/yourorg/proxy-pac/router"
	"github.com/yourorg/proxy-pac/socks5"
)

// generateSelfSignedCert creates a temporary self-signed certificate for testing.
func generateSelfSignedCert(t *testing.T) (certPath, keyPath string) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(24 * time.Hour),
		KeyUsage:  x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
		BasicConstraintsValid: true,
	}
	template.DNSNames = []string{"localhost", "127.0.0.1"}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}
	tmpDir := t.TempDir()
	certPath = filepath.Join(tmpDir, "cert.pem")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}), 0600); err != nil {
		t.Fatalf("failed to write cert: %v", err)
	}
	keyPath = filepath.Join(tmpDir, "key.pem")
	privDER := x509.MarshalPKCS1PrivateKey(priv)
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privDER}), 0600); err != nil {
		t.Fatalf("failed to write key: %v", err)
	}
	return certPath, keyPath
}

// TestHTTPSProxy_ListenerStartup tests that HTTPS proxy listener starts and accepts TLS connections.
func TestHTTPSProxy_ListenerStartup(t *testing.T) {
	// Generate self-signed cert
	certPath, keyPath := generateSelfSignedCert(t)

	// Create a minimal PAC file
	tmpDir := t.TempDir()
	pacPath := filepath.Join(tmpDir, "test.pac")
	pacContent := `function FindProxyForURL(url, host) { return "DIRECT"; }`
	if err := os.WriteFile(pacPath, []byte(pacContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Configure proxy with HTTPS on a specific port (use high number to avoid conflicts)
	httpsPort := "127.0.0.1:18443"
	proxyCfg := pac.ProxyConfig{
		HTTPListen:  "127.0.0.1:0",
		HTTPSListen: httpsPort,
		AdminListen: "127.0.0.1:0",
		PACPath:     pacPath,
		TLSConfig: pac.TLSConfig{
			CertificateFile: certPath,
			KeyFile:         keyPath,
		},
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
	defer serv.Stop()

	// Give server time to start
	time.Sleep(200 * time.Millisecond)

	// Verify Status reports the HTTPS address
	status := serv.Status()
	if status.HTTPSListen != httpsPort {
		t.Errorf("HTTPSListen in status = %q, want %q", status.HTTPSListen, httpsPort)
	}

	// Try to establish a TLS connection to the HTTPS proxy
	conn, err := tls.Dial("tcp", httpsPort, &tls.Config{
		InsecureSkipVerify: true, // skip cert verification for test
		ServerName:         "localhost",
	})
	if err != nil {
		t.Fatalf("TLS connection to HTTPS proxy failed: %v", err)
	}
	conn.Close()
}

// TestHTTPSProxy_DisabledWhenNoTLS tests that HTTPS listener is not started when TLS config is invalid.
func TestHTTPSProxy_DisabledWhenNoTLS(t *testing.T) {
	tmpDir := t.TempDir()
	pacPath := filepath.Join(tmpDir, "test.pac")
	if err := os.WriteFile(pacPath, []byte(`function FindProxyForURL(url, host) { return "DIRECT"; }`), 0644); err != nil {
		t.Fatal(err)
	}

	httpsPort := "127.0.0.1:18444" // different port
	// Configure HTTPSListen but NO TLS certificate
	proxyCfg := pac.ProxyConfig{
		HTTPSListen: httpsPort,
		TLSConfig:   pac.TLSConfig{}, // empty
	}

	evalCfg := pac.EvaluatorConfig{
		PACPath:       pacPath,
		LocalIP:       "127.0.0.1",
		FallbackRoute: pac.Route{Strategy: "DIRECT"},
	}
	evaluator, _ := pac.NewEvaluator(evalCfg)
	defer evaluator.Close()

	routerCfg := pac.RouterConfig{TUNIP: nil}
	r, _ := router.NewRouter(routerCfg)

	serv := proxy.NewServer(proxyCfg, evaluator, r)
	if err := serv.Start(); err != nil {
		t.Fatalf("Server Start: %v", err)
	}
	defer serv.Stop()

	time.Sleep(200 * time.Millisecond)

	// HTTPS should not be listening; connection should be refused
	_, err := net.Dial("tcp", httpsPort)
	if err == nil {
		t.Fatal("expected connection to fail when TLS not configured, but succeeded")
	}
}

// TestHTTPConnect_IPv6 tests that HTTP CONNECT to an IPv6 destination works correctly.
func TestHTTPConnect_IPv6(t *testing.T) {
	// Skip if IPv6 is not available on the system
	// Check if we can listen on ::1
	listener, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skip("IPv6 not available on this system")
	}
	listener.Close()

	// Start a simple echo server on an IPv6 address to act as the destination
	destLn, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Fatalf("Failed to start destination server: %v", err)
	}
	destPort := destLn.Addr().(*net.TCPAddr).Port
	destAddrWithPort := net.JoinHostPort("::1", strconv.Itoa(destPort))
	defer destLn.Close()

	// Echo server goroutine
	go func() {
		conn, err := destLn.Accept()
		if err != nil {
			t.Logf("Destination accept error: %v", err)
			return
		}
		defer conn.Close()
		// Echo data back
		io.Copy(conn, conn)
	}()

	// Create proxy server with HTTP CONNECT support
	tmpDir := t.TempDir()
	pacPath := filepath.Join(tmpDir, "test.pac")
	// PAC that routes everything to DIRECT
	pacContent := `function FindProxyForURL(url, host) { return "DIRECT"; }`
	if err := os.WriteFile(pacPath, []byte(pacContent), 0644); err != nil {
		t.Fatal(err)
	}

	proxyAddr := getFreePort(t)
	proxyCfg := pac.ProxyConfig{
		HTTPListen:  proxyAddr,
		AdminListen: getFreePort(t),
		PACPath:     pacPath,
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
	defer serv.Stop()

	// Give server time to start
	time.Sleep(200 * time.Millisecond)

	// Connect to proxy and send CONNECT request to IPv6 destination
	conn, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		t.Fatalf("Failed to connect to proxy: %v", err)
	}
	defer conn.Close()

	// Send HTTP CONNECT request
	connectReq := strings.Builder{}
	connectReq.WriteString("CONNECT ")
	connectReq.WriteString(destAddrWithPort)
	connectReq.WriteString(" HTTP/1.1\r\nHost: ")
	connectReq.WriteString(destAddrWithPort)
	connectReq.WriteString("\r\n\r\n")
	_, err = conn.Write([]byte(connectReq.String()))
	if err != nil {
		t.Fatalf("Failed to send CONNECT request: %v", err)
	}

	// Read response
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}
	_resp := string(buf[:n])
	if !strings.Contains(_resp, "200") {
		t.Fatalf("Expected 200 response, got: %s", _resp)
	}

	// Tunnel established - send test data
	testMsg := "Hello IPv6"
	_, err = conn.Write([]byte(testMsg))
	if err != nil {
		t.Fatalf("Failed to send test data: %v", err)
	}

	// Read echo response
	n, err = conn.Read(buf)
	if err != nil {
		t.Fatalf("Failed to read echo: %v", err)
	}
	got := string(buf[:n])
	if got != testMsg {
		t.Fatalf("Echo mismatch: got %q, want %q", got, testMsg)
	}

	t.Log("IPv6 HTTP CONNECT test passed")
}

// TestSOCKS5Connect_IPv6_Direct tests SOCKS5 CONNECT to IPv6 destination via DIRECT route.
func TestSOCKS5Connect_IPv6_Direct(t *testing.T) {
	// Skip if IPv6 not available
	if _, err := net.Listen("tcp6", "[::1]:0"); err != nil {
		t.Skip("IPv6 not available on this system")
	}

	// Start destination server on IPv6
	destLn, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Fatalf("Failed to start destination server: %v", err)
	}
	destPort := destLn.Addr().(*net.TCPAddr).Port
	destHost := "::1"
	defer destLn.Close()

	// Echo server
	go func() {
		conn, err := destLn.Accept()
		if err != nil {
			t.Logf("Destination accept error: %v", err)
			return
		}
		defer conn.Close()
		io.Copy(conn, conn)
	}()

	// Create proxy server with SOCKS5
	tmpDir := t.TempDir()
	pacPath := filepath.Join(tmpDir, "test.pac")
	pacContent := `function FindProxyForURL(url, host) { return "DIRECT"; }`
	if err := os.WriteFile(pacPath, []byte(pacContent), 0644); err != nil {
		t.Fatal(err)
	}

	socks5Addr := getFreePort(t)
	proxyCfg := pac.ProxyConfig{
		SOCKS5Listen: socks5Addr,
		HTTPListen:   getFreePort(t), // avoid default port 80
		AdminListen:  getFreePort(t),
		PACPath:      pacPath,
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
	defer serv.Stop()

	// Give server time to start
	time.Sleep(200 * time.Millisecond)

	t.Logf("SOCKS5 proxy listening on %s", socks5Addr)

	// Connect to SOCKS5 proxy
	conn, err := net.Dial("tcp", socks5Addr)
	if err != nil {
		t.Fatalf("Failed to connect to SOCKS5 proxy: %v", err)
	}
	defer conn.Close()

	// SOCKS5 handshake - greeting
	greeting := []byte{socks5.Version, 1, socks5.AuthNone}
	_, err = conn.Write(greeting)
	if err != nil {
		t.Fatalf("Failed to write greeting: %v", err)
	}

	// Read response
	resp := make([]byte, 2)
	err = socks5.ReadFull(conn, resp)
	if err != nil {
		t.Fatalf("Failed to read greeting response: %v", err)
	}
	if resp[0] != socks5.Version || resp[1] != socks5.AuthNone {
		t.Fatalf("Unexpected auth method: version=%d, method=%d", resp[0], resp[1])
	}

	// Build SOCKS5 CONNECT request to IPv6 destination
	req := make([]byte, 0, 20)
	req = append(req, socks5.Version, socks5.CMDConnect, 0x00, socks5.ATYPIPv6)
	ip := net.ParseIP(destHost)
	if ip.To16() == nil || len(ip.To16()) != 16 {
		t.Fatalf("Invalid IPv6 address: %s", destHost)
	}
	req = append(req, ip.To16()...)
	portBytes := []byte{byte(destPort >> 8), byte(destPort & 0xff)}
	req = append(req, portBytes...)
	_, err = conn.Write(req)
	if err != nil {
		t.Fatalf("Failed to write CONNECT request: %v", err)
	}

	// Read response - at least 10 bytes (IPv6 response)
	resp = make([]byte, 10)
	err = socks5.ReadFull(conn, resp)
	if err != nil {
		t.Fatalf("Failed to read CONNECT response: %v", err)
	}
	if resp[0] != socks5.Version {
		t.Fatalf("Invalid SOCKS5 version in response: %d", resp[0])
	}
	if resp[1] != socks5.REPSuccess {
		t.Fatalf("SOCKS5 CONNECT failed with code %d", resp[1])
	}

	// Connection established - send test data
	testMsg := "Hello IPv6 SOCKS5"
	_, err = conn.Write([]byte(testMsg))
	if err != nil {
		t.Fatalf("Failed to send test data: %v", err)
	}

	// Read echo
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("Failed to read echo: %v", err)
	}
	got := string(buf[:n])
	if got != testMsg {
		t.Fatalf("Echo mismatch: got %q, want %q", got, testMsg)
	}

	t.Log("IPv6 SOCKS5 CONNECT (direct) test passed")
}

// TestSOCKS5Connect_IPv6_Upstream tests SOCKS5 CONNECT to IPv6 destination through upstream SOCKS5 proxy.
func TestSOCKS5Connect_IPv6_Upstream(t *testing.T) {
	// Skip if IPv6 not available
	if _, err := net.Listen("tcp6", "[::1]:0"); err != nil {
		t.Skip("IPv6 not available on this system")
	}

	// Start destination echo server on IPv6
	destLn, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Fatalf("Failed to start destination server: %v", err)
	}
	destPort := destLn.Addr().(*net.TCPAddr).Port
	destHost := "::1"
	defer destLn.Close()

	go func() {
		conn, err := destLn.Accept()
		if err != nil {
			t.Logf("Destination accept error: %v", err)
			return
		}
		defer conn.Close()
		io.Copy(conn, conn)
	}()

	// Start an upstream SOCKS5 proxy (no auth, just forwards)
	upstreamLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start upstream SOCKS5: %v", err)
	}
	upstreamAddr := upstreamLn.Addr().String()
	defer upstreamLn.Close()

	go func() {
		for {
			conn, err := upstreamLn.Accept()
			if err != nil {
				return
			}
			go socks5HandleForward(conn, destHost, destPort, t)
		}
	}()

	// Create main proxy server with PAC that routes through upstream SOCKS5
	tmpDir := t.TempDir()
	pacPath := filepath.Join(tmpDir, "test.pac")
	// PAC returns SOCKS5 upstream
	pacContent := `function FindProxyForURL(url, host) { return "SOCKS5 ` + upstreamAddr + `"; }`
	if err := os.WriteFile(pacPath, []byte(pacContent), 0644); err != nil {
		t.Fatal(err)
	}

	socks5Addr := getFreePort(t)
	proxyCfg := pac.ProxyConfig{
		SOCKS5Listen: socks5Addr,
		HTTPListen:   getFreePort(t),
		AdminListen:  getFreePort(t),
		PACPath:      pacPath,
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
	defer serv.Stop()

	// Give server time to start
	time.Sleep(200 * time.Millisecond)

	t.Logf("Main SOCKS5 proxy on %s, upstream is %s", socks5Addr, upstreamAddr)

	// Connect client to main SOCKS5 proxy
	conn, err := net.Dial("tcp", socks5Addr)
	if err != nil {
		t.Fatalf("Failed to connect to main SOCKS5: %v", err)
	}
	defer conn.Close()

	// SOCKS5 handshake
	greeting := []byte{socks5.Version, 1, socks5.AuthNone}
	_, err = conn.Write(greeting)
	if err != nil {
		t.Fatalf("Write greeting failed: %v", err)
	}
	resp := make([]byte, 2)
	err = socks5.ReadFull(conn, resp)
	if err != nil {
		t.Fatalf("Read greeting response failed: %v", err)
	}
	if resp[1] != socks5.AuthNone {
		t.Fatalf("Auth method not none: %d", resp[1])
	}

	// Build CONNECT to IPv6 destination through upstream
	req := make([]byte, 0, 20)
	req = append(req, socks5.Version, socks5.CMDConnect, 0x00, socks5.ATYPIPv6)
	ip := net.ParseIP(destHost)
	req = append(req, ip.To16()...)
	portBytes := []byte{byte(destPort >> 8), byte(destPort & 0xff)}
	req = append(req, portBytes...)
	_, err = conn.Write(req)
	if err != nil {
		t.Fatalf("Write CONNECT request failed: %v", err)
	}

	// Read response
	resp = make([]byte, 10)
	err = socks5.ReadFull(conn, resp)
	if err != nil {
		t.Fatalf("Read CONNECT response failed: %v", err)
	}
	if resp[1] != socks5.REPSuccess {
		t.Fatalf("SOCKS5 CONNECT failed with code %d", resp[1])
	}

	// Tunnel established - test data roundtrip
	testMsg := "Hello IPv6 via Upstream"
	_, err = conn.Write([]byte(testMsg))
	if err != nil {
		t.Fatalf("Failed to send test data: %v", err)
	}

	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("Failed to read echo: %v", err)
	}
	got := string(buf[:n])
	if got != testMsg {
		t.Fatalf("Echo mismatch: got %q, want %q", got, testMsg)
	}

	t.Log("IPv6 SOCKS5 CONNECT (upstream) test passed")
}

// socks5HandleForward is a simple SOCKS5 forwarder used in upstream proxy test.
func socks5HandleForward(clientConn net.Conn, destHost string, destPort int, t *testing.T) {
	defer clientConn.Close()

	// Greet client
	buf := make([]byte, 258)
	if err := socks5.ReadFull(clientConn, buf[:2]); err != nil {
		return
	}
	if buf[0] != socks5.Version {
		return
	}
	nmethods := int(buf[1])
	if err := socks5.ReadFull(clientConn, buf[:nmethods]); err != nil {
		return
	}
	// Always select no-auth
	clientConn.Write([]byte{socks5.Version, socks5.AuthNone})

	// Read request
	if err := socks5.ReadFull(clientConn, buf[:4]); err != nil {
		return
	}
	if buf[0] != socks5.Version {
		return
	}
	cmd := buf[1]
	if cmd != socks5.CMDConnect {
		clientConn.Write(socks5.EncodeReply(socks5.REPCommandNotSupported))
		return
	}

	// Read address
	if err := socks5.ReadFull(clientConn, buf[4:5]); err != nil {
		return
	}
	atyp := buf[3]
	addrLen := 0
	switch atyp {
	case socks5.ATYPIPv4:
		addrLen = 4
	case socks5.ATYPDomain:
		addrLen = int(buf[4]) + 1
	case socks5.ATYPIPv6:
		addrLen = 16
	default:
		clientConn.Write(socks5.EncodeReply(socks5.REPAddrTypeNotSupported))
		return
	}
	if err := socks5.ReadFull(clientConn, buf[5:5+addrLen+2-1]); err != nil {
		return
	}

	// Connect to actual destination
	dest := net.JoinHostPort(destHost, strconv.Itoa(destPort))
	outbound, err := net.Dial("tcp", dest)
	if err != nil {
		t.Logf("Upstream failed to connect to destination: %v", err)
		clientConn.Write(socks5.EncodeReply(socks5.REPGeneralFailure))
		return
	}
	defer outbound.Close()

	// Send success reply
	clientConn.Write(socks5.EncodeReply(socks5.REPSuccess))

	// Relay both directions concurrently
	errCh := make(chan error, 2)
	go func() {
		_, err := io.Copy(outbound, clientConn)
		errCh <- err
	}()
	go func() {
		_, err := io.Copy(clientConn, outbound)
		errCh <- err
	}()
	<-errCh
}

// getFreePort returns a free "host:port" string.
func getFreePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to get free port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().String()
}
