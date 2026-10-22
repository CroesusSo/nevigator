package proxy

import (
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yourorg/proxy-pac/pac"
	"github.com/yourorg/proxy-pac/router"
	"github.com/yourorg/proxy-pac/socks5"
	"github.com/yourorg/proxy-pac/status"
)

// Server implements the HTTP CONNECT and SOCKS5 proxy.
type Server struct {
	cfg         pac.ProxyConfig
	evaluator   *pac.Evaluator
	router      *router.Router
	httpServer  *http.Server
	httpsServer *http.Server // Optional TLS-wrapped HTTP proxy
	socks5Conn  chan net.Conn
	stopped     chan struct{}

	mu         sync.RWMutex
	conns      map[string]*status.Connection
	totalConns atomic.Int64
	nextID     atomic.Int64
	tlsConfig  *tls.Config
}

// NewServer creates a new proxy server.
func NewServer(cfg pac.ProxyConfig, evaluator *pac.Evaluator, router *router.Router) *Server {
	s := &Server{
		cfg:        cfg,
		evaluator:  evaluator,
		router:     router,
		socks5Conn: make(chan net.Conn, 100),
		stopped:    make(chan struct{}),
		conns:      make(map[string]*status.Connection),
	}
	mux := http.NewServeMux()

	// Top-level handler: intercept CONNECT, otherwise delegate to mux (admin/API)
	topHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect {
			host, portStr, err := net.SplitHostPort(r.Host)
			if err != nil {
				http.Error(w, "Invalid CONNECT path", http.StatusBadRequest)
				return
			}
			port, _ := strconv.Atoi(portStr)
			s.handleCONNECT(w, r, host, port)
			return
		}
		mux.ServeHTTP(w, r)
	})

	s.httpServer = &http.Server{
		Addr:    cfg.HTTPListen,
		Handler: topHandler,
	}

	// Initialize TLS config if TLS settings are provided
	if cfg.TLSConfig.CertificateFile != "" && cfg.TLSConfig.KeyFile != "" {
		tlsConfig, err := s.loadTLSConfig(&cfg.TLSConfig)
		if err != nil {
			log.Printf("Warning: Failed to load TLS config: %v", err)
		} else {
			s.tlsConfig = tlsConfig
		}
	}

	// Initialize HTTPS server if configured
	if cfg.HTTPSListen != "" {
		if s.tlsConfig == nil {
			log.Printf("Warning: HTTPS proxy configured on %s but TLS certificate/key not provided", cfg.HTTPSListen)
		} else {
			s.httpsServer = &http.Server{
				Addr:    cfg.HTTPSListen,
				Handler: topHandler,
			}
			// Start HTTPS listener in a separate goroutine
			go func() {
				ln, err := net.Listen("tcp", cfg.HTTPSListen)
				if err != nil {
					log.Printf("HTTPS proxy listen failed on %s: %v", cfg.HTTPSListen, err)
					s.httpsServer = nil
					return
				}
				tlsLn := tls.NewListener(ln, s.tlsConfig)
				if err := s.httpsServer.Serve(tlsLn); err != nil && !errors.Is(err, http.ErrServerClosed) {
					log.Printf("HTTPS proxy server error: %v", err)
				}
			}()
			log.Printf("HTTPS proxy listening on %s", cfg.HTTPSListen)
		}
	}

	return s
}

// loadTLSConfig loads TLS configuration from the provided config.
func (s *Server) loadTLSConfig(cfg *pac.TLSConfig) (*tls.Config, error) {
	// Load server certificate
	cert, err := tls.LoadX509KeyPair(cfg.CertificateFile, cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load server certificate: %w", err)
	}

	// Load root CA if specified
	var rootCA *x509.CertPool
	if cfg.CAFile != "" {
		rootCA, err = loadCAFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load CA file: %w", err)
		}
	} else {
		// Use system root CAs
		rootCA, err = x509.SystemCertPool()
		if err != nil {
			rootCA = x509.NewCertPool()
		}
	}

	// Load client CAs if specified
	var clientCA *x509.CertPool
	if cfg.ClientCAs != "" {
		clientCA, err = loadCAFile(cfg.ClientCAs)
		if err != nil {
			return nil, fmt.Errorf("failed to load client CA file: %w", err)
		}
	}

	// Determine client auth type
	var clientAuth tls.ClientAuthType
	switch cfg.ClientAuth {
	case "":
		clientAuth = tls.NoClientCert
	case "request":
		clientAuth = tls.RequestClientCert
	case "require_any":
		clientAuth = tls.RequireAnyClientCert
	case "verify_if_given":
		clientAuth = tls.VerifyClientCertIfGiven
	case "require_verify":
		clientAuth = tls.RequireAndVerifyClientCert
	default:
		clientAuth = tls.NoClientCert
	}

	// Build TLS config
	tlsConfig := &tls.Config{
		Certificates:             []tls.Certificate{cert},
		RootCAs:                  rootCA,
		ClientCAs:                clientCA,
		MinVersion:               cfg.MinVersion,
		MaxVersion:               cfg.MaxVersion,
		CipherSuites:             cfg.CipherSuites,
		PreferServerCipherSuites: cfg.PreferServerCipherSuites,
		SessionTicketsDisabled:   cfg.SessionTicketsDisabled,
		InsecureSkipVerify:       cfg.InsecureSkipVerify,
		ClientAuth:               clientAuth,
	}

	// Set custom verification callback if provided
	if cfg.VerifyPeerCertificate != nil {
		tlsConfig.VerifyPeerCertificate = func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			// Convert verifiedChains to the expected format
			var chains [][][]byte
			for _, chain := range verifiedChains {
				var bchain [][]byte
				for _, cert := range chain {
					bchain = append(bchain, cert.Raw)
				}
				chains = append(chains, bchain)
			}
			return cfg.VerifyPeerCertificate(rawCerts, chains)
		}
	}

	return tlsConfig, nil
}

// loadCAFile loads a PEM-encoded CA file and returns a cert pool.
func loadCAFile(path string) (*x509.CertPool, error) {
	pemData, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	certPool := x509.NewCertPool()
	if ok := certPool.AppendCertsFromPEM(pemData); !ok {
		return nil, fmt.Errorf("failed to parse CA file %s", path)
	}

	return certPool, nil
}

// formatDest constructs a destination address suitable for net.Dial and routing.
// net.Dial requires IPv6 literals to be bracketed (e.g., [::1]:443).
// This helper ensures any IPv6 host (detected by presence of ':') is wrapped
// in brackets if not already, while leaving IPv4 and domain names unchanged.
func formatDest(host string, port int) string {
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") && !strings.HasSuffix(host, "]") {
		return fmt.Sprintf("[%s]:%d", host, port)
	}
	return fmt.Sprintf("%s:%d", host, port)
}

// Start begins accepting connections.
func (s *Server) Start() error {
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("HTTP proxy error: %v", err)
		}
	}()
	go s.listenSOCKS5()
	log.Printf("Proxy server started: HTTP on %s, SOCKS5 on %s", s.cfg.HTTPListen, s.cfg.SOCKS5Listen)
	return nil
}

// Stop gracefully shuts down the server.
func (s *Server) Stop() error {
	close(s.stopped)
	if s.httpsServer != nil {
		if err := s.httpsServer.Close(); err != nil {
			log.Printf("HTTPS server shutdown error: %v", err)
		}
	}
	return s.httpServer.Close()
}

// trackConnection records a new active connection.
func (s *Server) trackConnection(clientAddr, dest, route, protocol string) string {
	id := fmt.Sprintf("conn-%d", s.nextID.Add(1))
	conn := &status.Connection{
		ID:         id,
		ClientAddr: clientAddr,
		DestAddr:   dest,
		Route:      route,
		Protocol:   protocol,
		StartedAt:  time.Now(),
	}
	s.mu.Lock()
	s.conns[id] = conn
	s.mu.Unlock()
	s.totalConns.Add(1)
	return id
}

// untrackConnection removes a connection from tracking.
func (s *Server) untrackConnection(id string) {
	s.mu.Lock()
	delete(s.conns, id)
	s.mu.Unlock()
}

// Status returns the current server status.
func (s *Server) Status() status.ServerStatus {
	s.mu.RLock()
	activeConns := len(s.conns)
	s.mu.RUnlock()

	return status.ServerStatus{
		HTTPListen:   s.cfg.HTTPListen,
		SOCKS5Listen: s.cfg.SOCKS5Listen,
		AdminListen:  s.cfg.AdminListen,
		HTTPSListen:  s.cfg.HTTPSListen,
		ActiveConns:  activeConns,
		TotalConns:   s.totalConns.Load(),
	}
}

// Connections returns a snapshot of current active connections.
func (s *Server) Connections() []status.Connection {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]status.Connection, 0, len(s.conns))
	for _, c := range s.conns {
		result = append(result, *c)
	}
	return result
}

// handleCONNECT handles CONNECT tunneling requests for both HTTP and HTTPS proxies.
func (s *Server) handleCONNECT(w http.ResponseWriter, r *http.Request, host string, port int) {
	dest := formatDest(host, port)

	// Evaluate route for the destination
	route := s.evaluator.Evaluate(host)

	// Establish outbound connection based on route
	outbound, err := s.router.Route(dest, route)
	if err != nil {
		log.Printf("Route failed for %s: %v", dest, err)
		http.Error(w, "Service unavailable", http.StatusBadGateway)
		return
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}
	clientConn, _, err := hj.Hijack()
	if err != nil {
		log.Printf("Hijack failed: %v", err)
		http.Error(w, "Hijack failed", http.StatusInternalServerError)
		return
	}

	// Same rationale as handleSOCKS5: an opaque tunnel can only be kept warm at
	// the TCP layer.
	pac.ApplyKeepAlive(clientConn, s.cfg.TCPKeepAlive)

	// Determine protocol based on whether client connection is TLS
	protocol := "http"
	if _, isTLS := clientConn.(*tls.Conn); isTLS {
		protocol = "https"
	}

	// Send success response
	_, err = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	if err != nil {
		log.Printf("Failed to write CONNECT success response: %v", err)
		clientConn.Close()
		return
	}

	// Track the connection
	connID := s.trackConnection(clientConn.RemoteAddr().String(), dest, route.Strategy, protocol)

	// Relay data between client and server
	go s.relay(connID, clientConn, outbound)
}

// handleHTTPSConnect handles HTTPS CONNECT tunneling requests with TLS termination.
func (s *Server) handleHTTPSConnect(w http.ResponseWriter, r *http.Request, host string, port int, dest string, route pac.Route) {
	// For HTTPS, we need to establish a TLS connection with the client first
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}

	// Hijack gives us the underlying connection (already TLS-wrapped by the HTTPS listener)
	clientConn, _, err := hj.Hijack()
	if err != nil {
		log.Printf("Hijack failed: %v", err)
		http.Error(w, "Hijack failed", http.StatusInternalServerError)
		return
	}

	// The connection is already a TLS connection from the HTTPS server's listener.
	// Inspect TLS state for SNI.
	pac.ApplyKeepAlive(clientConn, s.cfg.TCPKeepAlive)
	sni := ""
	if tlsConn, ok := clientConn.(*tls.Conn); ok {
		if state := tlsConn.ConnectionState(); state.ServerName != "" {
			sni = state.ServerName
		}
	}

	// Evaluate route again with SNI information if needed
	// For now, we'll use the same route but this could be enhanced

	// Establish outbound connection based on route
	outbound, err := s.router.Route(dest, route)
	if err != nil {
		log.Printf("Route failed for %s: %v", dest, err)
		clientConn.Close()
		http.Error(w, "Service unavailable", http.StatusBadGateway)
		return
	}

	// Send success response to client over the TLS connection
	_, err = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	if err != nil {
		log.Printf("Failed to write CONNECT success response: %v", err)
		clientConn.Close()
		return
	}

	// Track the connection
	protocol := "https"
	if sni != "" {
		protocol = "https-sni:" + sni
	}
	connID := s.trackConnection(clientConn.RemoteAddr().String(), dest, route.Strategy, protocol)

	// Relay data between client and server
	go s.relay(connID, clientConn, outbound)
}

// listenSOCKS5 accepts SOCKS5 client connections.
func (s *Server) listenSOCKS5() {
	ln, err := net.Listen("tcp", s.cfg.SOCKS5Listen)
	if err != nil {
		log.Printf("SOCKS5 listen failed: %v", err)
		return
	}
	defer ln.Close()
	log.Printf("SOCKS5 listening on %s", s.cfg.SOCKS5Listen)

	for {
		select {
		case <-s.stopped:
			return
		default:
		}
		conn, err := ln.Accept()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				log.Printf("SOCKS5 accept error: %v", err)
			}
			continue
		}
		go s.handleSOCKS5(conn)
	}
}

// socks5HandshakeTimeout bounds how long a SOCKS5 client may take to complete
// the greeting, authentication and CONNECT request.
func (s *Server) socks5HandshakeTimeout() time.Duration {
	if s.cfg.SOCKS5HandshakeTimeout > 0 {
		return s.cfg.SOCKS5HandshakeTimeout
	}
	return 10 * time.Second
}

// chooseAuthMethod picks the authentication method to use from the client's
// offered list. When credentials are configured, user/pass is required and
// no-auth is refused so the listener is not an open proxy.
func (s *Server) chooseAuthMethod(offered []byte) (byte, error) {
	hasNone, hasUserPass := false, false
	for _, m := range offered {
		switch m {
		case socks5.AuthNone:
			hasNone = true
		case socks5.AuthUserPass:
			hasUserPass = true
		}
	}

	auth := s.cfg.SOCKS5Auth
	authRequired := auth != nil && (auth.Username != "" || auth.Password != "")

	if authRequired {
		if !hasUserPass {
			return 0, socks5.ErrNoAuthMethodFound
		}
		return socks5.AuthUserPass, nil
	}
	if hasNone {
		return socks5.AuthNone, nil
	}
	return 0, socks5.ErrNoAuthMethodFound
}

// authenticateUserPass runs the RFC 1929 username/password subnegotiation.
func (s *Server) authenticateUserPass(conn net.Conn) error {
	head := make([]byte, 2)
	if err := socks5.ReadFull(conn, head); err != nil {
		return fmt.Errorf("read auth header: %w", err)
	}
	if head[0] != socks5.AuthUserPassVersion {
		return socks5.ErrAuthVersion
	}

	user := make([]byte, head[1])
	if err := socks5.ReadFull(conn, user); err != nil {
		return fmt.Errorf("read username: %w", err)
	}
	plen := make([]byte, 1)
	if err := socks5.ReadFull(conn, plen); err != nil {
		return fmt.Errorf("read password length: %w", err)
	}
	pass := make([]byte, plen[0])
	if err := socks5.ReadFull(conn, pass); err != nil {
		return fmt.Errorf("read password: %w", err)
	}

	auth := s.cfg.SOCKS5Auth
	ok := auth != nil &&
		subtle.ConstantTimeCompare(user, []byte(auth.Username)) == 1 &&
		subtle.ConstantTimeCompare(pass, []byte(auth.Password)) == 1
	if !ok {
		conn.Write([]byte{socks5.AuthUserPassVersion, 0x01}) // non-zero status = failure
		return fmt.Errorf("invalid credentials")
	}
	conn.Write([]byte{socks5.AuthUserPassVersion, socks5.AuthSuccess})
	return nil
}

// handleSOCKS5 processes a SOCKS5 client connection.
func (s *Server) handleSOCKS5(conn net.Conn) {
	defer conn.Close()

	// Inbound tunnels are long-lived and opaque, so there is no protocol-level
	// way to keep them warm: TCP keepalive is the only mechanism available.
	pac.ApplyKeepAlive(conn, s.cfg.TCPKeepAlive)

	// Bound the client's handshake so a silent client cannot pin this
	// goroutine forever. The deadline is cleared as soon as the CONNECT
	// request has been read: the outbound dial is the server's own work and
	// carries its own timeout, so leaving the deadline armed across it would
	// expire it before the SOCKS5 reply could be delivered.
	conn.SetDeadline(time.Now().Add(s.socks5HandshakeTimeout()))

	// Read greeting: VER, NMETHODS, METHODS[nmethods]
	buf := make([]byte, 258) // max greeting: 2 + 256 methods
	if err := socks5.ReadFull(conn, buf[:2]); err != nil {
		return
	}
	if buf[0] != socks5.Version {
		return
	}
	nmethods := int(buf[1])
	if err := socks5.ReadFull(conn, buf[:nmethods]); err != nil {
		return
	}

	chosen, err := s.chooseAuthMethod(buf[:nmethods])
	if err != nil {
		conn.Write([]byte{socks5.Version, socks5.AuthNoAcceptable})
		log.Printf("SOCKS5 auth negotiation failed for %s: %v", conn.RemoteAddr(), err)
		return
	}
	if _, err := conn.Write([]byte{socks5.Version, chosen}); err != nil {
		return
	}
	if chosen == socks5.AuthUserPass {
		if err := s.authenticateUserPass(conn); err != nil {
			log.Printf("SOCKS5 authentication failed for %s: %v", conn.RemoteAddr(), err)
			return
		}
	}

	// Read request: VER, CMD, RSV, ATYP, ADDR, PORT
	reqBuf := make([]byte, 262) // max request: 4 + 1 + 255 + 2
	if err := socks5.ReadFull(conn, reqBuf[:4]); err != nil {
		return
	}
	if reqBuf[0] != socks5.Version {
		return
	}
	cmd := reqBuf[1]
	if cmd != socks5.CMDConnect {
		conn.Write(socks5.EncodeReply(socks5.REPCommandNotSupported))
		return
	}

	// Read address
	if err := socks5.ReadFull(conn, reqBuf[4:4+1]); err != nil {
		return
	}
	atyp := reqBuf[3]
	addrLen := 0
	switch atyp {
	case socks5.ATYPIPv4:
		addrLen = 4
	case socks5.ATYPDomain:
		addrLen = int(reqBuf[4]) + 1 // length byte + domain
	case socks5.ATYPIPv6:
		addrLen = 16
	default:
		conn.Write(socks5.EncodeReply(socks5.REPAddrTypeNotSupported))
		return
	}
	if err := socks5.ReadFull(conn, reqBuf[5:5+addrLen+2-1]); err != nil {
		return
	}

	// Use the full request buffer from offset 3 (ATYP position)
	// The buffer layout: reqBuf[3]=ATYP, reqBuf[4:]=addr+port
	host, port, _, err := socks5.DeserializeAddr(reqBuf, 3)
	if err != nil {
		conn.Write(socks5.EncodeReply(socks5.REPAddrTypeNotSupported))
		return
	}
	dest := formatDest(host, int(port))

	// The client-controlled part of the handshake is over. Clear the
	// deadline before the outbound dial: that dial has its own timeout, and
	// a slow upstream must not expire the deadline before the reply below
	// can be written.
	conn.SetDeadline(time.Time{})

	// Evaluate route and connect
	route := s.evaluator.Evaluate(host)
	outbound, err := s.router.Route(dest, route)
	if err != nil {
		log.Printf("SOCKS5 route failed for %s: %v", dest, err)
		conn.Write(socks5.EncodeReply(socks5.REPGeneralFailure))
		return
	}

	// Success
	conn.Write(socks5.EncodeReply(socks5.REPSuccess))

	// The tunnel must carry no deadline.
	conn.SetDeadline(time.Time{})

	connID := s.trackConnection(conn.RemoteAddr().String(), dest, route.Strategy, "socks5")
	s.relay(connID, conn, outbound)
}

// relay copies data between two connections bidirectionally.
func (s *Server) relay(connID string, left, right net.Conn) {
	defer s.untrackConnection(connID)
	defer left.Close()
	defer right.Close()
	errCh := make(chan error, 2)
	go func() {
		_, err := io.Copy(right, left)
		errCh <- err
	}()
	go func() {
		_, err := io.Copy(left, right)
		errCh <- err
	}()
	<-errCh
}
