package app

import (
	"net"
	"os"
	"strings"
	"time"

	"github.com/coredns/coredns/coremain"
	clog "github.com/coredns/coredns/plugin/pkg/log"

	"k8s.io/dns/cmd/kube-dns/app/options"
	"k8s.io/dns/pkg/dns/config"
	"k8s.io/dns/pkg/netif"
	"k8s.io/kubernetes/pkg/util/dbus"
	utiliptables "k8s.io/kubernetes/pkg/util/iptables"
	utilexec "k8s.io/utils/exec"
	utilnet "k8s.io/utils/net"
)

// ConfigParams lists the configuration options that can be provided to node-cache
type ConfigParams struct {
	LocalIPStr           string        // comma separated listen ips for the local cache agent
	LocalIPs             []net.IP      // parsed ip addresses for the local cache agent to listen for dns requests
	LocalPort            string        // port to listen for dns requests
	MetricsListenAddress string        // address to serve metrics on
	SetupInterface       bool          // Indicates whether to setup network interface
	InterfaceName        string        // Name of the interface to be created
	Interval             time.Duration // specifies how often to run iptables rules check
	Pidfile              string        // Path to the coredns server pidfile
	BaseCoreFile         string        // Path to the template config file for node-cache
	CoreFile             string        // Path to config file used by node-cache
	KubednsCMPath        string        // Directory where kube-dns configmap will be mounted
	UpstreamSvcName      string        // Name of the service whose clusterIP is the upstream for node-cache for cluster domain
	HealthPort           string        // port for the healthcheck
	SetupIptables        bool
	SkipTeardown         bool // Indicates whether the iptables rules and interface should be torn down
}

type iptablesRule struct {
	table utiliptables.Table
	chain utiliptables.Chain
	args  []string
}

// CacheApp contains all the config required to run node-cache.
type CacheApp struct {
	iptables      utiliptables.Interface
	iptablesRules []iptablesRule
	params        *ConfigParams
	netifHandle   *netif.NetifManager
	kubednsConfig *options.KubeDNSConfig
	exitChan      chan struct{} // Channel to terminate background goroutines
	clusterDNSIP  net.IP
}

func isLockedErr(err error) bool {
	return strings.Contains(err.Error(), "holding the xtables lock")
}

// Init initializes the parameters and networking setup necessary to run node-cache
func (c *CacheApp) Init() {
	if c.params.SetupInterface {
		c.netifHandle = netif.NewNetifManager(c.params.LocalIPs)
	}
	if c.params.SetupIptables {
		c.initIptables()
	}
	initMetrics(c.params.MetricsListenAddress)
	// Write the config file from template.
	// this is required in case there is no or erroneous kube-dns configpath specified.
	c.updateCorefile(&config.Config{})
	// Initialize periodic sync for node-local-dns, kube-dns configmap.
	c.initDNSConfigSync()
	// Setup only the network interface during this init. IPTables will be setup via runPeriodic.
	// This is to ensure that iptables rules don't get setup if the cache(coreDNS) is unable to startup due to config
	// error, port conflicts or other reasons.
	setupIptables := c.params.SetupIptables
	c.params.SetupIptables = false
	c.setupNetworking()
	c.params.SetupIptables = setupIptables
}

// isIPv6 return if the node-cache is working in IPv6 mode
// LocalIPs are guaranteed to have the same family
func (c *CacheApp) isIPv6() bool {
	if len(c.params.LocalIPs) > 0 {
		return utilnet.IsIPv6(c.params.LocalIPs[0])
	}
	return false
}

func (c *CacheApp) initIptables() {
	// using the localIPStr param since we need ip strings here
	for _, localIP := range strings.Split(c.params.LocalIPStr, ",") {
		c.iptablesRules = append(c.iptablesRules, []iptablesRule{
			// Match traffic destined for localIp:localPort and set the flows to be NOTRACKED, this skips connection tracking
			{utiliptables.Table("raw"), utiliptables.ChainPrerouting, []string{"-p", "tcp", "-d", localIP,
				"--dport", c.params.LocalPort, "-j", "NOTRACK"}},
			{utiliptables.Table("raw"), utiliptables.ChainPrerouting, []string{"-p", "udp", "-d", localIP,
				"--dport", c.params.LocalPort, "-j", "NOTRACK"}},
			// There are rules in filter table to allow tracked connections to be accepted. Since we skipped connection tracking,
			// need these additional filter table rules.
			{utiliptables.TableFilter, utiliptables.ChainInput, []string{"-p", "tcp", "-d", localIP,
				"--dport", c.params.LocalPort, "-j", "ACCEPT"}},
			{utiliptables.TableFilter, utiliptables.ChainInput, []string{"-p", "udp", "-d", localIP,
				"--dport", c.params.LocalPort, "-j", "ACCEPT"}},
			// Match traffic from localIp:localPort and set the flows to be NOTRACKED, this skips connection tracking
			{utiliptables.Table("raw"), utiliptables.ChainOutput, []string{"-p", "tcp", "-s", localIP,
				"--sport", c.params.LocalPort, "-j", "NOTRACK"}},
			{utiliptables.Table("raw"), utiliptables.ChainOutput, []string{"-p", "udp", "-s", localIP,
				"--sport", c.params.LocalPort, "-j", "NOTRACK"}},
			// Additional filter table rules for traffic frpm localIp:localPort
			{utiliptables.TableFilter, utiliptables.ChainOutput, []string{"-p", "tcp", "-s", localIP,
				"--sport", c.params.LocalPort, "-j", "ACCEPT"}},
			{utiliptables.TableFilter, utiliptables.ChainOutput, []string{"-p", "udp", "-s", localIP,
				"--sport", c.params.LocalPort, "-j", "ACCEPT"}},
			// Skip connection tracking for requests to nodelocalDNS that are locally generated, example - by hostNetwork pods
			{utiliptables.Table("raw"), utiliptables.ChainOutput, []string{"-p", "tcp", "-d", localIP,
				"--dport", c.params.LocalPort, "-j", "NOTRACK"}},
			{utiliptables.Table("raw"), utiliptables.ChainOutput, []string{"-p", "udp", "-d", localIP,
				"--dport", c.params.LocalPort, "-j", "NOTRACK"}},
			// skip connection tracking for healthcheck requests generated by liveness probe to health plugin
			{utiliptables.Table("raw"), utiliptables.ChainOutput, []string{"-p", "tcp", "-d", localIP,
				"--dport", c.params.HealthPort, "-j", "NOTRACK"}},
			{utiliptables.Table("raw"), utiliptables.ChainOutput, []string{"-p", "tcp", "-s", localIP,
				"--sport", c.params.HealthPort, "-j", "NOTRACK"}},
		}...)
	}
	c.iptables = newIPTables(c.isIPv6())
}

func newIPTables(isIPv6 bool) utiliptables.Interface {
	execer := utilexec.New()
	dbus := dbus.New()
	protocol := utiliptables.ProtocolIpv4
	if isIPv6 {
		protocol = utiliptables.ProtocolIpv6
	}
	return utiliptables.New(execer, dbus, protocol)
}

// TeardownNetworking removes all custom iptables rules and network interface added by node-cache
func (c *CacheApp) TeardownNetworking() error {
	clog.Infof("Tearing down")
	if c.exitChan != nil {
		// Stop the goroutine that periodically checks for iptables rules/dummy interface
		// exitChan is a buffered channel of size 1, so this will not block
		c.exitChan <- struct{}{}
	}
	var err error
	if c.params.SetupInterface {
		err = c.netifHandle.RemoveDummyDevice(c.params.InterfaceName)
	}
	if c.params.SetupIptables {
		for _, rule := range c.iptablesRules {
			exists := true
			for exists == true {
				c.iptables.DeleteRule(rule.table, rule.chain, rule.args...)
				exists, _ = c.iptables.EnsureRule(utiliptables.Prepend, rule.table, rule.chain, rule.args...)
			}
			// Delete the rule one last time since EnsureRule creates the rule if it doesn't exist
			c.iptables.DeleteRule(rule.table, rule.chain, rule.args...)
		}
	}
	return err
}

func (c *CacheApp) setupNetworking() {
	if c.params.SetupIptables {
		for _, rule := range c.iptablesRules {
			exists, err := c.iptables.EnsureRule(utiliptables.Prepend, rule.table, rule.chain, rule.args...)
			switch {
			case exists:
				// debug messages can be printed by including "debug" plugin in coreFile.
				clog.Debugf("iptables rule %v for nodelocaldns already exists", rule)
				continue
			case err == nil:
				clog.Infof("Added back nodelocaldns rule - %v", rule)
				continue
				// if we got here, either iptables check failed or adding rule back failed.
			case isLockedErr(err):
				clog.Infof("Error checking/adding iptables rule %v, due to xtables lock in use, retrying in %v", rule, c.params.Interval)
				setupErrCount.WithLabelValues("iptables_lock").Inc()
			default:
				clog.Errorf("Error adding iptables rule %v - %s", rule, err)
				setupErrCount.WithLabelValues("iptables").Inc()
			}
		}
	}

	if c.params.SetupInterface {
		exists, err := c.netifHandle.EnsureDummyDevice(c.params.InterfaceName)
		if !exists {
			if err != nil {
				clog.Errorf("Failed to add non-existent interface %s: %s", c.params.InterfaceName, err)
				setupErrCount.WithLabelValues("interface_add").Inc()
			}
			clog.Infof("Added interface - %s", c.params.InterfaceName)
		}
		if err != nil {
			clog.Errorf("Error checking dummy device %s - %s", c.params.InterfaceName, err)
			setupErrCount.WithLabelValues("interface_check").Inc()
		}
	}
}

func (c *CacheApp) runPeriodic() {
	// if a pidfile is defined in flags, setup iptables as soon as it's created
	if c.params.Pidfile != "" {
		for {
			if isFileExists(c.params.Pidfile) {
				break
			}
			clog.Infof("waiting for coredns pidfile '%s'", c.params.Pidfile)
			time.Sleep(time.Second * 1)
		}
		// we found the pidfile, coreDNS is running, we can setup networking early
		c.setupNetworking()
	}

	c.exitChan = make(chan struct{}, 1)
	tick := time.NewTicker(c.params.Interval * time.Second)
	for {
		select {
		case <-tick.C:
			c.setupNetworking()
		case <-c.exitChan:
			clog.Warningf("Exiting iptables/interface check goroutine")
			return
		}
	}
}

// RunApp invokes the background checks and runs coreDNS as a cache
func (c *CacheApp) RunApp() {
	go c.runPeriodic()
	coremain.Run()
	// Unlikely to reach here, if we did it is because coremain exited and the signal was not trapped.
	clog.Errorf("Untrapped signal, tearing down")
	c.TeardownNetworking()
}

// NewCacheApp returns a new instance of CacheApp by applying the specified config params.
func NewCacheApp(params *ConfigParams) (*CacheApp, error) {
	c := &CacheApp{params: params, kubednsConfig: options.NewKubeDNSConfig()}
	c.clusterDNSIP = net.ParseIP(os.ExpandEnv(toSvcEnv(params.UpstreamSvcName)))
	if c.clusterDNSIP == nil {
		clog.Warningf("Unable to lookup IP address of Upstream service %s, env %s `%s`", params.UpstreamSvcName, toSvcEnv(params.UpstreamSvcName), os.ExpandEnv(toSvcEnv(params.UpstreamSvcName)))
	}
	return c, nil
}

// toSvcEnv converts service name to the corresponding ENV variable. This is exposed in every pod and its value is the clusterIP.
// https://kubernetes.io/docs/concepts/services-networking/service/#environment-variables
func toSvcEnv(svcName string) string {
	envName := strings.Replace(svcName, "-", "_", -1)
	return "$" + strings.ToUpper(envName) + "_SERVICE_HOST"
}

// isFileExists returns true if a file exists with the given path
func isFileExists(path string) bool {
	f, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return !f.IsDir()
}
