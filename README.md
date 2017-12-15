# Bonding CNI plugin

- Bonding provides a method for aggregating multiple network interfaces into a single logical "bonded" interface.

- Linx Bonding drivers provides various flavour of bonded interface depending on the mode (bonding policies), such as round robin, active aggregation according to the 802.3 ad specification

- Bonding CNI plugin works currently with SRIOV CNI and Multus CNI plugin, standalone is WIP

- For more information on the bonding driver. Please refer to [kernel doc](https://www.kernel.org/doc/Documentation/networking/bonding.txt)

## Build & Clean

This plugin is recommended to build with Go 1.7.5 which is fully tested.

```
#./build
```

Build the source codes to binary, copy the bin/vhostuser to the CNI folder for the tests.

## Network configuration reference

* `name` (string, required): the name of the network
* `type` (string, required): "bond"
* `ifname` (string, required): name of the bond interface
* `miimon` (int, required): specifies the arp link monitoring frequency in milliseconds
* `links` (dictionary, required): master interface names.
* `ipam` (dictionary, required): IPAM configuration to be used for this network.

## Usage
### Integrated with Multus plugin and  SRIOV CNI for high performance container Networking solution for NFV Environment 

Refer Multus (NFV based Multi - Network plugin), DPDK-SRIOV CNI plugins
* [Multus - Multi Network plugin](https://github.com/Intel-Corp/multus-cni)
* [DPDK-SRIOV - Dataplane plugin](https://github.com/Intel-Corp/sriov-cni)

Encourage the users/developers to use Multus based Kubernetes CDR/TPR based network objects. Please follow the configuration details in the link: [Usage with Kubernetes CRD/TPR based Network Objects](https://github.com/Intel-Corp/multus-cni/blob/master/README.md#usage-with-kubernetes-crdtpr-based-network-objects)

Please refer the Kubernetes Network SIG - Multiple Network PoC proposal for more details refer the link - [K8s Multiple Network proposal](https://docs.google.com/document/d/1TW3P4c8auWwYy-w_5afIPDcGNLK3LZf0m14943eVfVg/edit)

### Configuration details
```
# cat > /etc/cni/net.d/00-multus.conf <<EOF
{
    "name": "multus-demo-network",
    "type": "multus",
    "delegates": [
        {
            "type": "sriov",
            "if0": "ens3",
            "l2enable": true,
            "if0name": "net0"
        },
        {
            "type": "bond_ipam",
            "ifname": "bond0",
            "mode": "active-backup",
             "miimon": "100",
            "links": [
                    {"name": "net0"},
                    {"name": "net0d1"}
            ],
            "ipam": {
                    "type": "host-local",
                    "subnet": "10.168.1.0/24",
                    "rangeStart": "10.168.1.11",
                    "rangeEnd": "10.168.1.20",
                    "routes": [
                            { "dst": "0.0.0.0/0" }
                    ],
                   "gateway": "10.168.1.1"
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
Launch the workload using yaml file in the kubernetes master, with above configuration in the Multus CNI, SRIOV CNI and Bonding CNI, , each pod should have multiple interfaces.

1. Create “multus-test.yaml” file containing below configuration. Created pod will consist of one “alpine” container running “sleep” command.
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
2. Create pod using command:
```
# kubectl create -f multus-test.yaml
pod "multus-test" created
```
3. Run “ifconfig” command inside the container:
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

net0d1    Link encap:Ethernet  HWaddr 52:00:54:89:42:02
          UP BROADCAST RUNNING SLAVE MULTICAST  MTU:1500  Metric:1
          RX packets:0 errors:0 dropped:0 overruns:0 frame:0
          TX packets:0 errors:0 dropped:0 overruns:0 carrier:0
          collisions:0 txqueuelen:1000
          RX bytes:0 (0.0 B)  TX bytes:0 (0.0 B)
```

Interface name | Description
------------ | -------------
lo | loopback
eth0 | Flannel network tap interface
net0 | Shared VF assigned to the container by [SR-IOV CNI](https://github.com/Intel-Corp/sriov-cni) plugin
net0d1 | Shared VF assigned to the container by [SR-IOV CNI](https://github.com/Intel-Corp/sriov-cni) plugin
bond0 | bond interface from "net0" and "net0d1"

### Contacts
For any questions about bond CNI, please reach out on github issue or feel free to contact the developer in our [Intel-Corp Slack](https://intel-corp.herokuapp.com/)

