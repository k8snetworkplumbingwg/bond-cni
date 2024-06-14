// Copyright (c) 2017 Intel Corporation
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// CNI plugin for container network interface bonding.

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strconv"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/containernetworking/plugins/pkg/ipam"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/intel/bond-cni/bond/util"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

type bondingConfig struct {
	types.NetConf
	Mode        string                   `json:"mode"`
	LinksContNs bool                     `json:"linksInContainer"`
	FailOverMac int                      `json:"failOverMac"`
	Miimon      string                   `json:"miimon"`
	Links       []map[string]interface{} `json:"links"`
	MTU         int                      `json:"mtu"`
}

var (
	bondCni = "bond"
)

func init() {
	// this ensures that main runs only on main thread (thread group leader).
	// since namespace ops (unshare, setns) are done for a single thread, we
	// must ensure that the goroutine does not jump from OS thread to thread
	runtime.LockOSThread()
}

// load the configuration file into a bondingConfig structure. return the bondConf & error
func loadConfigFile(bytes []byte) (*bondingConfig, string, error) {
	bondConf := &bondingConfig{}
	if err := json.Unmarshal(bytes, bondConf); err != nil {
		return nil, "", fmt.Errorf("Failed to load configuration file, error = %+v", err)
	}
	if bondConf.IPAM.Type == bondCni {
		return nil, "", fmt.Errorf("Bond is not a suitable IPAM type")
	}
	return bondConf, bondConf.CNIVersion, nil
}

// retrieve the link names from the bondConf & check they exist. return an array of linkObjectsToBond & error
func getLinkObjectsFromConfig(bondConf *bondingConfig, netNsHandle *netlink.Handle, releaseLinks bool) ([]netlink.Link, error) {
	linkNames := []string{}
	for _, linkName := range bondConf.Links {
		s, ok := linkName["name"].(string)
		if !ok {
			return nil, fmt.Errorf("failed to find link name")
		}
		linkNames = append(linkNames, s)
	}

	// currently supporting two or more links to one bond.
	if len(linkNames) < 2 {
		return nil, fmt.Errorf("Bonding requires at least two links, we have %+v", len(linkNames))
	}

	linkObjectsToBond := []netlink.Link{}
	for _, linkName := range linkNames {
		linkObject, err := netNsHandle.LinkByName(linkName)
		if err != nil {
			// Do not fail if device in container assigned to the bond has been deleted.
			// This device might have been deleted by another plugin.
			_, ok := err.(netlink.LinkNotFoundError)
			if !ok || !releaseLinks {
				return nil, fmt.Errorf("Failed to confirm that link (%+v) exists, error: %+v", linkName, err)
			}
		} else {
			linkObjectsToBond = append(linkObjectsToBond, linkObject)
		}
	}

	return linkObjectsToBond, nil
}

// configure the bonded link & add it using the netNsHandle context to add it to the required namespace. return a bondLinkObj pointer & error
func createBondedLink(bondName string, bondMode string, bondMiimon string, bondMTU int, failOverMac int, netNsHandle *netlink.Handle) (*netlink.Bond, error) {
	var err error
	bondLinkObj := netlink.NewLinkBond(netlink.NewLinkAttrs())
	bondModeObj := netlink.StringToBondMode(bondMode)
	bondLinkObj.Attrs().Name = bondName
	bondLinkObj.Mode = bondModeObj
	bondLinkObj.Miimon, err = strconv.Atoi(bondMiimon)
	if err != nil {
		return nil, fmt.Errorf("Failed to convert bondMiimon value (%+v) to an int, error: %+v", bondMiimon, err)
	}
	if bondMTU != 0 {
		bondLinkObj.MTU = bondMTU
	}
	bondLinkObj.FailOverMac = netlink.BondFailOverMac(failOverMac)

	err = netNsHandle.LinkAdd(bondLinkObj)
	if err != nil {
		return nil, fmt.Errorf("Failed to add link (%+v) to the netNsHandle, error: %+v", bondLinkObj.Attrs().Name, err)
	}

	return bondLinkObj, nil
}

// loop over the linkObjectsToBond, set each DOWN, update the interface MASTER & set it UP again.
// again we use the netNsHandle to interfact with these links in the namespace provided. return error
func attachLinksToBond(bondLinkObj *netlink.Bond, linkObjectsToBond []netlink.Link, netNsHandle *netlink.Handle) error {
	err := util.HandleMacDuplicates(linkObjectsToBond, netNsHandle)
	if err != nil {
		return fmt.Errorf("Failed to handle duplicated macs on link slaves, error: %+v", err)
	}

	bondLinkIndex := bondLinkObj.LinkAttrs.Index
	for _, linkObject := range linkObjectsToBond {
		err = netNsHandle.LinkSetDown(linkObject)
		if err != nil {
			return fmt.Errorf("Failed to set link: %+v DOWN, error: %+v", linkObject.Attrs().Name, err)
		}
		err = netNsHandle.LinkSetMasterByIndex(linkObject, bondLinkIndex)
		if err != nil {
			return fmt.Errorf("Failed to set link: %+v MASTER, master index used: %+v, error: %+v", linkObject.Attrs().Name, bondLinkIndex, err)
		}
		err = netNsHandle.LinkSetUp(linkObject)
		if err != nil {
			return fmt.Errorf("Failed to set link: %+v UP, error: %+v", linkObject.Attrs().Name, err)
		}
	}
	return nil
}

// loop over the linkObjectsToDeattach, set each DOWN, update the interface MASTER to nomaster & set it UP again.
// again we use the netNsHandle to interfact with these links in the namespace provided. return error
func deattachLinksFromBond(linkObjectsToDeattach []netlink.Link, netNsHandle *netlink.Handle) error {
	var err error

	for _, linkObject := range linkObjectsToDeattach {
		err = netNsHandle.LinkSetDown(linkObject)
		if err != nil {
			return fmt.Errorf("Failed to set link: %+v DOWN, error: %+v", linkObject.Attrs().Name, err)
		}
		err = netNsHandle.LinkSetNoMaster(linkObject)
		if err != nil {
			return fmt.Errorf("Failed to set link: %+v NOMASTER, error: %+v", linkObject.Attrs().Name, err)
		}
		err = netNsHandle.LinkSetUp(linkObject)
		if err != nil {
			return fmt.Errorf("Failed to set link: %+v UP, error: %+v", linkObject.Attrs().Name, err)
		}
	}
	return nil
}

func setLinksInNetNs(bondConf *bondingConfig, nspath string, releaseLinks bool) error {
	var podNs, hostNS ns.NetNS
	var err error

	linkNames := []string{}
	for _, linkName := range bondConf.Links {
		s, ok := linkName["name"].(string)
		if !ok {
			return fmt.Errorf("failed to find link name")
		}
		linkNames = append(linkNames, s)
	}

	if podNs, err = ns.GetNS(nspath); err != nil {
		return fmt.Errorf("failed to open netns %q: %v", nspath, err)
	}
	defer podNs.Close()

	if hostNS, err = ns.GetCurrentNS(); err != nil {
		return fmt.Errorf("failed to get init netns: %v", err)
	}

	if releaseLinks {
		return moveLinksBetweenNs(linkNames, podNs, hostNS, "host")
	}
	return moveLinksBetweenNs(linkNames, hostNS, podNs, "container")
}

func moveLinksBetweenNs(links []string, from ns.NetNS, to ns.NetNS, toNsName string) error {
	return from.Do(func(ns.NetNS) error {
		if len(links) < 2 { // currently supporting two or more links to one bond
			return fmt.Errorf("Bonding requires at least two links, we have %+v", len(links))
		}
		for _, linkName := range links {
			// get interface link in the network namespace
			link, err := netlink.LinkByName(linkName)
			if err != nil {
				return fmt.Errorf("failed to lookup link interface %q: %v", linkName, err)
			}

			// set link interface down
			if err = netlink.LinkSetDown(link); err != nil {
				return fmt.Errorf("failed to down link interface %q: %v", linkName, err)
			}

			// move link interface to network netns
			if err = netlink.LinkSetNsFd(link, int(to.Fd())); err != nil {
				return fmt.Errorf("failed to move link interface to %s netns %q: %v", toNsName, linkName, err)
			}
		}
		return nil
	})
}

func createBond(bondName string, bondConf *bondingConfig, nspath string, ns ns.NetNS) (*current.Interface, error) {
	bond := &current.Interface{}

	// get the namespace from the CNI_NETNS environment variable
	netNs, err := netns.GetFromPath(nspath)
	if err != nil {
		return nil, fmt.Errorf("Failed to retrieve netNs from path (%+v), error: %+v", nspath, err)
	}
	defer netNs.Close()

	// get a handle for the namespace above, this handle will be used to interact with existing links and add a new one
	netNsHandle, err := netlink.NewHandleAt(netNs)
	if err != nil {
		return nil, fmt.Errorf("Failed to create a new handle at netNs (%+v), error: %+v", netNs, err)
	}
	defer netNsHandle.Close()

	if !bondConf.LinksContNs {
		if err := setLinksInNetNs(bondConf, nspath, false); err != nil {
			return nil, fmt.Errorf("Failed to move the links (%+v) in container network namespace, error: %+v", bondConf.Links, err)
		}
	}

	linkObjectsToBond, err := getLinkObjectsFromConfig(bondConf, netNsHandle, false)
	if err != nil {
		return nil, fmt.Errorf("Failed to retrieve link objects from configuration file (%+v), error: %+v", bondConf, err)
	}

	err = util.ValidateMTU(linkObjectsToBond, bondConf.MTU)
	if err != nil {
		return nil, err
	}

	if bondConf.FailOverMac < 0 || bondConf.FailOverMac > 2 {
		return nil, fmt.Errorf("FailOverMac mode should be 0, 1 or 2 actual: %+v", bondConf.FailOverMac)
	}
	bondLinkObj, err := createBondedLink(bondName, bondConf.Mode, bondConf.Miimon, bondConf.MTU, bondConf.FailOverMac, netNsHandle)
	if err != nil {
		return nil, fmt.Errorf("Failed to create bonded link (%+v), error: %+v", bondName, err)
	}

	err = attachLinksToBond(bondLinkObj, linkObjectsToBond, netNsHandle)
	if err != nil {
		return nil, fmt.Errorf("Failed to attached links to bond, error: %+v", err)
	}

	if err := netNsHandle.LinkSetUp(bondLinkObj); err != nil {
		return nil, fmt.Errorf("Failed to set bond link UP, error: %v", err)
	}

	bond.Name = bondName

	// Re-fetch interface to get all properties/attributes
	contBond, err := netNsHandle.LinkByName(bond.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to refetch bond %q: %v", bond.Name, err)
	}
	bond.Mac = contBond.Attrs().HardwareAddr.String()
	bond.Sandbox = ns.Path()

	return bond, nil

}

func cmdAdd(args *skel.CmdArgs) error {
	var err error

	bondConf, cniVersion, err := loadConfigFile(args.StdinData)
	if err != nil {
		return err
	}

	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		return fmt.Errorf("failed to open netns %q: %v", netns, err)
	}
	defer netns.Close()

	bondInterface, err := createBond(args.IfName, bondConf, args.Netns, netns)
	if err != nil {
		return err
	}

	// Initialize an L2 default result.
	result := &current.Result{
		CNIVersion: cniVersion,
		Interfaces: []*current.Interface{bondInterface},
	}

	// run the IPAM plugin and get back the config to apply
	if bondConf.IPAM.Type != "" {
		r, err := ipam.ExecAdd(bondConf.IPAM.Type, args.StdinData)
		if err != nil {
			return err
		}
		// Convert whatever the IPAM result was into the current Result type
		ipamResult, err := current.NewResultFromResult(r)
		if err != nil {
			return err
		}

		if len(ipamResult.IPs) == 0 {
			return errors.New("IPAM plugin returned missing IP config")
		}
		for _, ipc := range ipamResult.IPs {
			// All addresses belong to the vlan interface
			ipc.Interface = current.Int(0)
		}

		result.IPs = ipamResult.IPs
		result.Routes = ipamResult.Routes

		err = netns.Do(func(_ ns.NetNS) error {
			return ipam.ConfigureIface(args.IfName, result)
		})
		if err != nil {
			return err
		}

		result.DNS = bondConf.DNS

	}

	return types.PrintResult(result, cniVersion)

}

func cmdDel(args *skel.CmdArgs) error {
	var err error

	bondConf, _, err := loadConfigFile(args.StdinData)
	if err != nil {
		return err
	}

	if bondConf.IPAM.Type != "" {
		err = ipam.ExecDel(bondConf.IPAM.Type, args.StdinData)
		if err != nil {
			return err
		}
	}

	if args.Netns == "" {
		return nil
	}

	// get the namespace from the CNI_NETNS environment variable
	netNs, err := netns.GetFromPath(args.Netns)
	if err != nil {
		return fmt.Errorf("Failed to retrieve netNs from path (%+v), error: %+v", args.Netns, err)
	}
	defer netNs.Close()
	// get a handle for the namespace above, this handle will be used to interact with existing links and add a new one
	netNsHandle, err := netlink.NewHandleAt(netNs)
	if err != nil {
		return fmt.Errorf("Failed to create a new handle at netNs (%+v), error: %+v", netNs, err)
	}
	defer netNsHandle.Close()

	linkObjToDel, err := netNsHandle.LinkByName(args.IfName)
	if err != nil {
		// Do not fail if the device is already removed. Delete can be called multiple times.
		if _, ok := err.(netlink.LinkNotFoundError); ok {
			return nil
		}
		return fmt.Errorf("Failed to find bonded link (%+v), error: %+v", bondConf.Name, err)
	}

	linkObjectsToDeattach, err := getLinkObjectsFromConfig(bondConf, netNsHandle, true)
	if err != nil {
		return fmt.Errorf("Failed to retrieve link objects from configuration file (%+v), error: %+v", bondConf, err)
	}

	err = netNsHandle.LinkSetDown(linkObjToDel)
	if err != nil {
		return fmt.Errorf("Failed to set bonded link: %+v DOWN, error: %+v", linkObjToDel.Attrs().Name, err)
	}

	if err = deattachLinksFromBond(linkObjectsToDeattach, netNsHandle); err != nil {
		return fmt.Errorf("Failed to deattached links from bond, error: %+v", err)
	}

	if err = util.HandleMacDuplicates(linkObjectsToDeattach, netNsHandle); err != nil {
		return fmt.Errorf("Failed to validate deattached links macs, error: %+v", err)
	}

	err = netNsHandle.LinkDel(linkObjToDel)
	if err != nil {
		return fmt.Errorf("Failed to delete bonded link (%+v), error: %+v", linkObjToDel.Attrs().Name, err)
	}

	if !bondConf.LinksContNs {
		if err := setLinksInNetNs(bondConf, args.Netns, true); err != nil {
			return fmt.Errorf("Failed set links (%+v) in host network namespace, error: %+v", bondConf.Links, err)
		}
	}

	return err
}
func cmdCheck(args *skel.CmdArgs) error {
	return nil
}

func main() {
	skel.PluginMainFuncs(skel.CNIFuncs{Add: cmdAdd, Del: cmdDel, Check: cmdCheck}, version.All, "")
}
