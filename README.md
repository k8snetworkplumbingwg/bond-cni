# Bond CNI plugin

[![Coverage Status](https://coveralls.io/repos/github/k8snetworkplumbingwg/bond-cni/badge.svg?branch=master)](https://coveralls.io/github/k8snetworkplumbingwg/bond-cni?branch=master)

## Overview

[Bonding](https://docs.kernel.org/networking/bonding.html) provides a method
for aggregating multiple network interfaces into a single logical "bonded"
interface. According to the 802.3ad specification, Linux Bonding drivers
provides various flavours of bonded interfaces depending on the mode (bonding
policies), such as round robin, active aggregation.

When Bond CNI is configured as a standalone plugin, interfaces are obtained
from the host network namespace. With these physical interfaces a bonded
interface is created in the container network namespace. When used with
[Multus](https://github.com/k8snetworkplumbingwg/multus-cni) users can bond
two interfaces that have previously been passed into the container.

A major use case for bonding in containers is network redundancy of an
application in the case of network device or path failure and unavailability.
For more information refer to [network redundancy using interface bonding](https://www.howtoforge.com/tutorial/how-to-configure-high-availability-and-network-bonding-on-linux/).

For general information on how CNI plugins are used, refer to the [CNI specification](https://www.cni.dev/docs/spec/).

## Example configuration

```json
{
  "name": "mynet",
  "type": "bond",
  "mode": "active-backup",
  "miimon": "100",
  "mtu": 1500,
  "failOverMac": 1,
  "links": [
    { "name": "ens3f2" },
    { "name": "ens3f2d1" }
  ],
  "ipam": {
    "type": "host-local",
    "subnet": "10.1.1.0/24"
  }
}
```

## Network configuration reference

* `name` (string, required): the name of the network
* `type` (string, required): "bond"
* `mode` (string, required): the bonding policy. Supported values: `balance-rr`, `active-backup`, `balance-xor`, `broadcast`, `802.3ad`, `balance-tlb`, `balance-alb`
* `miimon` (string, required): specifies the MII link monitoring frequency in milliseconds (e.g. `"100"`)
* `mtu` (integer, optional): the MTU of the bond. Default is 1500.
* `failOverMac` (integer, optional): specifies the fail_over_mac setting for the bond. Values: 0 (none), 1 (active), 2 (follow). Default is 0. Refer to the [kernel bonding documentation](https://docs.kernel.org/networking/bonding.html) for details on each mode.
* `linksInContainer` (boolean, optional): specifies if slave links are already in the container namespace. Default is false, i.e. look for interfaces on the host before bonding.
* `links` (array, required): array of objects, each with a `"name"` field specifying the slave interface name. At least two links are required. When `linksInContainer` is false (default), names refer to host interfaces; when true, names refer to interfaces already present in the container namespace.
* `ipam` (dictionary, optional): IPAM configuration to be used for this network. If omitted, the bond interface is created as L2-only (no IP address assigned).
* `allSlavesActive` (integer, optional): specifies that duplicate frames received on inactive ports should be dropped (0) or delivered (1). Default is 0.
* `tlbDynamicLb` (integer, optional): specifies if dynamic shuffling of flows is enabled. Only valid in `balance-tlb` and `balance-alb` modes. Values: 0 (disabled) or 1 (enabled). Default is 1.
* `xmitHashPolicy` (string, optional): selects the transmit hash policy to use for slave selection in `balance-xor`, `802.3ad`, and `balance-tlb` modes. Supported values: `layer2`, `layer3+4`, `layer2+3`, `encap2+3`, `encap3+4`, `vlan+srcmac`.

## Integration with Multus and SRIOV Network Operator

Users can take advantage of
[Multus](https://github.com/k8snetworkplumbingwg/multus-cni) to enable
adding multiple interfaces to a K8s Pod. The
[SRIOV Network Operator](https://github.com/k8snetworkplumbingwg/sriov-network-operator)
provisions and configures SR-IOV virtual functions for Kubernetes pods.
This example shows how Bond CNI could be used in conjunction with these
plugins to handle more advanced use cases e.g, high performance container
networking solution for NFV environment. Specifically the below
functionality shows how to set up failover for SR-IOV interfaces in
Kubernetes.

This configuration is only applicable to SRIOV VFs using the kernel
driver. Userspace driver VFs - such as those used in DPDK workloads -
can not be bonded with the Bond CNI.

Configuration is based on the Multus CRD Network Attachment Definition.
Please follow the configuration details in the link:
[Usage with Kubernetes CRD based Network Objects](https://github.com/k8snetworkplumbingwg/multus-cni/blob/master/docs/configuration.md#configuration-example).

### Bonded failover for SRIOV Workloads

Prerequisites:

* [Multus](https://github.com/k8snetworkplumbingwg/multus-cni) deployed
* [SRIOV Network Operator](https://github.com/k8snetworkplumbingwg/sriov-network-operator) deployed

The VFs used for bonding must come from **different physical functions**
so that a single NIC failure does not take down both slaves.

#### 1) Create SRIOV policies for two separate PFs

```yaml
apiVersion: sriovnetwork.openshift.io/v1
kind: SriovNetworkNodePolicy
metadata:
  name: pf1-policy
  namespace: sriov-network-operator
spec:
  resourceName: sriov_pf_1
  numVfs: 1
  nicSelector:
    pfNames: ["<PF_NAME_1>"]
  deviceType: netdevice
  nodeSelector:
    feature.node.kubernetes.io/network-sriov.capable: "true"
---
apiVersion: sriovnetwork.openshift.io/v1
kind: SriovNetworkNodePolicy
metadata:
  name: pf2-policy
  namespace: sriov-network-operator
spec:
  resourceName: sriov_pf_2
  numVfs: 1
  nicSelector:
    pfNames: ["<PF_NAME_2>"]
  deviceType: netdevice
  nodeSelector:
    feature.node.kubernetes.io/network-sriov.capable: "true"
```

#### 2) Create SRIOV networks (the operator generates NetworkAttachmentDefinitions)

```yaml
apiVersion: sriovnetwork.openshift.io/v1
kind: SriovNetwork
metadata:
  name: sriov-net1
  namespace: sriov-network-operator
spec:
  resourceName: sriov_pf_1
  networkNamespace: default
  spoofChk: "off"
---
apiVersion: sriovnetwork.openshift.io/v1
kind: SriovNetwork
metadata:
  name: sriov-net2
  namespace: sriov-network-operator
spec:
  resourceName: sriov_pf_2
  networkNamespace: default
  spoofChk: "off"
```

#### 3) Create Bond NetworkAttachmentDefinition

```yaml
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: bond-net1
spec:
  config: '{
    "type": "bond",
    "cniVersion": "0.3.1",
    "name": "bond-net1",
    "mode": "active-backup",
    "failOverMac": 1,
    "linksInContainer": true,
    "miimon": "100",
    "mtu": 1500,
    "links": [
      {"name": "net1"},
      {"name": "net2"}
    ],
    "ipam": {
      "type": "host-local",
      "subnet": "10.56.217.0/24",
      "routes": [{"dst": "0.0.0.0/0"}],
      "gateway": "10.56.217.1"
    }
  }'
```

The `"linksInContainer": true` flag tells Bond CNI to look for slave
interfaces inside the container namespace (created by SRIOV CNI via
Multus) rather than on the host.

#### 4) Deploy pod

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: test-pod
  annotations:
    k8s.v1.cni.cncf.io/networks: '[
      {"name": "sriov-net1", "interface": "net1"},
      {"name": "sriov-net2", "interface": "net2"},
      {"name": "bond-net1",  "interface": "bond0"}
    ]'
spec:
  containers:
  - name: bond-test
    image: alpine:latest
    command: ["/bin/sh", "-c", "sleep 60m"]
    resources:
      requests:
        openshift.io/sriov_pf_1: "1"
        openshift.io/sriov_pf_2: "1"
      limits:
        openshift.io/sriov_pf_1: "1"
        openshift.io/sriov_pf_2: "1"
```

The annotation order matters: SRIOV interfaces must be created before
the bond, since Bond CNI references them by name (`net1`, `net2`).
