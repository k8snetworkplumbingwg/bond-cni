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
	"github.com/containernetworking/cni/pkg/skel"
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
			"linksInContainer": true,
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

var _ = Describe("tuning plugin", func() {
	var podNS ns.NetNS

	BeforeEach(func() {
		var err error
		podNS, err = testutils.NewNS()
		Expect(err).NotTo(HaveOccurred())

		for _, ifName := range Slaves {
			err = podNS.Do(func(ns.NetNS) error {
				defer GinkgoRecover()
				err = netlink.LinkAdd(&netlink.Dummy{
					LinkAttrs: netlink.LinkAttrs{
						Name: ifName,
					},
				})
				Expect(err).NotTo(HaveOccurred())
				_, err := netlink.LinkByName(ifName)
				Expect(err).NotTo(HaveOccurred())
				return nil
			})
		}
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		Expect(podNS.Close()).To(Succeed())
		Expect(testutils.UnmountNS(podNS)).To(Succeed())
	})

	It("verifies a plugin is added and deleted correctly", func() {
		args := &skel.CmdArgs{
			ContainerID: "dummy",
			Netns:       podNS.Path(),
			IfName:      IfName,
			StdinData:   []byte(fmt.Sprintf(Config, "0.3.1", ActiveBackupMode)),
		}

		err := podNS.Do(func(ns.NetNS) error {
			defer GinkgoRecover()

			By("creating the plugin")
			r, _, err := testutils.CmdAddWithArgs(args, func() error {
				return cmdAdd(args)
			})
			Expect(err).NotTo(HaveOccurred())

			By("validationg the returned result is correct")
			result, err := types100.GetResult(r)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(result.Interfaces)).To(Equal(1))
			Expect(result.Interfaces[0].Name).To(Equal(IfName))

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

			By("validating the bond interface is deleted correctly")
			err = testutils.CmdDel(podNS.Path(),
				args.ContainerID, "", func() error { return cmdDel(args) })
			Expect(err).NotTo(HaveOccurred())
			_, err = netlink.LinkByName(IfName)
			Expect(err).To(HaveOccurred())
			return nil
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("verifies the plugin returns correct results for CNI versions 0.3.0, 0.3.1, 0.4.0", func() {
		for _, version := range []string{"0.3.0", "0.3.1", "0.4.0"} {
			args := &skel.CmdArgs{
				ContainerID: "dummy",
				Netns:       podNS.Path(),
				IfName:      IfName,
				StdinData:   []byte(fmt.Sprintf(Config, version, ActiveBackupMode)),
			}
			err := podNS.Do(func(ns.NetNS) error {
				defer GinkgoRecover()

				By(fmt.Sprintf("creating the plugin with config in version %s", version))
				r, _, err := testutils.CmdAddWithArgs(args, func() error {
					return cmdAdd(args)
				})
				Expect(err).NotTo(HaveOccurred())

				By(fmt.Sprintf("expecting the result version to be %s", version))
				result, ok := r.(*types040.Result)
				Expect(ok).To(BeTrue())
				Expect(result.CNIVersion).To(Equal(version))
				Expect(len(result.Interfaces)).To(Equal(1))
				Expect(result.Interfaces[0].Name).To(Equal("bond0"))

				By("deleting plugin")
				err = testutils.CmdDel(podNS.Path(),
					args.ContainerID, "", func() error { return cmdDel(args) })
				Expect(err).NotTo(HaveOccurred())
				return nil
			})
			Expect(err).NotTo(HaveOccurred())
		}
	})

	It("verifies the plugin returns correct results for CNI versions 1.0.0", func() {
		for _, version := range []string{"1.0.0"} {
			args := &skel.CmdArgs{
				ContainerID: "dummy",
				Netns:       podNS.Path(),
				IfName:      IfName,
				StdinData:   []byte(fmt.Sprintf(Config, version, ActiveBackupMode)),
			}
			err := podNS.Do(func(ns.NetNS) error {
				defer GinkgoRecover()

				By(fmt.Sprintf("creating the plugin with config in version %s", version))
				r, _, err := testutils.CmdAddWithArgs(args, func() error {
					return cmdAdd(args)
				})
				Expect(err).NotTo(HaveOccurred())

				By(fmt.Sprintf("expecting the result version to be %s", version))
				result, ok := r.(*types100.Result)
				Expect(ok).To(BeTrue())
				Expect(result.CNIVersion).To(Equal(version))
				Expect(len(result.Interfaces)).To(Equal(1))
				Expect(result.Interfaces[0].Name).To(Equal("bond0"))

				By("deleting plugin")
				err = testutils.CmdDel(podNS.Path(),
					args.ContainerID, "", func() error { return cmdDel(args) })
				Expect(err).NotTo(HaveOccurred())
				return nil
			})
			Expect(err).NotTo(HaveOccurred())
		}
	})

	It("verifies the plugin returns correct results for CNI versions 0.2.0, 0.1.0", func() {
		for _, version := range []string{"0.2.0", "0.1.0"} {
			args := &skel.CmdArgs{
				ContainerID: "dummy",
				Netns:       podNS.Path(),
				IfName:      IfName,
				StdinData:   []byte(fmt.Sprintf(Config, version, ActiveBackupMode)),
			}
			err := podNS.Do(func(ns.NetNS) error {
				defer GinkgoRecover()

				By(fmt.Sprintf("creating the plugin with config in version %s", version))
				r, _, err := testutils.CmdAddWithArgs(args, func() error {
					return cmdAdd(args)
				})
				Expect(err).NotTo(HaveOccurred())

				By(fmt.Sprintf("expecting the result version to be %s", version))
				result, ok := r.(*types020.Result)
				Expect(ok).To(BeTrue())
				Expect(result.CNIVersion).To(Equal(version))
				Expect(result.IP4).To(BeNil())
				Expect(result.IP6).To(BeNil())

				By("deleting plugin")
				err = testutils.CmdDel(podNS.Path(),
					args.ContainerID, "", func() error { return cmdDel(args) })
				Expect(err).NotTo(HaveOccurred())
				return nil
			})
			Expect(err).NotTo(HaveOccurred())
		}
	})

	It("verifies the plugin copes with duplicated macs in balance-tlb mode", func() {
		args := &skel.CmdArgs{
			ContainerID: "dummy",
			Netns:       podNS.Path(),
			IfName:      IfName,
			StdinData:   []byte(fmt.Sprintf(Config, "0.3.1", BalanceTlbMode)),
		}

		err := podNS.Do(func(ns.NetNS) error {
			defer GinkgoRecover()

			slave1, err := netlink.LinkByName(Slave1)
			Expect(err).NotTo(HaveOccurred())

			slave2, err := netlink.LinkByName(Slave2)
			Expect(err).NotTo(HaveOccurred())

			err = netlink.LinkSetHardwareAddr(slave2, slave1.Attrs().HardwareAddr)
			Expect(err).NotTo(HaveOccurred())

			By("creating the plugin")
			r, _, err := testutils.CmdAddWithArgs(args, func() error {
				return cmdAdd(args)
			})
			Expect(err).NotTo(HaveOccurred())

			By("checking the bond was created")
			result, err := types100.GetResult(r)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(result.Interfaces)).To(Equal(1))
			Expect(result.Interfaces[0].Name).To(Equal(IfName))
			return nil
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("verifies the plugin handles duplicated macs on delete", func() {
		args := &skel.CmdArgs{
			ContainerID: "dummy",
			Netns:       podNS.Path(),
			IfName:      IfName,
			StdinData:   []byte(fmt.Sprintf(Config, "0.3.1", ActiveBackupMode)),
		}

		err := podNS.Do(func(ns.NetNS) error {
			defer GinkgoRecover()

			slave1, err := netlink.LinkByName(Slave1)
			Expect(err).NotTo(HaveOccurred())

			slave2, err := netlink.LinkByName(Slave2)
			Expect(err).NotTo(HaveOccurred())

			err = netlink.LinkSetHardwareAddr(slave2, slave1.Attrs().HardwareAddr)
			Expect(err).NotTo(HaveOccurred())

			By("creating the plugin")
			r, _, err := testutils.CmdAddWithArgs(args, func() error {
				return cmdAdd(args)
			})
			Expect(err).NotTo(HaveOccurred())

			By("checking the bond was created")
			result, err := types100.GetResult(r)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(result.Interfaces)).To(Equal(1))
			Expect(result.Interfaces[0].Name).To(Equal(IfName))

			By("duplicating the macs on the slaves")
			err = netlink.LinkSetHardwareAddr(slave2, slave1.Attrs().HardwareAddr)
			Expect(err).NotTo(HaveOccurred())

			By("deleting the plugin")
			err = testutils.CmdDel(podNS.Path(),
				args.ContainerID, "", func() error { return cmdDel(args) })
			Expect(err).NotTo(HaveOccurred())

			By("validating the macs are not duplicated")
			slave1, err = netlink.LinkByName(Slave1)
			Expect(err).NotTo(HaveOccurred())
			slave2, err = netlink.LinkByName(Slave2)
			Expect(slave1.Attrs().HardwareAddr.String()).NotTo(Equal(slave2.Attrs().HardwareAddr.String()))
			return nil
		})
		Expect(err).NotTo(HaveOccurred())
	})
})
