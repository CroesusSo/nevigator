package pac

import "strings"

// parsePACResult parses the return value from FindProxyForURL.
// It returns only the first route since we don't support chains.
func parsePACResult(s string) Route {
	s = strings.TrimSpace(s)
	if s == "" {
		return Route{Strategy: "DIRECT"}
	}

	tokens := strings.Fields(s)
	if len(tokens) == 0 {
		return Route{Strategy: "DIRECT"}
	}

	switch strings.ToUpper(tokens[0]) {
	case "SOCKS5":
		if len(tokens) >= 2 {
			return Route{Strategy: "SOCKS5", Address: tokens[1]}
		}
		return Route{Strategy: "DIRECT"}
	case "EXTERNAL":
		if len(tokens) >= 2 {
			return Route{Strategy: "EXTERNAL", ExternalID: tokens[1]}
		}
		return Route{Strategy: "DIRECT"}
	case "DEFAULT":
		return Route{Strategy: "DEFAULT"}
	case "DIRECT":
		return Route{Strategy: "DIRECT"}
	default:
		// Unknown strategy defaults to DIRECT
		return Route{Strategy: "DIRECT"}
	}
}
