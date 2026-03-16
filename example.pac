function FindProxyForURL(url, host) {
  // Route internal traffic via TUN
  if (dnsDomainIs(host, ".internal.example.com")) {
    return "TUN";
  }
  // Route Tor hidden services via SOCKS5
  if (shExpMatch(host, "*.onion")) {
    return "SOCKS5 127.0.0.1:9050";
  }
  // Everything else uses system default route
  return "DIRECT";
}
