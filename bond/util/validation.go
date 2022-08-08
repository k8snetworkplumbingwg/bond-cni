package util

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"github.com/vishvananda/netlink"
)

func ValidateMTU(slaveLinks []netlink.Link, mtu int) error {
	// if not specified set MTU to default
	if mtu == 0 {
		mtu = 1500
	}

	if mtu < 68 {
		return fmt.Errorf("Invalid bond MTU value (%+v), should be 68 or bigger", mtu)
	}
	netHandle, err := netlink.NewHandle()
	if err != nil {
		return fmt.Errorf("Failed to create a new handle, error: %+v", err)
	}
	defer netHandle.Delete()

	// handle the nics like macvlan, ipvlan, etc..
	for _, link := range slaveLinks {
		if mtu > link.Attrs().MTU {
			return fmt.Errorf("Invalid MTU (%+v). The requested MTU for bond is bigger than that of the slave link (%+v), slave MTU (%+v)", mtu, link.Attrs().Name, link.Attrs().MTU)
		}
	}

	pfLinks, err := netHandle.LinkList()
	if err != nil {
		return fmt.Errorf("Failed to lookup physical functions links, error: %+v", err)
	}
	for _, pfLink := range pfLinks {
		vritualFunctions := pfLink.Attrs().Vfs
		if vritualFunctions == nil || len(vritualFunctions) == 0 {
			continue
		}
		for _, vf := range vritualFunctions {
			for _, vfLink := range slaveLinks {
				if bytes.Equal(vf.Mac, vfLink.Attrs().HardwareAddr) {
					if mtu > pfLink.Attrs().MTU {
						return fmt.Errorf("Invalid MTU (%+v). The requested MTU for bond is bigger than that of the physical function (%+v) owning the slave link (%+v)", mtu, pfLink.Attrs().Name, pfLink.Attrs().MTU)
					}
				}
			}
		}
	}
	return nil
}

func HandleMacDuplicates(linkObjectsToBond []netlink.Link, netNsHandle *netlink.Handle) error {
	macsInUse := []string{}
	var err error
	for _, link := range linkObjectsToBond {
		linkMac := link.Attrs().HardwareAddr.String()
		if isMacDuplicated(linkMac, macsInUse) {
			linkMac, err = updateDuplicateMac(link, netNsHandle, macsInUse)
			if err != nil {
				return err
			}
		}
		macsInUse = append(macsInUse, linkMac)
	}
	return nil
}

func isMacDuplicated(mac string, macsInUse []string) bool {
	for _, usedMac := range macsInUse {
		if mac == usedMac {
			return true
		}
	}
	return false
}

func updateDuplicateMac(link netlink.Link, netNsHandle *netlink.Handle, macsInUse []string) (string, error) {
	newMac, err := generateUnusedMac(macsInUse)
	if err != nil {
		return "", err
	}
	err = netNsHandle.LinkSetHardwareAddr(link, []byte(newMac))
	if err != nil {
		return "newMac", nil
	}
	return newMac, nil
}

func generateUnusedMac(macsInUse []string) (string, error) {
	var newMac string
	var err error
	for duplicated := true; duplicated; duplicated = isMacDuplicated(newMac, macsInUse) {
		newMac, err = randomMac()
		if err != nil {
			return "", err
		}
	}
	return newMac, nil
}

func randomMac() (string, error) {
	buf := make([]byte, 5)
	_, err := rand.Read(buf)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x\n", byte(2), buf[0], buf[1], buf[2], buf[3], buf[4]), nil
}
