package main

import (
	"fmt"
	"strings"
)

type cliOptions struct {
	subdomain     string
	token         string
	region        string
	server        string
	tlsCA         string
	tlsServerName string
	tlsSkipVerify bool
	help          bool
	showVersion   bool
}

// valueFlags maps option names to their value-taking kind.
var valueFlags = map[string]bool{
	"subdomain":       true,
	"token":           true,
	"region":          true,
	"server":          true,
	"tls-ca":          true,
	"tls-server-name": true,
}

// boolFlags maps option names that take no value (or an optional =bool).
var boolFlags = map[string]bool{
	"tls-skip-verify": true,
}

// parseArgs splits args into options and positional arguments, supporting both
// --flag=value and --flag value forms (with single or double dashes).
func parseArgs(args []string) (cliOptions, []string, error) {
	var opts cliOptions
	var positional []string

	i := 0
	for i < len(args) {
		a := args[i]
		i++

		if a == "" {
			continue
		}
		if !strings.HasPrefix(a, "-") {
			positional = append(positional, a)
			continue
		}

		rest := strings.TrimLeft(a, "-")
		if rest == "" {
			positional = append(positional, a)
			continue
		}

		name, val, hasVal := strings.Cut(rest, "=")

		switch name {
		case "help", "h":
			opts.help = true
			continue
		case "version":
			opts.showVersion = true
			continue
		}

		if boolFlags[name] {
			if hasVal {
				if !isBool(val) {
					return opts, nil, fmt.Errorf("flag --%s expects a boolean value", name)
				}
				opts.tlsSkipVerify = val == "true"
			} else {
				opts.tlsSkipVerify = true
			}
			continue
		}

		if valueFlags[name] {
			if !hasVal {
				if i >= len(args) {
					return opts, nil, fmt.Errorf("flag --%s requires a value", name)
				}
				val = args[i]
				i++
			}
			setOption(&opts, name, val)
			continue
		}

		return opts, nil, fmt.Errorf("unknown flag %q", a)
	}

	return opts, positional, nil
}

func setOption(opts *cliOptions, name, val string) {
	switch name {
	case "subdomain":
		opts.subdomain = val
	case "token":
		opts.token = val
	case "region":
		opts.region = val
	case "server":
		opts.server = val
	case "tls-ca":
		opts.tlsCA = val
	case "tls-server-name":
		opts.tlsServerName = val
	}
}

func isBool(s string) bool {
	return s == "true" || s == "false" || s == "1" || s == "0"
}
