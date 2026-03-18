## ADDED Requirements

### Requirement: CONNECT requests are processed by the proxy
The proxy server SHALL correctly route CONNECT requests to the CONNECT handler, ensuring that HTTPS (and other protocols using CONNECT) can be proxied without receiving a 404 response.

#### Scenario: CONNECT request succeeds
- **WHEN** a client sends a CONNECT request to `example.com:443`
- **THEN** the proxy SHALL respond with status `200 Connection established` and begin tunneling traffic

#### Scenario: CONNECT request fails due to routing
- **WHEN** a client sends a CONNECT request to a destination that cannot be routed
- **THEN** the proxy SHALL return an appropriate error status (e.g., 502) rather than 404
- **AND** the response SHALL indicate the routing failure reason
