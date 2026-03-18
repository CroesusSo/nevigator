# Release Note

Fix IPv6 destination handling for CONNECT and SOCKS5.

The proxy now correctly routes to IPv6 destinations by properly formatting bracketed addresses when using HTTP CONNECT or SOCKS5 upstream connections.
