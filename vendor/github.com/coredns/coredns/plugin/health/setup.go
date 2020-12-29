package health

import (
	"fmt"
	"net"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin"
)

func init() { plugin.Register("health", setup) }

func setup(c *caddy.Controller) error {
	addr, lame, err := parse(c)
	if err != nil {
		return plugin.Error("health", err)
	}

	h := &health{Addr: addr, stop: make(chan bool), lameduck: lame}

	c.OnStartup(h.OnStartup)
	c.OnRestart(h.OnFinalShutdown)
	c.OnFinalShutdown(h.OnFinalShutdown)
	c.OnRestartFailed(h.OnStartup)

	// Don't do AddPlugin, as health is not *really* a plugin just a separate webserver running.
	return nil
}

func parse(c *caddy.Controller) (string, time.Duration, error) {
	addr := ""
	dur := time.Duration(0)
	for c.Next() {
		args := c.RemainingArgs()

		switch len(args) {
		case 0:
		case 1:
			addr = args[0]
			if _, _, e := net.SplitHostPort(addr); e != nil {
				return "", 0, e
			}
		default:
			return "", 0, c.ArgErr()
		}

		for c.NextBlock() {
			switch c.Val() {
			case "lameduck":
				args := c.RemainingArgs()
				if len(args) != 1 {
					return "", 0, c.ArgErr()
				}
				l, err := time.ParseDuration(args[0])
				if err != nil {
					return "", 0, fmt.Errorf("unable to parse lameduck duration value: '%v' : %v", args[0], err)
				}
				dur = l
			default:
				return "", 0, c.ArgErr()
			}
		}
	}
	return addr, dur, nil
}
