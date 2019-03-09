package main

import (
	"fmt"

	"flag"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/coredns/coredns/coremain"
	_ "github.com/coredns/coredns/plugin/bind"
	_ "github.com/coredns/coredns/plugin/cache"
	_ "github.com/coredns/coredns/plugin/errors"
	_ "github.com/coredns/coredns/plugin/forward"
	_ "github.com/coredns/coredns/plugin/health"
	_ "github.com/coredns/coredns/plugin/log"
	_ "github.com/coredns/coredns/plugin/loop"
	_ "github.com/coredns/coredns/plugin/metrics"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	_ "github.com/coredns/coredns/plugin/reload"
	"github.com/mholt/caddy"
	"k8s.io/dns/pkg/netif"
	"k8s.io/kubernetes/pkg/util/dbus"
	utilexec "k8s.io/kubernetes/pkg/util/exec"
	utiliptables "k8s.io/kubernetes/pkg/util/iptables"
)

// configParams lists the configuration options that can be provided to dns-cache
type configParams struct {
	localIP            string        // ip address for the local cache agent to listen for dns requests
	localPort          string        // port to listen for dns requests
	intfName           string        // Name of the interface to be created
	interval           time.Duration // specifies how often to run iptables rules check
	maxIptablesLockErr int           // Max consecutive iptables lock errors to ignore before restarting
	exitChan           chan bool     // Channel to terminate background goroutines
}

type iptablesRule struct {
	table utiliptables.Table
	chain utiliptables.Chain
	args  []string
}

type cacheApp struct {
	iptables      utiliptables.Interface
	iptablesRules []iptablesRule
	params        configParams
	netifHandle   *netif.NetifManager
}

var cache = cacheApp{params: configParams{localPort: "53"}}

func IsIPTablesLocked(err error) bool {
	return strings.Contains(err.Error(), "holding the xtables lock")
}

func (c *cacheApp) Init() {
	err := c.parseAndValidateFlags()
	if err != nil {
		clog.Fatalf("Error parsing flags - %s, Exiting", err)
	}
	c.netifHandle = netif.NewNetifManager(net.ParseIP(c.params.localIP))
	c.initIptables()
	err = c.teardownNetworking()
	if err != nil {
		// It is likely to hit errors here if previous shutdown cleaned up all iptables rules and interface.
		// Logging error at info level
		clog.Infof("Hit error during teardown - %s", err)
	}
	err = c.setupNetworking()
	if err != nil {
		cache.teardownNetworking()
		clog.Fatalf("Failed to setup - %s, Exiting", err)
	}
}

func init() {
	cache.Init()
	caddy.OnProcessExit = append(caddy.OnProcessExit, func() { cache.teardownNetworking() })
}

func (c *cacheApp) initIptables() {

	c.iptablesRules = []iptablesRule{
		// Match traffic destined for localIp:localPort and set the flows to be NOTRACKED, this skips connection tracking
		{utiliptables.Table("raw"), utiliptables.ChainPrerouting, []string{"-p", "tcp", "-d", c.params.localIP,
			"--dport", c.params.localPort, "-j", "NOTRACK"}},
		{utiliptables.Table("raw"), utiliptables.ChainPrerouting, []string{"-p", "udp", "-d", c.params.localIP,
			"--dport", c.params.localPort, "-j", "NOTRACK"}},
		// There are rules in filter table to allow tracked connections to be accepted. Since we skipped connection tracking,
		// need these additional filter table rules.
		{utiliptables.TableFilter, utiliptables.ChainInput, []string{"-p", "tcp", "-d", c.params.localIP,
			"--dport", c.params.localPort, "-j", "ACCEPT"}},
		{utiliptables.TableFilter, utiliptables.ChainInput, []string{"-p", "udp", "-d", c.params.localIP,
			"--dport", c.params.localPort, "-j", "ACCEPT"}},
		// Match traffic from localIp:localPort and set the flows to be NOTRACKED, this skips connection tracking
		{utiliptables.Table("raw"), utiliptables.ChainOutput, []string{"-p", "tcp", "-s", c.params.localIP,
			"--sport", c.params.localPort, "-j", "NOTRACK"}},
		{utiliptables.Table("raw"), utiliptables.ChainOutput, []string{"-p", "udp", "-s", c.params.localIP,
			"--sport", c.params.localPort, "-j", "NOTRACK"}},
		// Additional filter table rules for traffic frpm localIp:localPort
		{utiliptables.TableFilter, utiliptables.ChainOutput, []string{"-p", "tcp", "-s", c.params.localIP,
			"--sport", c.params.localPort, "-j", "ACCEPT"}},
		{utiliptables.TableFilter, utiliptables.ChainOutput, []string{"-p", "udp", "-s", c.params.localIP,
			"--sport", c.params.localPort, "-j", "ACCEPT"}},
	}
	c.iptables = newIPTables()
}

func newIPTables() utiliptables.Interface {
	execer := utilexec.New()
	dbus := dbus.New()
	return utiliptables.New(execer, dbus, utiliptables.ProtocolIpv4)
}

func (c *cacheApp) setupNetworking() error {
	var err error
	clog.Infof("Setting up networking for node cache")
	err = c.netifHandle.AddDummyDevice(c.params.intfName)
	if err != nil {
		return err
	}
	for _, rule := range c.iptablesRules {
		_, err = c.iptables.EnsureRule(utiliptables.Prepend, rule.table, rule.chain, rule.args...)
		if err != nil {
			return err
		}
	}
	return err
}

func (c *cacheApp) teardownNetworking() error {
	clog.Infof("Tearing down")
	if c.params.exitChan != nil {
		// Stop the goroutine that periodically checks for iptables rules/dummy interface
		// exitChan is a buffered channel of size 1, so this will not block
		c.params.exitChan <- true
	}
	err := c.netifHandle.RemoveDummyDevice(c.params.intfName)
	for _, rule := range c.iptablesRules {
		exists := true
		for exists == true {
			c.iptables.DeleteRule(rule.table, rule.chain, rule.args...)
			exists, _ = c.iptables.EnsureRule(utiliptables.Prepend, rule.table, rule.chain, rule.args...)
		}
		// Delete the rule one last time since EnsureRule creates the rule if it doesn't exist
		c.iptables.DeleteRule(rule.table, rule.chain, rule.args...)
	}
	return err
}

func (c *cacheApp) parseAndValidateFlags() error {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Runs coreDNS v1.2.5 as a nodelocal cache listening on the specified ip:port")
		flag.PrintDefaults()
	}

	flag.StringVar(&c.params.localIP, "localip", "", "ip address to bind dnscache to")
	flag.StringVar(&c.params.intfName, "intfname", "nodelocaldns", "name of the interface to be created")
	flag.DurationVar(&c.params.interval, "syncinterval", 60, "interval(in seconds) to check for iptables rules")
	flag.IntVar(&c.params.maxIptablesLockErr, "maxiptableslockerr", 10, "Max consecutive iptables lock errors to ignore before restarting")
	flag.Parse()

	if net.ParseIP(c.params.localIP) == nil {
		return fmt.Errorf("Invalid localip specified - %q", c.params.localIP)
	}
	// lookup specified dns port
	if f := flag.Lookup("dns.port"); f == nil {
		return fmt.Errorf("Failed to lookup \"dns.port\" parameter")
	} else {
		c.params.localPort = f.Value.String()
	}
	if _, err := strconv.Atoi(c.params.localPort); err != nil {
		return fmt.Errorf("Invalid port specified - %q", c.params.localPort)
	}
	if c.params.maxIptablesLockErr < 0 {
		return fmt.Errorf("Invalid value for maxIptablesLockErr - %d", c.params.maxIptablesLockErr)
	}
	return nil
}

// returns true if checks failed due to iptables lock issue
func (c *cacheApp) runChecks() bool {
	iptablesLocked := false
	for _, rule := range c.iptablesRules {
		exists, err := c.iptables.EnsureRule(utiliptables.Prepend, rule.table, rule.chain, rule.args...)
		if exists {
			continue
		} else if err == nil {
			clog.Infof("Added back nonexistent rule - %v", rule)
			continue
		}
		// if we got here, either iptables check failed or adding rule back failed.
		if IsIPTablesLocked(err) {
			clog.Infof("Failed to check/add back rule %v, due to xtables lock in use, retrying in %v seconds", rule, c.params.interval)
			iptablesLocked = true
		} else {
			cache.teardownNetworking()
			clog.Fatalf("Failed to add back non-existent rule %v - %s", rule, err)
		}
	}

	exists, err := c.netifHandle.EnsureDummyDevice(c.params.intfName)
	if !exists {
		if err != nil {
			cache.teardownNetworking()
			clog.Fatalf("Failed to add back non-existent interface %s", c.params.intfName)
		}
		clog.Infof("Added back nonexistent interface - %s", c.params.intfName)
	}
	if err != nil {
		clog.Errorf("Failed to check dummy device %s - %s", c.params.intfName, err)
	}
	return iptablesLocked
}

func (c *cacheApp) run() {
	c.params.exitChan = make(chan bool, 1)
	tick := time.NewTicker(c.params.interval * time.Second)
	numLockErr := 0
	iptablesLocked := false
	for {
		select {
		case <-tick.C:
			iptablesLocked = c.runChecks()
			if iptablesLocked {
				numLockErr++
			} else {
				numLockErr = 0
			}
			if numLockErr > c.params.maxIptablesLockErr {
				cache.teardownNetworking()
				clog.Fatalf("Too many consecutive failures when running iptables checks")
			}
		case <-c.params.exitChan:
			clog.Warningf("Exiting iptables check goroutine")
			return
		}
	}
}

func main() {
	// Ensure that the required setup is ready
	// https://github.com/kubernetes/dns/issues/282 sometimes the interface gets the ip and then loses it, if added too soon.
	cache.runChecks()
	go cache.run()
	coremain.Run()
	// Unlikely to reach here, if we did it is because coremain exited and the signal was not trapped.
	clog.Errorf("Untrapped signal, tearing down")
	cache.teardownNetworking()
}
