# Design: Fix HTTPS CONNECT Handler for Proper Tunneling

## Problem
The `handleHTTPSConnect` function in `proxy/server.go` was incorrectly implementing TLS termination with the client for HTTPS CONNECT requests. Instead of establishing a TCP tunnel (as expected for CONNECT method), it was:
1. Performing TLS handshake with the client
2. Extracting SNI from the established TLS connection
3. Using that TLS connection for relaying data

This caused clients attempting to use HTTPS CONNECT (e.g., for accessing HTTPS websites through the proxy) to receive TLS handshake data when they expected plain TCP tunneling, leading to OpenSSL errors like "wrong version number".

According to RFC 7231, the CONNECT method is used to establish a tunnel to a server identified by the target resource. For HTTPS, the client sends:
```
CONNECT www.example.com:443 HTTP/1.1
Host: www.example.com:443
```

The proxy should then:
1. Establish a TCP connection to the target (www.example.com:443)
2. Respond with HTTP/1.1 200 Connection Established
3. Relay TCP data bidirectionally between client and target without modifying the data

## Solution
Refactor `handleHTTPSConnect` to implement proper HTTPS CONNECT tunneling:
1. Keep the HTTP hijacking to get raw TCP connection from client
2. Do NOT perform TLS handshake with client (the client will handle TLS internally)
3. Extract SNI from the ClientHello message for routing decisions (optional enhancement)
4. Establish outbound TCP connection to target using the router
5. Send "HTTP/1.1 200 Connection Established" to client
6. Relay data bidirectionally between client and outbound connections

## Implementation Details

### Before (Incorrect)
```go
func (s *Server) handleHTTPSConnect(w http.ResponseWriter, r *http.Request, host string, port int, dest string, route pac.Route) {
	// For HTTPS, we need to establish a TLS connection with the client first
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

	// Create TLS connection with client
	tlsConn := tls.Server(clientConn, s.tlsConfig)
	if err := tlsConn.Handshake(); err != nil {
		log.Printf("TLS handshake failed: %v", err)
		clientConn.Close()
		http.Error(w, "TLS handshake failed", http.StatusInternalServerError)
		return
	}

	// Extract SNI from the TLS connection for routing decisions
	sni := ""
	if tlsConn.ConnectionState().ServerName != "" {
		sni = tlsConn.ConnectionState().ServerName
	}

	// ... establish outbound connection using tlsConn for relay
}
```

### After (Correct)
```go
func (s *Server) handleHTTPSConnect(w http.ResponseWriter, r *http.Request, host string, port int, dest string, route pac.Route) {
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

	// Extract SNI from ClientHello for routing (optional but useful)
	sni := extractSNIFromClientHello(clientConn)
	if sni != "" {
		// Optionally use SNI for enhanced routing decisions
		// For now, we keep the original host-based routing but could enhance
	}

	// Establish outbound connection based on route (using original dest)
	outbound, err := s.router.Route(dest, route)
	if err != nil {
		log.Printf("Route failed for %s: %v", dest, err)
		clientConn.Close()
		http.Error(w, "Service unavailable", http.StatusBadGateway)
		return
	}

	// Send success response to client
	clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

	// Track the connection
	protocol := "https"
	if sni != "" {
		protocol = "https-sni:" + sni
	}
	connID := s.trackConnection(clientConn.RemoteAddr().String(), dest, route.Strategy, protocol)

	// Relay data between client and outbound connection
	go s.relay(connID, clientConn, outbound)
}
```

Where `extractSNIFromClientHello` reads the initial ClientHello message from the client to extract SNI without completing the TLS handshake.

## Files Modified
- `proxy/server.go`: Refactored `handleHTTPSConnect` function

## Testing Approach
1. Manual testing with `curl -x localhost:8080 https://example.com`
2. Verify no "wrong version number" SSL errors occur
3. Confirm that HTTP CONNECT still works for non-HTTPS targets
4. Check that connection tracking and relay functions correctly

## Diagram
```
Before Fix (Incorrect TLS Termination):
Client --> [CONNECT to proxy:8080] --> [Proxy does TLS handshake with client] 
                                  --> [Proxy establishes outbound connection]
                                  --> [Relays data between client TLS and outbound TCP]
                                  
                                   ↓
                     Client receives unexpected TLS handshake when expecting TCP tunnel
                     → OpenSSL error: wrong version number

After Fix (Proper TCP Tunneling):
Client --> [CONNECT to proxy:8080] --> [Proxy hijacks connection, gets raw TCP]
                                  --> [Extracts SNI from ClientHello (optional)]
                                  --> [Proxy establishes outbound TCP connection]
                                  --> [Sends "200 Connection Established"]
                                  --> [Relays raw TCP data bidirectionally]
                                  
                                   ↓
                     Client establishes TLS connection directly with target server
                     → HTTPS connection works correctly
```