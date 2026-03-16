package pac

import (
	"context"
	"log"
	"net"
	"regexp"
	"strings"

	"github.com/dop251/goja"
)

func registerBuiltins(vm *goja.Runtime, cfg EvaluatorConfig, cache *dnsCache) {
	// shExpMatch
	vm.Set("shExpMatch", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return vm.ToValue(goja.Undefined())
		}
		str := call.Argument(0).String()
		pattern := call.Argument(1).String()
		matched, _ := regexp.MatchString(shellPatternToRegex(pattern), str)
		return vm.ToValue(matched)
	})

	// dnsDomainIs
	vm.Set("dnsDomainIs", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return vm.ToValue(goja.Undefined())
		}
		host := strings.ToLower(call.Argument(0).String())
		pattern := strings.ToLower(call.Argument(1).String())
		if strings.HasSuffix(host, pattern) {
			return vm.ToValue(true)
		}
		return vm.ToValue(false)
	})

	// isInNet
	vm.Set("isInNet", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 3 {
			return vm.ToValue(goja.Undefined())
		}
		ipStr := call.Argument(0).String()
		pattern := call.Argument(1).String()
		mask := call.Argument(2).String()
		ip := net.ParseIP(ipStr)
		if ip == nil {
			return vm.ToValue(false)
		}
		_, netIP, err := net.ParseCIDR(pattern + "/" + mask)
		if err != nil {
			_, netIP, err = net.ParseCIDR(pattern)
			if err != nil {
				return vm.ToValue(false)
			}
		}
		if netIP.Contains(ip) {
			return vm.ToValue(true)
		}
		return vm.ToValue(false)
	})

	// myIpAddress
	vm.Set("myIpAddress", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(cfg.LocalIP)
	})

	// dnsResolve
	vm.Set("dnsResolve", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return vm.ToValue(goja.Undefined())
		}
		host := call.Argument(0).String()
		if cache != nil {
			if addrs, ok := cache.get(host); ok && len(addrs) > 0 {
				return vm.ToValue(addrs[0])
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), cfg.DNSTimeout)
		defer cancel()
		addrs, err := lookupHost(ctx, host)
		if err != nil || len(addrs) == 0 {
			return vm.ToValue(goja.Undefined())
		}
		if cache != nil {
			cache.set(host, addrs)
		}
		return vm.ToValue(addrs[0])
	})

	// isResolvable
	vm.Set("isResolvable", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return vm.ToValue(false)
		}
		host := call.Argument(0).String()
		if cache != nil {
			if _, ok := cache.get(host); ok {
				return vm.ToValue(true)
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), cfg.DNSTimeout)
		defer cancel()
		addrs, err := lookupHost(ctx, host)
		if err != nil || len(addrs) == 0 {
			return vm.ToValue(false)
		}
		return vm.ToValue(true)
	})

	// alert
	vm.Set("alert", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			log.Printf("[PAC alert] %s", call.Argument(0).String())
		}
		return vm.ToValue(goja.Undefined())
	})

	// console.log
	vm.Set("console", map[string]interface{}{
		"log": func(call goja.FunctionCall) goja.Value {
			args := make([]interface{}, len(call.Arguments))
			for i, a := range call.Arguments {
				args[i] = a.Export()
			}
			log.Printf("[PAC console] %v", args)
			return vm.ToValue(goja.Undefined())
		},
	})
}

func shellPatternToRegex(pattern string) string {
	var sb strings.Builder
	sb.WriteByte('^')
	for _, r := range pattern {
		switch r {
		case '*':
			sb.WriteString(".*")
		case '?':
			sb.WriteByte('.')
		case '.', '+', '^', '$', '|', '(', ')', '[', ']', '{', '}', '\\':
			sb.WriteByte('\\')
			sb.WriteRune(r)
		default:
			sb.WriteRune(r)
		}
	}
	sb.WriteByte('$')
	return sb.String()
}
