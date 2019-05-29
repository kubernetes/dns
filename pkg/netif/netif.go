package netif

import (
	"fmt"
	"github.com/vishvananda/netlink"
	"net"
)

type NetifManager struct {
	netlink.Handle
	Addrs []*netlink.Addr
}

// NewNetifManager returns a new instance of NetifManager with the ip address set to the provided values
// These ip addresses will be bound to any devices created by this instance.
func NewNetifManager(ips []net.IP) *NetifManager {
	nm := &NetifManager{netlink.Handle{}, nil}
	for _, ip := range ips {
		nm.Addrs = append(nm.Addrs, &netlink.Addr{IPNet: netlink.NewIPNet(ip)})
	}
	return nm
}

// EnsureDummyDevice checks for the presence of the given dummy device and creates one if it does not exist.
// Returns a boolean to indicate if this device was found and error if any.
func (m *NetifManager) EnsureDummyDevice(name string) (bool, error) {
	l, err := m.LinkByName(name)
	if err == nil {
		// found dummy device, make sure ip matches. AddrAdd will return error if address exists, will add it otherwise
		for _, addr := range m.Addrs {
			m.AddrAdd(l, addr)
		}
		return true, nil
	}
	return false, m.AddDummyDevice(name)
}

// AddDummyDevice creates a dummy device with the given name. It also binds the ip address of the NetifManager instance
// to this device. This function returns an error if the device exists or if address binding fails.
func (m *NetifManager) AddDummyDevice(name string) error {
	_, err := m.LinkByName(name)
	if err == nil {
		return fmt.Errorf("Link %s exists", name)
	}
	dummy := &netlink.Dummy{
		LinkAttrs: netlink.LinkAttrs{Name: name},
	}
	err = m.LinkAdd(dummy)
	if err != nil {
		return err
	}
	l, _ := m.LinkByName(name)
	for _, addr := range m.Addrs {
		err = m.AddrAdd(l, addr)
		if err != nil {
			return err
		}
	}
	return err
}

// RemoveDummyDevice deletes the dummy device with the given name.
func (m *NetifManager) RemoveDummyDevice(name string) error {
	link, err := m.LinkByName(name)
	if err != nil {
		return err
	}
	return m.LinkDel(link)
}
