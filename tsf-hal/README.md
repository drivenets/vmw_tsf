# Interfaces & Flows HAL API

For interacting with the HALO application we offer a Go module that allows retrieval of interface statistics and performance data, traffic flow statistics as well as the ability to steer (set the next hop) specific flows.

These are the supported functions:

    // HAL interface
    type DnHal interface {
        Steer(*FlowKey, net.IP) error
        GetInterfaces(InterfaceVisitor) error
        GetFlows(FlowVisitor) error
    }

## GetInterfaces

To avoid exposing implementation details retrieving the interfaces starts with HALO application calling _GetInterfaces()_ and providing a visitor (which can be an anonymous function). HAL API iterates through internal structures holding the interfaces data. For each entry it calls the visitor with the corresponding interface name and statistics/performance data.

Here's an example:

    hal.GetInterfaces(
        func(ifc IfName, tm *InterfaceTelemetry) error {
            /*
            * do something with name and telemetry data
            */
            return nil
        },
    )

When visitor returns an error then iteration of interfaces stops and control returns to the caller.

Interface telemetry data contains statistics about the associated NCP port. This data may not be accurate in case this port is also shared with DNOS and/or other service instances. An option here is to provide statistics collected by Linux for the virtual interface used by the HALO container but in this case we may not count offloaded traffic.

    type InterfaceTelemetry struct {
        Speed   uint64
        RxBytes uint64
        RxBps   uint64
        TxBytes uint64
        TxBps   uint64
        Links map[string]LinkTelemetry
    }

Link telemetry reports __only__ information about nodes/neighbors that are configured as directly connected. For example, if a node is _reachable_ from the current interface through another node that acts as a next-hop but it's not present in the configuration then it won't be listed in the _Links_ field.

    type LinkTelemetry struct {
        // assumption: focused on the TX
        Delay  float64
        Jitter float64
    }

We do not provide delay and jitter per egress or ingress and instead provide round-trip aggregated values. Depending on the mechanism used to measure these values it may not be possible to get per Rx or Tx info.

## GetFlows

Similar to _GetInterfaces_ in order to retrieve IP flows data the HALO application is calling _GetFlows()_ and provides a visitor. HAL API iterates through internal structures and then calls back the visitor for each flow present inside some internal structures. The visitor is provided with 5-tuple flow key and corresponding statistics.

Here's an example:

    hal.GetFlows(
        func(fk *FlowKey, tm *FlowTelemetry) error {
            /*
            * do something with flow and telemetry data
            */
        },
    )

_FlowKey_ is a 5-tuple containing: IP protocol (TCP or UDP), source IP and port, destination IP and port.

    type FlowKey struct {
        Protocol FlowProto
        SrcAddr  net.IP
        DstAddr  net.IP
        SrcPort  uint16
        DstPort  uint16
    }

_FlowTelemetry_ provides info about current flow's traffic: rate (per second), total counters, and interfaces (_IfName_ is a string):

    type FlowTelemetry struct {
        RxRatePps uint64
        TxRatePps uint64
        RxRateBps uint64
        TxRateBps uint64

        RxTotalPkts  uint64
        TxTotalPkts  uint64
        RxTotalBytes uint64
        TxTotalBytes uint64

        IngressIf IfName
        EgressIf  IfName
    }

There is an open discussion here about what the internal flow table should contain:

1. keep the full table of flows as reported by periodic NetFlow updates. As more and more updates are coming in we update the corresponding entries (based on the 5-tuple key) in the gobal table and never delete them.

2. store only the last update received from NetFlow. When a new update is received the old information is discarded. Here we have a race between iterating the flow table with _GetFlows()_ and discarding it because a new update is received. It fixes however deletion of old flows.

## Steer

To steer a flow you just need to provide a 5-tuple flow key and the desired next hop:

    Steer(*FlowKey, net.IP) error

Next-hop is provided as`net.IP` address to handle both IPv4 and IPv6.

Here's an example:

    err := hal.Steer(
        &FlowKey{
            Protocol: TCP,
            SrcAddr:  net.ParseIP("10.10.0.1"),
            DstAddr:  net.ParseIP("10.11.0.1"),
            SrcPort:  2038,
            DstPort:  443,
        },
        net.ParseIP("10.10.0.15"),
    )


