package proxy

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
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
	cfg        pac.ProxyConfig
	evaluator  *pac.Evaluator
	router     *router.Router
	httpServer *http.Server
	socks5Conn chan net.Conn
	stopped    chan struct{}

	mu         sync.RWMutex
	conns      map[string]*status.Connection
	totalConns atomic.Int64
	nextID     atomic.Int64
	tunDevice  string
	tunIP      net.IP
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
		tunDevice:  cfg.TUNDevice,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleHTTPConnect)
	s.httpServer = &http.Server{
		Addr:    cfg.HTTPListen,
		Handler: mux,
	}
	return s
}

// SetTUNIP sets the TUN IP address (called from main after discovery).
func (s *Server) SetTUNIP(ip net.IP) {
	s.mu.Lock()
	s.tunIP = ip
	s.mu.Unlock()
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
	tunIP := s.tunIP
	tunDevice := s.tunDevice
	activeConns := len(s.conns)
	s.mu.RUnlock()

	tunUp := false
	tunIPStr := ""
	if tunIP != nil {
		tunIPStr = tunIP.String()
		// Re-check interface is still up
		iface, err := net.InterfaceByName(tunDevice)
		if err == nil {
			addrs, err := iface.Addrs()
			if err == nil {
				for _, addr := range addrs {
					if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil {
						tunUp = true
						break
					}
				}
			}
		}
	}

	return status.ServerStatus{
		HTTPListen:   s.cfg.HTTPListen,
		SOCKS5Listen: s.cfg.SOCKS5Listen,
		AdminListen:  s.cfg.AdminListen,
		TUNDevice:    tunDevice,
		TUNIP:        tunIPStr,
		TUNUp:        tunUp,
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

// handleHTTPConnect handles HTTP CONNECT tunneling requests.
func (s *Server) handleHTTPConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != "CONNECT" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	host, portStr, err := net.SplitHostPort(r.URL.Path)
	if err != nil {
		http.Error(w, "Invalid CONNECT path", http.StatusBadRequest)
		return
	}
	port, _ := strconv.Atoi(portStr)
	dest := fmt.Sprintf("%s:%d", host, port)

	route := s.evaluator.Evaluate(host)
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
	clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

	connID := s.trackConnection(clientConn.RemoteAddr().String(), dest, route.Strategy, "http")
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

// handleSOCKS5 processes a SOCKS5 client connection.
func (s *Server) handleSOCKS5(conn net.Conn) {
	defer conn.Close()

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

	// Choose no-auth if available
	var chosen byte = socks5.AuthNoAcceptable
	for _, m := range buf[:nmethods] {
		if m == socks5.AuthNone {
			chosen = socks5.AuthNone
			break
		}
	}
	conn.Write([]byte{socks5.Version, chosen})
	if chosen != socks5.AuthNone {
		return
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
	dest := fmt.Sprintf("%s:%d", host, port)

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
