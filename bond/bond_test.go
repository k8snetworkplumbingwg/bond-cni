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
	IfName string = "bond0"
	Slave1 string = "net1"
	Slave2 string = "net2"
	Config string = `{
			"name": "bond",
			"type": "bond",
			"cniVersion": "%s",
			"mode": "%s",
			"failOverMac": 1,
			"linksInContainer": %s,
			"miimon": "100",
			"mtu": 1400,
			"links": [
				{"name": "net1"},
				{"name": "net2"}
			]
		}`
	ActiveBackupMode = "active-backup"
	BalanceTlbMode   = "balance-tlb"
)

var Slaves = []string{Slave1, Slave2}

var _ = Describe("bond plugin", func() {
	var podNS ns.NetNS
	var initNS ns.NetNS
	var args *skel.CmdArgs
	var linksInContainer bool
	var linkAttrs = []netlink.LinkAttrs{
		{Name: Slave1},
		{Name: Slave2},
	}

	AfterEach(func() {
		Expect(podNS.Close()).To(Succeed())
		Expect(testutils.UnmountNS(podNS)).To(Succeed())
	})

	When("links are in container`s network namespace at initial state (meaning linksInContainer is true)", func() {
		BeforeEach(func() {
			var err error
			linksInContainer = true
			podNS, err = testutils.NewNS()
			Expect(err).NotTo(HaveOccurred())
			addLinksInNS(podNS, linkAttrs)
			args = &skel.CmdArgs{
				ContainerID: "dummy",
				Netns:       podNS.Path(),
				IfName:      IfName,
				StdinData:   []byte(fmt.Sprintf(Config, "0.3.1", ActiveBackupMode, strconv.FormatBool(true))),
			}
		})

		It("verifies a plugin is added and deleted correctly", func() {
			By("creating the plugin")
			r, _, err := testutils.CmdAddWithArgs(args, func() error {
				return cmdAdd(args)
			})
			Expect(err).NotTo(HaveOccurred())

			By("validationg the returned result is correct")
			checkAddReturnResult(&r, IfName)

			err = podNS.Do(func(ns.NetNS) error {
				defer GinkgoRecover()
				By("validating the bond interface is configured correctly")
				link, err := netlink.LinkByName(IfName)
				Expect(err).NotTo(HaveOccurred())
				bond := link.(*netlink.Bond)
				Expect(bond.Attrs().MTU).To(Equal(1400))
				Expect(bond.Mode.String()).To(Equal(ActiveBackupMode))
				Expect(bond.Miimon).To(Equal(100))

				By("validating the bond slaves are configured correctly")
				for _, slaveName := range Slaves {
					slave, err := netlink.LinkByName(slaveName)
					Expect(err).NotTo(HaveOccurred())
					Expect(slave.Attrs().Slave).NotTo(BeNil())
					Expect(slave.Attrs().MasterIndex).To(Equal(bond.Attrs().Index))
				}
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

			args.StdinData = []byte(fmt.Sprintf(Config, version, ActiveBackupMode, strconv.FormatBool(linksInContainer)))

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
			args.StdinData = []byte(fmt.Sprintf(Config, "0.3.1", BalanceTlbMode, strconv.FormatBool(linksInContainer)))

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

		It("verifies the plugin handles duplicated macs on delete", func() {
			var slave1, slave2 netlink.Link
			var err error

			err = podNS.Do(func(ns.NetNS) error {
				defer GinkgoRecover()
				slave1, err = netlink.LinkByName(Slave1)
				Expect(err).NotTo(HaveOccurred())

				slave2, err = netlink.LinkByName(Slave2)
				Expect(err).NotTo(HaveOccurred())

				err = netlink.LinkSetHardwareAddr(slave2, slave1.Attrs().HardwareAddr)
				Expect(err).NotTo(HaveOccurred())
				return nil
			})

			By("creating the plugin")
			r, _, err := testutils.CmdAddWithArgs(args, func() error {
				return cmdAdd(args)
			})
			Expect(err).NotTo(HaveOccurred())

			By("checking the bond was created")
			checkAddReturnResult(&r, IfName)

			err = podNS.Do(func(ns.NetNS) error {
				defer GinkgoRecover()
				By("duplicating the macs on the slaves")
				err = netlink.LinkSetHardwareAddr(slave2, slave1.Attrs().HardwareAddr)
				Expect(err).NotTo(HaveOccurred())
				return nil
			})
			By("deleting the plugin")
			err = testutils.CmdDel(podNS.Path(),
				args.ContainerID, "", func() error { return cmdDel(args) })
			Expect(err).NotTo(HaveOccurred())

			err = podNS.Do(func(ns.NetNS) error {
				defer GinkgoRecover()
				By("validating the macs are not duplicated")
				slave1, err = netlink.LinkByName(Slave1)
				Expect(err).NotTo(HaveOccurred())
				slave2, err = netlink.LinkByName(Slave2)
				Expect(err).NotTo(HaveOccurred())
				Expect(slave1.Attrs().HardwareAddr.String()).NotTo(Equal(slave2.Attrs().HardwareAddr.String()))
				return nil
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
	When("links are in the initial network namespace at initial state (meaning linksInContainer is false)", func() {
		BeforeEach(func() {
			var err error
			linksInContainer = false
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
				StdinData:   []byte(fmt.Sprintf(Config, "0.3.1", ActiveBackupMode, strconv.FormatBool(linksInContainer))),
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
				validateBondIFConf(link, 1400, ActiveBackupMode, 100)

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
