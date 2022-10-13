package pprof

import (
	"net"
	"strconv"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
)

var log = clog.NewWithPlugin("pprof")

const defaultAddr = "localhost:6053"

func init() { plugin.Register("pprof", setup) }

func setup(c *caddy.Controller) error {
	h := &handler{addr: defaultAddr}

	i := 0
	for c.Next() {
		if i > 0 {
			return plugin.Error("pprof", plugin.ErrOnce)
		}
		i++

		args := c.RemainingArgs()
		if len(args) == 1 {
			h.addr = args[0]
			_, _, e := net.SplitHostPort(h.addr)
			if e != nil {
				return plugin.Error("pprof", c.Errf("%v", e))
			}
		}

		if len(args) > 1 {
			return plugin.Error("pprof", c.ArgErr())
		}

		for c.NextBlock() {
			switch c.Val() {
			case "block":
				args := c.RemainingArgs()
				if len(args) > 1 {
					return plugin.Error("pprof", c.ArgErr())
				}
				h.rateBloc = 1
				if len(args) > 0 {
					t, err := strconv.Atoi(args[0])
					if err != nil {
						return plugin.Error("pprof", c.Errf("property '%s' invalid integer value '%v'", "block", args[0]))
					}
					h.rateBloc = t
				}
			default:
				return plugin.Error("pprof", c.Errf("unknown property '%s'", c.Val()))
			}
		}

	}

	c.OnStartup(h.Startup)
	c.OnShutdown(h.Shutdown)
	return nil
}
