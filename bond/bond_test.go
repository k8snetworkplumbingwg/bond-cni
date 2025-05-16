// Copyright 2022 CNI authors
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

package main

import (
	"fmt"
	"strconv"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	types020 "github.com/containernetworking/cni/pkg/types/020"
	types040 "github.com/containernetworking/cni/pkg/types/040"
	types100 "github.com/containernetworking/cni/pkg/types/100"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/containernetworking/plugins/pkg/testutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vishvananda/netlink"
)

const (
	IfName           string = "bond0"
	Slave1           string = "net1"
	Slave2           string = "net2"
	ActiveBackupMode        = "active-backup"
	BalanceTlbMode          = "balance-tlb"
	DefaultMTU              = 1400
)

var Slaves = []string{Slave1, Slave2}

var _ = Describe("bond plugin", func() {
	var podNS ns.NetNS
	var initNS ns.NetNS
	var args *skel.CmdArgs
	var linksInContainer bool
	AfterEach(func() {
		Expect(podNS.Close()).To(Succeed())
		Expect(testutils.UnmountNS(podNS)).To(Succeed())
	})

	When("links are in container`s network namespace at initial state (meaning linksInContainer is true)", func() {
		var config string

		BeforeEach(func() {
			var err error

			config = `{
			"name": "bond",
			"type": "bond",
			"cniVersion": "%s",
			"mode": "%s",
			"failOverMac": %d,
			"linksInContainer": %s,
			"miimon": "100",
			"mtu": %s,
			"links": [
				{"name": "net1"},
				{"name": "net2"}
			]
		}`

			linksInContainer = true
			linkAttrs := []netlink.LinkAttrs{
				{Name: Slave1},
				{Name: Slave2},
			}
			podNS, err = testutils.NewNS()
			Expect(err).NotTo(HaveOccurred())
			addLinksInNS(podNS, linkAttrs)
			args = &skel.CmdArgs{
				ContainerID: "dummy",
				Netns:       podNS.Path(),
				IfName:      IfName,
				StdinData:   []byte(fmt.Sprintf(config, "1.0.0", ActiveBackupMode, 1, strconv.FormatBool(linksInContainer), strconv.Itoa(DefaultMTU))),
			}
		})

		It("verifies a plugin is added and deleted correctly", func() {
			By("creating the plugin")
			r, _, err := testutils.CmdAddWithArgs(args, func() error {
				return cmdAdd(args)
			})
			Expect(err).NotTo(HaveOccurred())

			By("validating the returned result is correct")
			checkAddReturnResult(&r, IfName)

			err = podNS.Do(func(ns.NetNS) error {
				defer GinkgoRecover()
				By("validating the bond interface is configured correctly")
				link, err := netlink.LinkByName(IfName)
				Expect(err).NotTo(HaveOccurred())
				validateBondIFConf(link, DefaultMTU, ActiveBackupMode, 100)

				By("validating the bond slaves are configured correctly")
				validateBondSlavesConf(link, Slaves)
				return nil
			})
			Expect(err).NotTo(HaveOccurred())

			By("validating the bond interface is deleted correctly")
			err = testutils.CmdDel(podNS.Path(),
				args.ContainerID, "", func() error { return cmdDel(args) })
			Expect(err).NotTo(HaveOccurred())

			err = podNS.Do(func(ns.NetNS) error {
				defer GinkgoRecover()
				_, err = netlink.LinkByName(IfName)
				Expect(err).To(HaveOccurred())
				return nil
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("verifies the plugin handles multiple del commands", func() {
			By("adding a bond interface")
			_, _, err := testutils.CmdAddWithArgs(args, func() error {
				return cmdAdd(args)
			})
			Expect(err).NotTo(HaveOccurred())

			By("deleting the bond interface")
			err = testutils.CmdDel(podNS.Path(),
				args.ContainerID, "", func() error { return cmdDel(args) })
			Expect(err).NotTo(HaveOccurred())

			By("deleting again the bond interface")
			err = testutils.CmdDel(podNS.Path(),
				args.ContainerID, "", func() error { return cmdDel(args) })
			Expect(err).NotTo(HaveOccurred())

		})

		It("verifies the del command does not fail when a device (in container) assigned to the bond has been deleted", func() {
			By("adding a bond interface")
			_, _, err := testutils.CmdAddWithArgs(args, func() error {
				return cmdAdd(args)
			})
			Expect(err).NotTo(HaveOccurred())

			err = podNS.Do(func(ns.NetNS) error {
				defer GinkgoRecover()

				By("deleting a slave interface")
				slave, err := netlink.LinkByName(Slave1)
				Expect(err).NotTo(HaveOccurred())
				err = netlink.LinkDel(slave)
				Expect(err).NotTo(HaveOccurred())
				return nil
			})

			Expect(err).NotTo(HaveOccurred())

			By("deleting the bond interface")
			err = testutils.CmdDel(podNS.Path(),
				args.ContainerID, "", func() error { return cmdDel(args) })
			Expect(err).NotTo(HaveOccurred())

		})

		DescribeTable("verifies the plugin returns correct results for supported tested versions", func(version string) {

			args.StdinData = []byte(fmt.Sprintf(config, version, ActiveBackupMode, 1, strconv.FormatBool(linksInContainer), strconv.Itoa(DefaultMTU)))

			By(fmt.Sprintf("creating the plugin with config in version %s", version))
			r, _, err := testutils.CmdAddWithArgs(args, func() error {
				return cmdAdd(args)
			})
			Expect(err).NotTo(HaveOccurred())
			By(fmt.Sprintf("expecting the result version to be %s", version))
			Expect(r.Version()).To(Equal(version))
			checkAddReturnResult(&r, IfName)

			By("deleting plugin")
			err = testutils.CmdDel(podNS.Path(),
				args.ContainerID, "", func() error { return cmdDel(args) })
			Expect(err).NotTo(HaveOccurred())
		},
			Entry("When Version is 0.3.0", "0.3.0"),
			Entry("When Version is 0.3.1", "0.3.1"),
			Entry("When Version is 0.4.0", "0.4.0"),
			Entry("When Version is 1.0.0", "1.0.0"),
			Entry("When Version is 0.2.0", "0.2.0"),
			Entry("When Version is 0.1.0", "0.1.0"),
		)

		It("verifies the plugin copes with duplicated macs in balance-tlb mode", func() {
			args.StdinData = []byte(fmt.Sprintf(config, "0.3.1", BalanceTlbMode, 1, strconv.FormatBool(linksInContainer), strconv.Itoa(DefaultMTU)))

			err := podNS.Do(func(ns.NetNS) error {
				defer GinkgoRecover()

				slave1, err := netlink.LinkByName(Slave1)
				Expect(err).NotTo(HaveOccurred())

				slave2, err := netlink.LinkByName(Slave2)
				Expect(err).NotTo(HaveOccurred())

				err = netlink.LinkSetHardwareAddr(slave2, slave1.Attrs().HardwareAddr)
				Expect(err).NotTo(HaveOccurred())
				return nil
			})
			Expect(err).NotTo(HaveOccurred())

			By("creating the plugin")
			r, _, err := testutils.CmdAddWithArgs(args, func() error {
				return cmdAdd(args)
			})
			Expect(err).NotTo(HaveOccurred())

			By("checking the bond was created")
			checkAddReturnResult(&r, IfName)

			Expect(err).NotTo(HaveOccurred())
		})

		It("verifies that mac addresses are restored correctly in active-backup with fail_over_mac 0", func() {
			var bond netlink.Link
			var slave1 netlink.Link
			var slave2 netlink.Link
			var err error

			By("storing mac addresses of slaves")
			err = podNS.Do(func(ns.NetNS) error {
				defer GinkgoRecover()
				slave1, err = netlink.LinkByName(Slave1)
				Expect(err).NotTo(HaveOccurred())

				slave2, err = netlink.LinkByName(Slave2)
				Expect(err).NotTo(HaveOccurred())

				return nil
			})
			Expect(err).NotTo(HaveOccurred())

			macSlave1 := slave1.Attrs().HardwareAddr.String()
			macSlave2 := slave2.Attrs().HardwareAddr.String()

			By("creating the bond with fail_over_mac 0 to force the backup to change the mac")
			args = &skel.CmdArgs{
				ContainerID: "dummy",
				Netns:       podNS.Path(),
				IfName:      IfName,
				StdinData:   []byte(fmt.Sprintf(config, "1.0.0", ActiveBackupMode, 0, strconv.FormatBool(linksInContainer), strconv.Itoa(DefaultMTU))),
			}

			r, _, err := testutils.CmdAddWithArgs(args, func() error {
				return cmdAdd(args)
			})
			Expect(err).NotTo(HaveOccurred())

			By("checking that all slaves have the same mac address")
			checkAddReturnResult(&r, IfName)

			err = podNS.Do(func(ns.NetNS) error {
				defer GinkgoRecover()
				bond, err = netlink.LinkByName(IfName)
				Expect(err).NotTo(HaveOccurred())

				slave1, err = netlink.LinkByName(Slave1)
				Expect(err).NotTo(HaveOccurred())

				slave2, err = netlink.LinkByName(Slave2)
				Expect(err).NotTo(HaveOccurred())

				return nil
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(bond.Attrs().HardwareAddr.String()).To(Equal(slave1.Attrs().HardwareAddr.String()))
			Expect(bond.Attrs().HardwareAddr.String()).To(Equal(slave2.Attrs().HardwareAddr.String()))

			By("deleting the bond")
			err = testutils.CmdDel(podNS.Path(),
				args.ContainerID, "", func() error { return cmdDel(args) })
			Expect(err).NotTo(HaveOccurred())

			By("fetching the slaves mac addresses again")
			err = podNS.Do(func(ns.NetNS) error {
				defer GinkgoRecover()

				_, err = netlink.LinkByName(IfName)
				Expect(err).To(HaveOccurred())

				slave1, err = netlink.LinkByName(Slave1)
				Expect(err).NotTo(HaveOccurred())

				slave2, err = netlink.LinkByName(Slave2)
				Expect(err).NotTo(HaveOccurred())

				return nil
			})
			Expect(err).NotTo(HaveOccurred())

			By("checking that the mac addresses of the slaves are restored")
			Expect(slave1.Attrs().HardwareAddr.String()).To(Equal(macSlave1))
			Expect(slave2.Attrs().HardwareAddr.String()).To(Equal(macSlave2))
		})
	})
	When("Links Have Custom MTU", func() {
		const Slave1Mtu = 1000
		const Slave2Mtu = 800

		var config string

		BeforeEach(func() {
			var err error

			config = `{
			"name": "bond",
			"type": "bond",
			"cniVersion": "%s",
			"mode": "%s",
			"failOverMac": %d,
			"linksInContainer": %s,
			"miimon": "100",
			"mtu": %s,
			"links": [
				{"name": "net1"},
				{"name": "net2"}
			]
		}`

			linksInContainer = true
			linkAttrs := []netlink.LinkAttrs{
				{MTU: Slave1Mtu, Name: Slave1},
				{MTU: Slave2Mtu, Name: Slave2},
			}
			podNS, err = testutils.NewNS()
			Expect(err).NotTo(HaveOccurred())
			addLinksInNS(podNS, linkAttrs)
		})

		DescribeTable("Verify plugin raises error when Bond MTU is bigger then links MTU", func(bondMTU string) {
			args := &skel.CmdArgs{
				ContainerID: "dummy",
				Netns:       podNS.Path(),
				IfName:      IfName,
				StdinData:   []byte(fmt.Sprintf(config, "0.3.1", ActiveBackupMode, 1, strconv.FormatBool(linksInContainer), bondMTU)),
			}
			By("creating the plugin")
			_, _, err := testutils.CmdAddWithArgs(args, func() error {
				return cmdAdd(args)
			})
			Expect(err).To(HaveOccurred())
		},
			Entry("Bond MTU is bigger then one of links MTU", "1200"),
			Entry("Bond MTU is bigger then all of links MTU", "900"),
		)
	})
	When("all_slaves_active is added to the config", func() {
		var config string

		BeforeEach(func() {
			var err error

			config = `{
			"name": "bond",
			"type": "bond",
			"cniVersion": "0.3.1",
			"mode": "%s",
			"failOverMac": 1,
			"linksInContainer": true,
			"miimon": "100",
			"mtu": 1400,
			"links": [
				{"name": "net1"},
				{"name": "net2"}
			],
            "allSlavesActive": %d
		}`

			linksInContainer = true
			linkAttrs := []netlink.LinkAttrs{
				{Name: Slave1},
				{Name: Slave2},
			}
			podNS, err = testutils.NewNS()
			Expect(err).NotTo(HaveOccurred())
			addLinksInNS(podNS, linkAttrs)
		})

		DescribeTable("Verify all slaves active is properly set", func(allSlavesActive int) {
			args := &skel.CmdArgs{
				ContainerID: "dummy",
				Netns:       podNS.Path(),
				IfName:      IfName,
				StdinData:   []byte(fmt.Sprintf(config, ActiveBackupMode, allSlavesActive)),
			}
			By("creating the plugin")
			r, _, err := testutils.CmdAddWithArgs(args, func() error {
				return cmdAdd(args)
			})

			if allSlavesActive != 0 && allSlavesActive != 1 {
				Expect(err).To(HaveOccurred())
				return
			}

			By("validating the returned result is correct")
			checkAddReturnResult(&r, IfName)

			Expect(err).To(Not(HaveOccurred()))

			err = podNS.Do(func(ns.NetNS) error {
				defer GinkgoRecover()
				By("validating the bond interface is configured correctly")
				link, err := netlink.LinkByName(IfName)
				Expect(err).NotTo(HaveOccurred())

				Expect(link.(*netlink.Bond).AllSlavesActive).To(Equal(allSlavesActive))
				return nil
			})
			Expect(err).NotTo(HaveOccurred())
		},
			Entry("all_slaves_active is disabled", 0),
			Entry("all_slaves_active is enabled", 1),
			Entry("all_slaves_active value is invaled", 2),
		)
	})

	When("links are in the initial network namespace at initial state (meaning linksInContainer is false)", func() {
		var config string
		BeforeEach(func() {
			var err error

			config = `{
			"name": "bond",
			"type": "bond",
			"cniVersion": "%s",
			"mode": "%s",
			"failOverMac": %d,
			"linksInContainer": %s,
			"miimon": "100",
			"mtu": %s,
			"links": [
				{"name": "net1"},
				{"name": "net2"}
			]
		}`

			linksInContainer = false
			linkAttrs := []netlink.LinkAttrs{
				{Name: Slave1},
				{Name: Slave2},
			}
			podNS, err = testutils.NewNS()
			Expect(err).NotTo(HaveOccurred())
			initNS, err = testutils.NewNS()
			Expect(err).NotTo(HaveOccurred())
			addLinksInNS(initNS, linkAttrs)
		})

		AfterEach(func() {
			Expect(initNS.Close()).To(Succeed())
			Expect(testutils.UnmountNS(initNS)).To(Succeed())
		})
		It("verifies a plugin is added and deleted correctly ", func() {
			args := &skel.CmdArgs{
				ContainerID: "dummy",
				Netns:       podNS.Path(),
				IfName:      IfName,
				StdinData:   []byte(fmt.Sprintf(config, "0.3.1", ActiveBackupMode, 1, strconv.FormatBool(linksInContainer), strconv.Itoa(DefaultMTU))),
			}
			err := initNS.Do(func(ns.NetNS) error {
				defer GinkgoRecover()
				By("creating the plugin")
				r, _, err := testutils.CmdAddWithArgs(args, func() error {
					return cmdAdd(args)
				})
				Expect(err).NotTo(HaveOccurred())
				By("validating the returned result is correct")
				checkAddReturnResult(&r, IfName)

				return nil
			})
			Expect(err).NotTo(HaveOccurred())

			err = podNS.Do(func(ns.NetNS) error {
				defer GinkgoRecover()
				By("validating the bond interface is configured correctly")
				link, err := netlink.LinkByName(IfName)
				Expect(err).NotTo(HaveOccurred())
				validateBondIFConf(link, DefaultMTU, ActiveBackupMode, 100)

				By("validating the bond slaves are configured correctly")
				validateBondSlavesConf(link, Slaves)
				return nil
			})
			Expect(err).NotTo(HaveOccurred())

			err = initNS.Do(func(ns.NetNS) error {
				By("validating the bond interface is deleted correctly")
				err = testutils.CmdDelWithArgs(args, func() error {
					return cmdDel(args)
				})
				Expect(err).NotTo(HaveOccurred())
				return nil
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that links are not in pod namespace")

			err = podNS.Do(func(ns.NetNS) error {
				defer GinkgoRecover()
				for _, slaveName := range Slaves {
					_, err := netlink.LinkByName(slaveName)
					Expect(err).To(HaveOccurred())
				}
				return nil
			})
			Expect(err).NotTo(HaveOccurred())

			err = initNS.Do(func(ns.NetNS) error {
				By("Checking that links are in initial namespace")
				for _, slaveName := range Slaves {
					_, err := netlink.LinkByName(slaveName)
					Expect(err).NotTo(HaveOccurred())
				}
				return nil
			})
			Expect(err).NotTo(HaveOccurred())

		})
	})
})

func addLinksInNS(initNS ns.NetNS, links []netlink.LinkAttrs) {
	for _, link := range links {
		var err error
		err = initNS.Do(func(ns.NetNS) error {
			defer GinkgoRecover()
			err = netlink.LinkAdd(&netlink.Dummy{
				LinkAttrs: link,
			})
			Expect(err).NotTo(HaveOccurred())
			return nil
		})
		Expect(err).NotTo(HaveOccurred())
	}
}

func checkAddReturnResult(r *types.Result, bondIfName string) {
	switch result := (*r).(type) {
	case *types040.Result:
		Expect(len(result.Interfaces)).To(Equal(1))
		Expect(result.Interfaces[0].Name).To(Equal(bondIfName))
	case *types100.Result:
		Expect(len(result.Interfaces)).To(Equal(1))
		Expect(result.Interfaces[0].Name).To(Equal(bondIfName))
	case *types020.Result:
		Expect(result.IP4).To(BeNil())
		Expect(result.IP6).To(BeNil())
	default:
		Fail("Unsupported result type")
	}
}

func validateBondIFConf(link netlink.Link, expectedMTU int, expectedMode string, expectedMiimon int) {
	bond := link.(*netlink.Bond)
	Expect(bond.Attrs().MTU).To(Equal(expectedMTU))
	Expect(bond.Mode.String()).To(Equal(expectedMode))
	Expect(bond.Miimon).To(Equal(expectedMiimon))
}

func validateBondSlavesConf(link netlink.Link, slaves []string) {
	bond := link.(*netlink.Bond)
	for _, slaveName := range slaves {
		slave, err := netlink.LinkByName(slaveName)
		Expect(err).NotTo(HaveOccurred())
		Expect(slave.Attrs().Slave).NotTo(BeNil())
		Expect(slave.Attrs().MasterIndex).To(Equal(bond.Attrs().Index))
	}
}
