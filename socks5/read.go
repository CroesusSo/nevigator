package socks5

import "net"

// ReadFull reads exactly len(buf) bytes from conn, handling short reads.
func ReadFull(conn net.Conn, buf []byte) error {
	n := 0
	for n < len(buf) {
		read, err := conn.Read(buf[n:])
		if err != nil {
			return err
		}
		n += read
	}
	return nil
}
