package socks5

import (
	"io"
	"net"
	"testing"
	"time"
)

// startSimpleSocks5Server starts a SOCKS5 server for testing handshake only.
// If authRequired is false, it allows no authentication.
// If authRequired is true, it requires username/password and checks them using checkCreds.
func startSimpleSocks5Server(t *testing.T, authRequired bool, checkCreds func(user, pass string) bool) (addr string, stop func()) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				// Greeting from client: VER, NMETHODS, METHODS...
				hdr := make([]byte, 2)
				if _, err := io.ReadFull(c, hdr); err != nil {
					return
				}
				nmethods := hdr[1]
				methods := make([]byte, nmethods)
				if _, err := io.ReadFull(c, methods); err != nil {
					return
				}

				// Choose method
				var chosen byte
				if authRequired {
					chosen = AuthNoAcceptable
					for _, m := range methods {
						if m == AuthUserPass {
							chosen = AuthUserPass
							break
						}
					}
					if chosen == AuthNoAcceptable {
						// No acceptable method, respond with no acceptable and return
						c.Write([]byte{Version, AuthNoAcceptable})
						return
					}
				} else {
					chosen = AuthNone
					for _, m := range methods {
						if m == AuthNone {
							chosen = AuthNone
							break
						}
					}
				}

				// Send chosen method
				if _, err := c.Write([]byte{Version, chosen}); err != nil {
					return
				}

				if chosen == AuthUserPass {
					// Read credentials: VER, ULEN, UNAME, PLEN, PASSWD
					ver := make([]byte, 1)
					if _, err := io.ReadFull(c, ver); err != nil {
						return
					}
					ulenB := make([]byte, 1)
					if _, err := io.ReadFull(c, ulenB); err != nil {
						return
					}
					ulen := ulenB[0]
					uname := make([]byte, ulen)
					if _, err := io.ReadFull(c, uname); err != nil {
						return
					}
					plenB := make([]byte, 1)
					if _, err := io.ReadFull(c, plenB); err != nil {
						return
					}
					plen := plenB[0]
					passwd := make([]byte, plen)
					if _, err := io.ReadFull(c, passwd); err != nil {
						return
					}
					// Validate
					ok := checkCreds(string(uname), string(passwd))
					var resp byte
					if ok {
						resp = REPSuccess
					} else {
						resp = 1 // failure
					}
					c.Write([]byte{Version, resp})
					return
				}
				// No auth handshake complete, return.
				return
			}(conn)
		}
	}()
	return l.Addr().String(), func() { l.Close() }
}

func TestDialWithAuth_NoAuth(t *testing.T) {
	addr, stop := startSimpleSocks5Server(t, false, nil)
	defer stop()
	conn, err := DialWithAuth("tcp", addr, nil, 5*time.Second)
	if err != nil {
		t.Fatalf("DialWithAuth failed: %v", err)
	}
	conn.Close()
}

func TestDialWithAuth_UserPassSuccess(t *testing.T) {
	addr, stop := startSimpleSocks5Server(t, true, func(user, pass string) bool {
		return user == "testuser" && pass == "testpass"
	})
	defer stop()
	auth := &Auth{Username: "testuser", Password: "testpass"}
	conn, err := DialWithAuth("tcp", addr, auth, 5*time.Second)
	if err != nil {
		t.Fatalf("DialWithAuth failed: %v", err)
	}
	conn.Close()
}

func TestDialWithAuth_UserPassFailure(t *testing.T) {
	addr, stop := startSimpleSocks5Server(t, true, func(user, pass string) bool {
		// accept only correct credentials
		return user == "testuser" && pass == "correctpass"
	})
	defer stop()
	auth := &Auth{Username: "testuser", Password: "wrongpass"}
	conn, err := DialWithAuth("tcp", addr, auth, 5*time.Second)
	if err == nil {
		t.Fatalf("expected auth failure, got no error")
	}
	if conn != nil {
		conn.Close()
		t.Error("expected nil conn on auth failure, got non-nil")
	}
}
