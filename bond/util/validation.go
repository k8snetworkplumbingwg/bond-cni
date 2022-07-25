package util

import (
	"crypto/rand"
	"fmt"
	"github.com/vishvananda/netlink"
)

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
