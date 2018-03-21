# Bonding CNI plugin

- Bonding provides a method for aggregating multiple network interfaces into a single logical &quot;bonded&quot; interface.
- According to the 802.3ad specification, Linux Bonding drivers provides various flavours of bonded interfaces depending on the mode (bonding policies), such as round robin, active aggregation
- When Bonding CNI configured as a standalone plugin, physical interfaces are obtained from host network namespace. With these physical interfaces a bonded interface is created in container network namespace.
- A major user case for bonding in containers is network redundancy of an application in the case of network device or path failure and unavailability. For more information - refer to [network redundancy using interface bonding](https://www.howtoforge.com/tutorial/how-to-configure-high-availability-and-network-bonding-on-linux/)
- And for more information on the bonding driver please refer to [kernel doc](https://www.kernel.org/doc/Documentation/networking/bonding.txt)

## Build &amp; Clean

This plugin is recommended to be built with Go 1.7.5 which has been fully tested.

- Build the source codes to binary:
```
#./build
```
- Copy the binary to the CNI folder for the testing:
```
# cp ./bin/bond /opt/cni/bin/
```
### Network configuration reference

- name (string, required): the name of the network
- type (string, required): &quot;bond&quot;
- ifname (string, optional): name of the bond interface
- miimon (int, required): specifies the arp link monitoring frequency in milliseconds
- links (dictionary, required): master interface names
- ipam (dictionary, required): IPAM configuration to be used for this network

## Usage

### Work standalone

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

### Integration with Multus and SRIOV CNI plugin

User can take advantage of [Multus](https://github.com/Intel-Corp/multus-cni) that enables adding multiple interfaces to a K8s Pod. The [sriov-dpdk](https://github.com/Intel-Corp/sriov-cni) plugin allows a SRIOV VF (Virtual Function) to be added to a container. This example shows how bond CNI could be used in conjunction with these plugins to handle more advance use cases e.g, high performance container networking solution for NFV environment.

- [Multus - Multi Network plugin](https://github.com/Intel-Corp/multus-cni)
- [DPDK-SRIOV - Dataplane plugin](https://github.com/Intel-Corp/sriov-cni)

Users/developers are encouraged to use kubernetes CRD/TPR based network objects with Multus.  Please follow the configuration details in the link: [Usage with Kubernetes CRD/TPR based Network Objects](https://github.com/Intel-Corp/multus-cni/blob/master/README.md#usage-with-kubernetes-crdtpr-based-network-objects)

Please refer to the [K8s Multiple Network PoC proposal](https://docs.google.com/document/d/1TW3P4c8auWwYy-w_5afIPDcGNLK3LZf0m14943eVfVg/edit)for more details.

### Configuration details
```json
# cat > /etc/cni/net.d/00-multus.conf <<EOF
{                                                                        
    "name": "multus-demo-network",                                       
    "type": "multus",                                                    
    "delegates": [                                                       
        {
            "type": "sriov",
            "if0": "ens6f0",
            "if0name": "net0",
            "l2enable": true
        },
        {
            "type": "sriov",
            "if0": "ens6f3",
            "if0name": "net1",
            "l2enable": true
        },
        {
            "type": "bond",
            "ifname": "bond0",
            "mode": "active-backup",
            "miimon": "100",
            "links": [
                    {"name": "net0"},
                    {"name": "net1"}
            ],

            "ipam": {
                 "type": "host-local",
                 "subnet": "192.168.1.0/24",
                 "rangeStart": "192.168.1.21",
                 "rangeEnd": "192.168.1.30",
                 "routes": [
                      { "dst": "0.0.0.0/0" }
                 ],
                 "gateway": "192.168.1.1"
            }
        },
        {
            "type": "flannel",
            "name": "control-network",
            "masterplugin": true,
            "delegate": {
                    "isDefaultGateway": true
            }
        }
    ]
}
EOF
```
#### Launching workloads in Kubernetes

With above Multus configuration, we can now deploy a Pod using the pod spec shown below. (Assuming bond, sriov and flannel CNI  binaries are present in default CNI location). Once successfully created, this Pod should have multiple network interfaces attached to it.

1. Create &quot;multus-test.yaml&quot; file containing below configuration.
```
apiVersion: v1
kind: Pod
metadata:
  name: multus-test
spec:  # specification of the pod's contents
  restartPolicy: Never
  containers:
  - name: multus-test
    image: alpine:latest
    command:
      - /bin/sh
      - "-c"
      - "sleep 60m"
    imagePullPolicy: IfNotPresent
```
2. Create pod using following command:
```
# kubectl create -f multus-test.yaml

pod "multus-test" created
```
3. Run &quot;ifconfig&quot; command inside the container:
```
bond0     Link encap:Ethernet  HWaddr 52:00:54:89:42:02
          inet addr:10.168.1.12  Bcast:0.0.0.0  Mask:255.255.255.0
          inet6 addr: fe80::5000:54ff:fe89:4202/64 Scope:Link
          UP BROADCAST RUNNING MASTER MULTICAST  MTU:1500  Metric:1
          RX packets:0 errors:0 dropped:0 overruns:0 frame:0
          TX packets:16 errors:0 dropped:0 overruns:0 carrier:0
          collisions:0 txqueuelen:1000
          RX bytes:0 (0.0 B)  TX bytes:1296 (1.2 KiB)

eth0      Link encap:Ethernet  HWaddr 0A:58:C0:A8:78:F6
          inet addr:192.168.120.246  Bcast:0.0.0.0  Mask:255.255.252.0
          inet6 addr: fe80::48a0:bff:fe9e:213/64 Scope:Link
          UP BROADCAST RUNNING MULTICAST  MTU:1450  Metric:1
          RX packets:96 errors:0 dropped:0 overruns:0 frame:0
          TX packets:8 errors:0 dropped:0 overruns:0 carrier:0
          collisions:0 txqueuelen:0
          RX bytes:10190 (9.9 KiB)  TX bytes:648 (648.0 B)

lo        Link encap:Local Loopback
          inet addr:127.0.0.1  Mask:255.0.0.0
          inet6 addr: ::1/128 Scope:Host
          UP LOOPBACK RUNNING  MTU:65536  Metric:1
          RX packets:0 errors:0 dropped:0 overruns:0 frame:0
          TX packets:0 errors:0 dropped:0 overruns:0 carrier:0
          collisions:0 txqueuelen:1
          RX bytes:0 (0.0 B)  TX bytes:0 (0.0 B)

net0      Link encap:Ethernet  HWaddr 52:00:54:89:42:02
          UP BROADCAST RUNNING SLAVE MULTICAST  MTU:1500  Metric:1
          RX packets:0 errors:0 dropped:0 overruns:0 frame:0
          TX packets:16 errors:0 dropped:0 overruns:0 carrier:0
          collisions:0 txqueuelen:1000
          RX bytes:0 (0.0 B)  TX bytes:1296 (1.2 KiB)

net1      Link encap:Ethernet  HWaddr 52:00:54:89:42:02
          UP BROADCAST RUNNING SLAVE MULTICAST  MTU:1500  Metric:1
          RX packets:0 errors:0 dropped:0 overruns:0 frame:0
          TX packets:0 errors:0 dropped:0 overruns:0 carrier:0
          collisions:0 txqueuelen:1000
          RX bytes:0 (0.0 B)  TX bytes:0 (0.0 B)
```
| Interface name | Description |
| --- | --- |
| lo | loopback |
| eth0 | Flannel network tap interface |
| net0 | VF assigned to the container by [SR-IOV CNI](https://github.com/Intel-Corp/sriov-cni) plugin from phy port 1(&quot;ens6f0&quot;) |
| net1 | VF assigned to the container by [SR-IOV CNI](https://github.com/Intel-Corp/sriov-cni) plugin from phy port 4(&quot;ens6f3&quot;) |
| bond0 | bond interface from &quot;net0&quot; and &quot;net1&quot; |

### Contacts

For any questions about bond CNI, please reach out on github issue or feel free to contact the developer in our [Intel-Corp Slack](https://intel-corp.herokuapp.com/)

### Contributors
* Abdul Halim
* Derek O'Connor
* Kuralamudhan Ramakrishnan
