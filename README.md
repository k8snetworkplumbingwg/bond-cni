# Bond CNI plugin

- Bonding provides a method for aggregating multiple network interfaces into a single logical &quot;bonded&quot; interface.
- According to the 802.3ad specification, Linux Bonding drivers provides various flavours of bonded interfaces depending on the mode (bonding policies), such as round robin, active aggregation
- When Bond CNI is configured as a standalone plugin, interfaces are obtained from the host network namespace. With these physical interfaces a bonded interface is created in the container network namespace.
- When used with [Multus](https://github.com/intel/multus-cni) users can bond two interfaces that have previously been passed into the container.
- A major use case for bonding in containers is network redundancy of an application in the case of network device or path failure and unavailability. For more information - refer to [network redundancy using interface bonding](https://www.howtoforge.com/tutorial/how-to-configure-high-availability-and-network-bonding-on-linux/)
- And for more information on the bonding driver please refer to [kernel doc](https://www.kernel.org/doc/Documentation/networking/bonding.txt)

## Build

It is recommended that Bond CNI be built with Go 1.12+ with dependencies managed using Go modules.


- Build the source code to binary:

```
make build-bin
```

- Copy the binary to the CNI folder for the testing:

```
cp ./bin/bond /opt/cni/bin/
```

The binary should be placed at /opt/cni/bin on all nodes on which bonding will take place. That is all nodes to which a container with a bonded interface can be deployed.

## Network configuration reference

- name (string, required): the name of the network
- type (string, required): &quot;bond&quot;
- ifname (string, optional): name of the bond interface
- miimon (int, required): specifies the arp link monitoring frequency in milliseconds
- failOverMac (int, optional): specifies the failOverMac setting for the bond. Should be set to 1 for active-backup bond modes. Default is 0.
- linksInContainer(boolean, optional): specifies if slave links are in container to start. Default is false i.e. look for interfaces on host before bonding.
- links (dictionary, required): master interface names
- ipam (dictionary, required): IPAM configuration to be used for this network

## Usage

### Standalone operation

Given the following network configuration:
```json
# cat > /etc/cni/net.d/00-flannel-bonding.conf <<EOF
{
	"name": "mynet",
	"type": "flannel",
	"delegate": {
		"type": "bond",
		"mode": "active-backup",
		"miimon": "100",
                "failOverMac": 1,
		"links": [
            {
				"name": "ens3f2"
			},
			{
				"name": "ens3f2d1"
			}
		]
	}
}
EOF
```
Note: In this example configuration above required &quot;ipam&quot; is provided by flannel plugin implicitly.

## Integration with Multus, SRIOV CNI and SRIOV Device Plugin

Users can take advantage of [Multus](https://github.com/intel/multus-cni) to enable adding multiple interfaces to a K8s Pod. The [SRIOV CNI](https://github.com/intel/sriov-cni) plugin allows a SRIOV VF (Virtual Function) to be added to a container. Additionally the [SRIOV Device Plugin](https://github.com/intel/sriov-network-device-plugin) allows Kubelet to manage SRIOV virtual functions. This example shows how Bond CNI could be used in conjunction with these plugins to handle more advanced use cases e.g, high performance container networking solution for NFV environment. Specifically the below functionality shows how to set up failover for SR-IOV interfaces in Kubernetes.
This configuration is only applicable to SRIOV VFs using the kernel driver. Userspace driver VFs - such as those used in DPDK workloads - can not be bonded with the Bond CNI.
- [Multus CNI- Multi Network plugin](https://github.com/intel/multus-cni)
- [SRIOV CNI](https://github.com/intel/sriov-cni)
- [SRIOV Network Device Plugin](https://github.com/intel/sriov-network-device-plugin)

Configuration is based on the Multus CRD Network Attachment Definition. Please follow the configuration details in the link: [Usage with Kubernetes CRD based Network Objects](https://github.com/intel/multus-cni/blob/master/doc/configuration.md#configuration-example)

For more information and advanced use refer to the [Network Custom Resource standard](https://docs.google.com/document/d/1TW3P4c8auWwYy-w_5afIPDcGNLK3LZf0m14943eVfVg/edithttps://docs.google.com/document/d/1Ny03h6IDVy_e_vmElOqR7UdTPAG_RNydhVE1Kx54kFQ/edit#heading=h.hylsbqoj5fxd) for more details.

### Bonded failover for SRIOV Workloads 

Prerequisites:

- Multus configured as per the [quick start guide](https://github.com/intel/multus-cni/blob/master/doc/quickstart.md)

- SRIOV CNI and Multus CNI placed in /opt/cni/bin

- SRIOV Device Plugin running as a Daemonset on the cluster

The SRIOV Device Plugin will need to be configured to ensure the VFs in the pod are from different network cards. This is important because failover requires that the bonded interface still have connection even if one of the slave interfaces goes down. If both virtual functions are from the same root any connection issues on the physical interface and card will be reflected in both VFs at the same time.

An example SRIOV config - which works on the basis of physical interface names- is:

```
apiVersion: v1
kind: ConfigMap
metadata:
  name: sriovdp-config
  namespace: kube-system
data:
  config.json: |
    {
        "resourceList": [{
                "resourceName": "intel_sriov_PF_1",
                "selectors": {
                    "vendors": ["8086"],
                    "devices": ["154c", "10ed"],
                    "drivers": ["i40evf", "ixgbevf"],
                    "pfNames": ["<PF_NAME_2>"]

                }
            },
        {
                "resourceName": "intel_sriov_PF_2",
                "selectors": {
                    "vendors": ["8086"],
                    "devices": ["154c", "10ed"],
                    "drivers": ["i40evf", "ixgbevf"],
                    "pfNames": ["<PF_NAME_2>"]
                }
            }
        ]
    }

```
In the above specific PF names will have to be entered - based on available PFs in the cluster - in order to make the selectors pick up the correct VFs. The other selectors in the above configuration are identical.
Note that SRIOV device plugin only picks up new configuration at startup - so if the daemonset was previously running the pods will have to be killed and redeployed before this is advertised again.

## Steps for deployment
1) Deploy Network Attach Definiton for SRIOV

```
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: sriov-net1
  annotations:
    k8s.v1.cni.cncf.io/resourceName: intel.com/intel_sriov_PF_1
spec:
  config: '{
  "type": "sriov",
  "name": "sriov-network",
  "spoofchk":"off"
}'
```

We will create a separate - but equivalent except for naming - SRIOV network attach definition. This allows us to keep our definitions seperate for our two Physical Function pools as definited above.
```
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: sriov-net2
  annotations:
    k8s.v1.cni.cncf.io/resourceName: intel.com/intel_sriov_PF_2
spec:
  config: '{
  "type": "sriov",
  "name": "sriov-network",
  "spoofchk":"off"
}'
```
2) Deploy Network Attach Definition for Bond CNI:
```
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: bond-net1
spec:
  config: '{
  "type": "bond",
  "cniVersion": "0.3.1",
  "name": "bond-net1",
  "ifname": "bond0",
  "mode": "active-backup",
  "failOverMac": 1,
  "linksInContainer": true,
  "miimon": "100",
  "links": [
     {"name": "net1"},
     {"name": "net2"}
  ],
  "ipam": {
    "type": "host-local",
    "subnet": "10.56.217.0/24",
    "routes": [{
      "dst": "0.0.0.0/0"
    }],
    "gateway": "10.56.217.1"
  }
}'
```
Note above the `"linksInContainer": true` flag. This tells the Bond CNI that the interfaces we're looking for are to be found inside the container. By default it will look for these interfaces on the host which does not work for integration with SRIOV/Multus.

3) Deploy a pod which requests two SRIOV networks, one from each PF, and one bonded network.

```
apiVersion: v1
kind: Pod
metadata:
  name: test-pod
  annotations:
        k8s.v1.cni.cncf.io/networks: '[
{"name": "sriov-net1",
"interface": "net1"
},
{"name": "sriov-net2",
"interface": "net2"
},
{"name": "bond-net",
"interface": "bond0"
}]'
spec:
  restartPolicy: Never
  containers:
  - name: bond-test
    image: alpine:latest
    command:
      - /bin/sh
      - "-c"
      - "sleep 60m"
    imagePullPolicy: IfNotPresent
    resources:
      requests:
        intel.com/intel_sriov_PF_1: '1'
        intel.com/intel_sriov_PF_2: '1'
      limits:
        intel.com/intel_sriov_PF_1: '1'
        intel.com/intel_sriov_PF_2: '1'
```

The order in the request annotation `k8s.v1.cni.cncf.io/networks: sriov-net1, sriov-net2, bond-net1` is important as it is the same order in which networks will be added. In the above spec we add one SRIOV network, then we add another identically configured SRIOV network from our second SRIOV VF pool. Multus will give these networks the names net1 and net2 respectively.

Next the bond-net1 network is created - using interfaces net1 and net2. If bond is created before the SRIOV networks the CNI will not be able to find the interfaces in the container.

The name of each interface can be set manually in the annotation according to the [CRD Spec](https://docs.google.com/document/d/1TW3P4c8auWwYy-w_5afIPDcGNLK3LZf0m14943eVfVg/edithttps://docs.google.com/document/d/1Ny03h6IDVy_e_vmElOqR7UdTPAG_RNydhVE1Kx54kFQ/edit#heading=h.hylsbqoj5fxd). Changing the names applied in the annotation configuration requires matching changes to be made in the bond network attachment definition. 

After deploying the above pod spec on Kubernetes running the following command:

```kubectl exec -it test-pod -- ip a```

Will result in  output like:

```
1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UNKNOWN qlen 1000
    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
    inet 127.0.0.1/8 scope host lo
       valid_lft forever preferred_lft forever
3: eth0@if150: <BROADCAST,MULTICAST,UP,LOWER_UP,M-DOWN> mtu 1450 qdisc noqueue state UP 
    link/ether 62:b1:b5:c8:fb:7a brd ff:ff:ff:ff:ff:ff
    inet 10.244.1.122/24 brd 10.244.1.255 scope global eth0
       valid_lft forever preferred_lft forever
4: bond0: <BROADCAST,MULTICAST,UP,LOWER_UP400> mtu 1500 qdisc noqueue state UP qlen 1000
    link/ether 9e:23:69:42:fb:8a brd ff:ff:ff:ff:ff:ff
    inet 10.56.217.66/24 scope global bond0
       valid_lft forever preferred_lft forever
43: net1: <BROADCAST,MULTICAST,UP,LOWER_UP800> mtu 1500 qdisc mq master bond0 state UP qlen 1000
    link/ether 9e:23:69:42:fb:8a brd ff:ff:ff:ff:ff:ff
44: net2: <BROADCAST,MULTICAST,UP,LOWER_UP800> mtu 1500 qdisc mq master bond0 state UP qlen 1000
    link/ether 9e:23:69:42:fb:8a brd ff:ff:ff:ff:ff:ff
```

We have three new interfaces added to our pod - net1 and net2 are SRIOV interfaces while bond0 is the bond over the two of them. Net1 and Net2 don't require IP addresses - and this can be changed in their CRD.
