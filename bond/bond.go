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
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/containernetworking/plugins/pkg/ipam"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

var (
	maxMTU = 9216
	minMTU = 68
	stdMTU = 1500
)

type bondingConfig struct {
	types.NetConf
	Name        string                   `json:"ifname"`
	Mode        string                   `json:"mode"`
	LinksContNs bool                     `json:"linksInContainer"`
	FailOverMac int                      `json:"failOverMac"`
	Miimon      string                   `json:"miimon"`
	Mtu         int                      `json:"mtu"`
	Links       []map[string]interface{} `json:"links"`
}

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
	return bondConf, bondConf.CNIVersion, nil
}

// retrieve the link names from the bondConf & check they exist. return an array of linkObjectsToBond & error
func getLinkObjectsFromConfig(bondConf *bondingConfig, netNsHandle *netlink.Handle) ([]netlink.Link, error) {
	linkNames := []string{}
	for _, linkName := range bondConf.Links {
		linkNames = append(linkNames, linkName["name"].(string))
	}
	linkObjectsToBond := []netlink.Link{}
	if len(linkNames) > 1 && len(linkNames) <= 2 { // currently only supporting two links to one bond
		for _, linkName := range linkNames {
			linkObject, err := checkLinkExists(linkName, netNsHandle)
			if err != nil {
				return nil, fmt.Errorf("Failed to confirm that link (%+v) exists, error: %+v", linkName, err)
			}
			linkObjectsToBond = append(linkObjectsToBond, linkObject)
		}
	} else {
		return nil, fmt.Errorf("Bonding requires exactly two links, we have %+v", len(linkNames))
	}
	return linkObjectsToBond, nil
}

// check if a "linkName" exists. return the linkObject & error
func checkLinkExists(linkName string, netNsHandle *netlink.Handle) (netlink.Link, error) {
	link, err := netNsHandle.LinkByName(linkName)
	if err != nil {
		return nil, fmt.Errorf("Failed to lookup link name %+v, error: %+v", linkName, err)
	}
	return link, nil
}

// configure the bonded link & add it using the netNsHandle context to add it to the required namespace. return a bondLinkObj pointer & error
func createBondedLink(name string, bondMode string, miimon string, failOverMac int, mtu int, netNsHandle *netlink.Handle) (*netlink.Bond, error) {
	var err error
	bondLinkObj := netlink.NewLinkBond(netlink.NewLinkAttrs())
	bondModeObj := netlink.StringToBondMode(bondMode)
	bondLinkObj.Attrs().Name = name
	bondLinkObj.Attrs().MTU = mtu
	bondLinkObj.Mode = bondModeObj
	bondLinkObj.Miimon, err = strconv.Atoi(miimon)
	bondLinkObj.FailOverMac = netlink.BondFailOverMac(failOverMac)

	if err != nil {
		return nil, fmt.Errorf("Failed to convert bondMiimon value (%+v) to an int, error: %+v", miimon, err)
	}

	err = netNsHandle.LinkAdd(bondLinkObj)
	if err != nil {
		return nil, fmt.Errorf("Failed to add link (%+v) to the netNsHandle, error: %+v", bondLinkObj.Attrs().Name, err)
	}

	return bondLinkObj, nil
}

// loop over the linkObjectsToBond, set each DOWN, update the interface MASTER & set it UP again.
// again we use the netNsHandle to interfact with these links in the namespace provided. return error
func attachLinksToBond(bondLinkObj *netlink.Bond, linkObjectsToBond []netlink.Link, netNsHandle *netlink.Handle) error {
	var err error
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

func setLinksinNetNs(bondConf *bondingConfig, nspath string, releaseLinks bool) error {
	var netNs, curnetNs ns.NetNS
	var err error

	linkNames := []string{}
	for _, linkName := range bondConf.Links {
		linkNames = append(linkNames, linkName["name"].(string))
	}

	if netNs, err = ns.GetNS(nspath); err != nil {
		return fmt.Errorf("failed to open netns %q: %v", nspath, err)
	}

	if curnetNs, err = ns.GetCurrentNS(); err != nil {
		return fmt.Errorf("failed to get init netns: %v", err)
	}

	if releaseLinks == true {
		if err := netNs.Set(); err != nil {
			return fmt.Errorf("failed to enter netns %q: %v", netNs, err)
		}
	}

	if len(linkNames) > 1 && len(linkNames) <= 2 { // currently only supporting two links to one bond
		for _, linkName := range linkNames {
			// get interface link in the network namespace
			link, err := netlink.LinkByName(linkName)
			if err != nil {
				return fmt.Errorf("failed to lookup link interface %q: %v", linkName, err)
			}

			// set link interface down
			if err = netlink.LinkSetDown(link); err != nil {
				return fmt.Errorf("failed to down link interface %q: %v", linkName, err)
			}

			if releaseLinks == true { // move link inteface to network netns
				if err = netlink.LinkSetNsFd(link, int(curnetNs.Fd())); err != nil {
					return fmt.Errorf("failed to move link interface to host netns %q: %v", linkName, err)
				}
			} else {
				if err = netlink.LinkSetNsFd(link, int(netNs.Fd())); err != nil {
					return fmt.Errorf("failed to move link interface to container netns %q: %v", linkName, err)
				}
			}

		}
	} else {
		return fmt.Errorf("Bonding requires exactly two links, we have %+v", len(linkNames))
	}

	return nil
}

func createBond(bondConf *bondingConfig, nspath string, ns ns.NetNS) (*current.Interface, error) {
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
	defer netNsHandle.Delete()

	if bondConf.LinksContNs != true {
		if err := setLinksinNetNs(bondConf, nspath, false); err != nil {
			return nil, fmt.Errorf("Failed to move the links (%+v) in container network namespace, error: %+v", bondConf.Links, err)
		}
	}

	linkObjectsToBond, err := getLinkObjectsFromConfig(bondConf, netNsHandle)
	if err != nil {
		return nil, fmt.Errorf("Failed to retrieve link objects from configuration file (%+v), error: %+v", bondConf, err)
	}
	if bondConf.FailOverMac < 0 || bondConf.FailOverMac > 2 {
		return nil, fmt.Errorf("FailOverMac mode should be 0, 1 or 2 actual: %+v", bondConf.FailOverMac)
	}
	// check if MTU is set outside normal bounds
	// 0 value is used to check if Mtu is set in config
	//TODO: change mtu and other int types to *int to get rid of bad assumption about 0 value.
	if bondConf.Mtu != 0 && ( bondConf.Mtu < minMTU || bondConf.Mtu > maxMTU) {
		return nil, fmt.Errorf("MTU parameter should be between 68, 9216. Requested value: %v", bondConf.Mtu)
	}
	bondLinkObj, err := createBondedLink(bondConf.Name, bondConf.Mode, bondConf.Miimon, bondConf.FailOverMac, bondConf.Mtu, netNsHandle)
	if err != nil {
		return nil, fmt.Errorf("Failed to create bonded link (%+v), error: %+v", bondConf.Name, err)
	}
	err = attachLinksToBond(bondLinkObj, linkObjectsToBond, netNsHandle)
	if err != nil {
		return nil, fmt.Errorf("Failed to attach links to bond, error: %+v", err)
	}

	bond.Name = bondConf.Name

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

	if bondConf.Name == "" {
		bondConf.Name = args.IfName
	}

	bondInterface, err := createBond(bondConf, args.Netns, netns)
	if err != nil {
		return err
	}

	// run the IPAM plugin and get back the config to apply
	r, err := ipam.ExecAdd(bondConf.IPAM.Type, args.StdinData)
	if err != nil {
		return err
	}
	// Convert whatever the IPAM result was into the current Result type
	result, err := current.NewResultFromResult(r)
	if err != nil {
		return err
	}

	if len(result.IPs) == 0 {
		return errors.New("IPAM plugin returned missing IP config")
	}
	for _, ipc := range result.IPs {
		// All addresses belong to the vlan interface
		ipc.Interface = current.Int(0)
	}

	result.Interfaces = []*current.Interface{bondInterface}

	err = netns.Do(func(_ ns.NetNS) error {
		return ipam.ConfigureIface(bondConf.Name, result)
	})
	if err != nil {
		return err
	}

	result.DNS = bondConf.DNS

	return types.PrintResult(result, cniVersion)

}

func cmdDel(args *skel.CmdArgs) error {
	var err error

	bondConf, _, err := loadConfigFile(args.StdinData)
	if err != nil {
		return err
	}

	err = ipam.ExecDel(bondConf.IPAM.Type, args.StdinData)
	if err != nil {
		return err
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
	defer netNsHandle.Delete()

	if bondConf.Name == "" {
		bondConf.Name = args.IfName
	}

	linkObjectsToDeattach, err := getLinkObjectsFromConfig(bondConf, netNsHandle)
	if err != nil {
		return fmt.Errorf("Failed to retrieve link objects from configuration file (%+v), error: %+v", bondConf, err)
	}

	linkObjToDel, err := checkLinkExists(bondConf.Name, netNsHandle)
	if err != nil {
		return fmt.Errorf("Failed to find bonded link (%+v), error: %+v", bondConf.Name, err)
	}
	//reset mtu value of bond to a "standard" value of 1500 which also resets value of each slave link
	//check if original config had mtu set in config - i.e. value of 0
	if bondConf.Mtu != 0 || linkObjToDel.Attrs().MTU != stdMTU {
		err = netNsHandle.LinkSetMTU(linkObjToDel, stdMTU)
		if err != nil {
			return fmt.Errorf("Failed to reset MTU value to default")
		}
	}
	err = netNsHandle.LinkSetDown(linkObjToDel)
	if err != nil {
		return fmt.Errorf("Failed to set bonded link: %+v DOWN, error: %+v", linkObjToDel.Attrs().Name, err)
	}

	if err = deattachLinksFromBond(linkObjectsToDeattach, netNsHandle); err != nil {
		return fmt.Errorf("Failed to detatch links from bond, error: %+v", err)
	}

	err = netNsHandle.LinkDel(linkObjToDel)
	if err != nil {
		return fmt.Errorf("Failed to delete bonded link (%+v), error: %+v", linkObjToDel.Attrs().Name, err)
	}

	if bondConf.LinksContNs != true {
		if err := setLinksinNetNs(bondConf, args.Netns, true); err != nil {
			return fmt.Errorf("Failed set links (%+v) in host network namespace, error: %+v", bondConf.Links, err)
		}
	}

	return err
}
func cmdCheck(args *skel.CmdArgs) error {
	return nil
}

func main() {
	skel.PluginMain(cmdAdd, cmdCheck, cmdDel, version.All, "")
}
