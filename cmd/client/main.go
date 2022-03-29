package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	hal "github.com/drivenets/vmw_tsf/pkg/hal"
	pb "github.com/drivenets/vmw_tsf/pkg/hal/proto"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var monFlowsOpt bool
var monIfcOpt bool
var monInterval float64
var steerFromOpt string
var steerToOpt string
var steerProtoOpt string
var steerNextHopOpt string
var noClsOpt bool
var noTwampOpt bool
var serverOpt bool
var countOpt int
var accOpt bool
var tunNameOpt string
var tunTypeOpt string
var tunSrcOpt string
var tunDstOpt string
var tunHaloIfcOpt string
var ifcUpperOpt string
var ifcLowerOpt string
var ifcNextHopOpt string
var ifcTwampOpt string
var ifcNameOpt string
var flowThreshold uint64 = hal.DEFAULT_FLOW_THRESHOLD

func handleSteer(h hal.DnHal) {

	if steerFromOpt == "" {
		return
	}

	if steerToOpt == "" {
		panic("missing flow destination")
	}

	var err error
	var port uint64

	fk := hal.FlowKey{}

	tokens := strings.Split(steerFromOpt, ":")
	if len(tokens) != 2 {
		panic("failed to parse flow source")
	}
	fk.SrcAddr = net.ParseIP(tokens[0])
	if port, err = strconv.ParseUint(tokens[1], 10, 16); err != nil {
		panic("failed to parse flow source port")
	}
	fk.SrcPort = uint16(port)

	tokens = strings.Split(steerToOpt, ":")
	if len(tokens) != 2 {
		panic("failed to parse flow source")
	}
	fk.DstAddr = net.ParseIP(tokens[0])
	if port, err = strconv.ParseUint(tokens[1], 10, 16); err != nil {
		panic("failed to parse flow source port")
	}
	fk.DstPort = uint16(port)

	switch steerProtoOpt {
	case "tcp":
		fk.Protocol = hal.TCP
	case "udp":
		fk.Protocol = hal.UDP
	default:
		panic("unexpected flow protocol")
	}

	if steerNextHopOpt == "" {
		err = h.RemoveSteer([]hal.FlowKey{fk})
	} else {
		log.Infof("steer %v to %s", fk, steerNextHopOpt)
		steerItem := hal.SteerItem{Rule: &fk, NextHop: steerNextHopOpt}
		err = h.Steer([]hal.SteerItem{steerItem})
		log.Infof("verifying rule %v to %s has been applied", fk, steerNextHopOpt)
		log.Infoln(h.GetSteerInterface([]hal.SteerItem{steerItem}))
	}
	if err != nil {
		panic(err)
	}

	//fk1 := hal.FlowKey{Protocol: hal.TCP, SrcAddr: net.ParseIP("1.4.4.8"), SrcPort: 81, DstAddr: net.ParseIP("4.4.4.9"), DstPort: 91}
	//fk2 := hal.FlowKey{Protocol: hal.TCP, SrcAddr: net.ParseIP("2.4.5.8"), SrcPort: 81, DstAddr: net.ParseIP("4.4.4.9"), DstPort: 91}
	//fk3 := hal.FlowKey{Protocol: hal.TCP, SrcAddr: net.ParseIP("3.4.6.8"), SrcPort: 81, DstAddr: net.ParseIP("4.4.4.9"), DstPort: 91}
	//fk4 := hal.FlowKey{Protocol: hal.TCP, SrcAddr: net.ParseIP("4.4.7.8"), SrcPort: 81, DstAddr: net.ParseIP("4.4.4.9"), DstPort: 91}
	//
	//var acl = []hal.SteerItem{
	//	{Rule: &fk1, NextHop: steerNextHopOpt},
	//	{Rule: &fk2, NextHop: steerNextHopOpt},
	//	{Rule: &fk3, NextHop: ""},
	//	{Rule: &fk4, NextHop: steerNextHopOpt}}
	//err = h.Steer(acl)
	//if err != nil {
	//	panic(err)
	//}
	//
	//fmt.Println(h.GetSteerInterface(acl))
	//
	//err = h.RemoveSteer([]hal.FlowKey{fk1, fk2, fk3, fk})
	//if err != nil {
	//	panic(err)
	//}
	//
	//fmt.Println(h.GetSteerInterface(acl))
}

func clearScreen() {
	if noClsOpt {
		return
	}
	fmt.Print("\033[H\033[2J")
}

func printInterfaces(h hal.DnHal) {
	fmt.Println()
	fmt.Println()
	w := tabwriter.NewWriter(os.Stdout, 1, 1, 1, ' ', 0)
	fmt.Fprintln(w, "Interface\tType\tSpeed\tRx.Bits/sec\tRx.Bytes/total\tTx.Bits/sec\tTx.Bytes/total\tDelay\tJitter")
	fmt.Fprintln(w, "---------\t----\t-----\t-----------\t--------------\t-----------\t--------------\t-----\t------")
	count := 0
	ifType := ""
	getIfStats := func(ifc string, tm *hal.InterfaceTelemetry) error {
		count += 1
		fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%d\t%d\t%d\t%f\t%f\n",
			ifc, ifType, tm.Speed,
			tm.RxBps, tm.RxBytes,
			tm.TxBps, tm.TxBytes,
			tm.Link.Delay, tm.Link.Jitter)
		return nil
	}
	ifType = "lan"
	h.GetLanInterfaces(getIfStats)
	ifType = "wan"
	h.GetWanInterfaces(getIfStats)
	w.Flush()
	if count == 0 {
		fmt.Println("(none)")
	}
}

func printFlows(h hal.DnHal) {
	fmt.Println()
	w := tabwriter.NewWriter(os.Stdout, 1, 1, 1, ' ', 0)
	fmt.Fprintln(w, "Source\tDestination\tProto\tIngress\tEgress\tPkt/sec\tPkt/total\tBytes/sec\tBytes/total")
	fmt.Fprintln(w, "------\t-----------\t-----\t-------\t------\t-------\t---------\t---------\t-----------")
	flows := make([]string, 0, 1024)
	h.GetFlows(
		func(key *hal.FlowKey, stat *hal.FlowTelemetry) error {
			flows = append(flows,
				fmt.Sprintf("%s:%d\t%s:%d\t%s\t%s\t%s\t%d\t%d\t%d\t%d\n",
					key.SrcAddr, key.SrcPort,
					key.DstAddr, key.DstPort,
					key.Protocol,
					stat.IngressIf, stat.EgressIf,
					stat.RxRatePps, stat.RxTotalPkts,
					stat.RxRateBps, stat.RxTotalBytes))
			return nil
		})
	sort.Strings(flows)
	for _, fl := range flows {
		fmt.Fprint(w, fl)
	}
	w.Flush()
	if len(flows) == 0 {
		fmt.Println("(none)")
	}
}

func monitor(h hal.DnHal) {
	for {
		clearScreen()
		fmt.Println(time.Now())
		if monIfcOpt {
			printInterfaces(h)
		}
		if monFlowsOpt {
			printFlows(h)
		}
		time.Sleep(time.Duration(monInterval * float64(time.Second)))
	}
}

type BatchInterfaceOutput struct {
	Name    string        `json:"name"`
	Type    string        `json:"type"`
	Speed   uint64        `json:"speed"`
	RxBytes uint64        `json:"rx-bytes"`
	RxBps   uint64        `json:"rx-bps"`
	TxBytes uint64        `json:"tx-bytes"`
	TxBps   uint64        `json:"tx-bps"`
	Delay   float64       `json:"delay"`
	Jitter  float64       `json:"jitter"`
	Time    time.Duration `json:"-"` // accumulated time
}

type BatchFlowOutput struct {
	Source      string        `json:"source"`
	Destination string        `json:"destination"`
	Protocol    string        `json:"protocol"`
	Ingress     string        `json:"ingress"`
	Egress      string        `json:"egress"`
	RxPps       uint64        `json:"rx-pps"`
	RxPkts      uint64        `json:"rx-packets"`
	RxBps       uint64        `json:"rx-bps"`
	RxBytes     uint64        `json:"rx-bytes"`
	Time        time.Duration `json:"-"` // accumulated time
}

type BatchOutput struct {
	Time       float64                `json:"time"`
	Interfaces []BatchInterfaceOutput `json:"interfaces"`
	Flows      []BatchFlowOutput      `json:"flows"`
}

func jsonInterfaces(h hal.DnHal, b *BatchOutput, t time.Duration) {
	ifType := "n/a"
	getIfStats := func(ifc string, tm *hal.InterfaceTelemetry) error {
		b.Interfaces = append(b.Interfaces, BatchInterfaceOutput{
			Name:    ifc,
			Type:    ifType,
			Speed:   tm.Speed,
			RxBps:   tm.RxBps,
			RxBytes: tm.RxBytes,
			TxBps:   tm.TxBps,
			TxBytes: tm.TxBytes,
			Delay:   tm.Link.Delay,
			Jitter:  tm.Link.Jitter,
			Time:    t,
		})
		return nil
	}
	ifType = "lan"
	h.GetLanInterfaces(getIfStats)
	ifType = "wan"
	h.GetWanInterfaces(getIfStats)
}

func jsonFlows(h hal.DnHal, b *BatchOutput, t time.Duration) {
	h.GetFlows(
		func(key *hal.FlowKey, stat *hal.FlowTelemetry) error {
			b.Flows = append(b.Flows, BatchFlowOutput{
				Source:      fmt.Sprintf("%s:%d", key.SrcAddr, key.SrcPort),
				Destination: fmt.Sprintf("%s:%d", key.DstAddr, key.DstPort),
				Protocol:    key.Protocol.String(),
				Ingress:     stat.IngressIf,
				Egress:      stat.EgressIf,
				RxPps:       stat.RxRatePps,
				RxPkts:      stat.RxTotalPkts,
				RxBps:       stat.RxRateBps,
				RxBytes:     stat.RxTotalBytes,
				Time:        t,
			})
			return nil
		})
}

type AccumulatedOutput struct {
	Time       float64                         `json:"time"`
	Interfaces map[string]BatchInterfaceOutput `json:"interfaces"`
	Flows      map[string]BatchFlowOutput      `json:"flows"`
}

func NewAccumulatedOutput() AccumulatedOutput {
	acc := AccumulatedOutput{}
	acc.Interfaces = make(map[string]BatchInterfaceOutput)
	acc.Flows = make(map[string]BatchFlowOutput)
	return acc
}

func (acc *AccumulatedOutput) Copy() AccumulatedOutput {
	cp := NewAccumulatedOutput()
	cp.Time = acc.Time
	for key, val := range acc.Interfaces {
		cp.Interfaces[key] = val
	}
	for key, val := range acc.Flows {
		cp.Flows[key] = val
	}
	return cp
}

type AverageF64 struct {
	weightA float64
	weightB float64
}

func MakeAverageF64(wa float64, wb float64) AverageF64 {
	return AverageF64{
		weightA: wa,
		weightB: wb,
	}
}

func (avg AverageF64) F64(a float64, b float64) float64 {
	return (a*avg.weightA + b*avg.weightB) / (avg.weightA + avg.weightB)
}

func (avg AverageF64) U64(a uint64, b uint64) uint64 {
	return uint64((float64(a)*avg.weightA + float64(b)*avg.weightB) / (avg.weightA + avg.weightB))
}

func updateAccInterfaces(acc AccumulatedOutput, entry *BatchOutput, interval time.Duration, next AccumulatedOutput) {
	for _, ifc := range entry.Interfaces {
		if accIf, found := acc.Interfaces[ifc.Name]; found {
			// Update existing interfaces
			nextIf := BatchInterfaceOutput{
				Name: ifc.Name,
				Type: ifc.Type,
			}

			avg := MakeAverageF64(accIf.Time.Seconds(), ifc.Time.Seconds())
			nextIf.Speed = avg.U64(accIf.Speed, ifc.Speed)
			nextIf.RxBps = avg.U64(accIf.RxBps, ifc.RxBps)
			nextIf.RxBytes = avg.U64(accIf.RxBytes, ifc.RxBytes)
			nextIf.TxBps = avg.U64(accIf.TxBps, ifc.TxBps)
			nextIf.TxBytes = avg.U64(accIf.TxBytes, ifc.TxBytes)
			nextIf.Delay = avg.F64(accIf.Delay, ifc.Delay)
			nextIf.Jitter = avg.F64(accIf.Jitter, ifc.Jitter)

			next.Interfaces[nextIf.Name] = nextIf
		} else {
			// Track new interfaces
			next.Interfaces[ifc.Name] = ifc
		}
	}
	for _, accIf := range acc.Interfaces {
		if _, found := next.Interfaces[accIf.Name]; !found {
			// Update average of interfaces not in current update
			nextIf := BatchInterfaceOutput{
				Name: accIf.Name,
				Type: accIf.Type,
			}
			avg := MakeAverageF64(accIf.Time.Seconds(), interval.Seconds())
			nextIf.Speed = avg.U64(accIf.Speed, 0)
			nextIf.RxBps = avg.U64(accIf.RxBps, 0)
			nextIf.RxBytes = avg.U64(accIf.RxBytes, 0)
			nextIf.TxBps = avg.U64(accIf.TxBps, 0)
			nextIf.TxBytes = avg.U64(accIf.TxBytes, 0)
			nextIf.Delay = avg.F64(accIf.Delay, 0)
			nextIf.Jitter = avg.F64(accIf.Jitter, 0)

			next.Interfaces[nextIf.Name] = nextIf
		}
	}
}

func (fo *BatchFlowOutput) Key() string {
	return fmt.Sprintf("%s:%s:%s", fo.Source, fo.Destination, fo.Protocol)
}

func updateAccFlows(acc AccumulatedOutput, entry *BatchOutput, interval time.Duration, next AccumulatedOutput) {
	for _, flow := range entry.Flows {
		key := flow.Key()
		if accFlow, found := acc.Flows[key]; found {
			// Update existing flows
			nextFlow := BatchFlowOutput{
				Source:      flow.Source,
				Destination: flow.Destination,
				Protocol:    flow.Protocol,
				Ingress:     flow.Ingress,
				Egress:      flow.Egress,
			}

			avg := MakeAverageF64(accFlow.Time.Seconds(), flow.Time.Seconds())
			nextFlow.RxPps = avg.U64(accFlow.RxPps, flow.RxPps)
			nextFlow.RxPkts = avg.U64(accFlow.RxPkts, flow.RxPkts)
			nextFlow.RxBps = avg.U64(accFlow.RxBps, flow.RxBps)
			nextFlow.RxBytes = avg.U64(accFlow.RxBytes, flow.RxBytes)
			next.Flows[key] = nextFlow
		} else {
			// Track new flows
			next.Flows[key] = flow
		}
	}
	for _, accFlow := range acc.Flows {
		key := accFlow.Key()
		if _, found := next.Flows[key]; !found {
			// Update average of flows not in current update
			nextFlow := BatchFlowOutput{
				Source:      accFlow.Source,
				Destination: accFlow.Destination,
				Protocol:    accFlow.Protocol,
				Ingress:     accFlow.Ingress,
				Egress:      accFlow.Egress,
			}
			avg := MakeAverageF64(accFlow.Time.Seconds(), interval.Seconds())
			nextFlow.RxPps = avg.U64(accFlow.RxPps, 0)
			nextFlow.RxPkts = avg.U64(accFlow.RxPkts, 0)
			nextFlow.RxBps = avg.U64(accFlow.RxBps, 0)
			nextFlow.RxBytes = avg.U64(accFlow.RxBytes, 0)

			next.Flows[key] = nextFlow
		}
	}
}

func updateAccOutput(acc AccumulatedOutput, entry *BatchOutput, interval time.Duration) AccumulatedOutput {
	next := NewAccumulatedOutput()
	updateAccInterfaces(acc, entry, interval, next)
	updateAccFlows(acc, entry, interval, next)
	return next
}

func batch(h hal.DnHal) {
	batch := make([]BatchOutput, countOpt)
	accLog := make([]AccumulatedOutput, countOpt)
	acc := NewAccumulatedOutput()
	start := time.Now()
	prev := time.Now()
	for idx := 0; idx < countOpt; idx++ {
		time.Sleep(time.Duration(monInterval * float64(time.Second)))
		elapsed := time.Since(start)
		tick := time.Since(prev)
		prev = time.Now()

		entry := BatchOutput{
			Time:       elapsed.Seconds(),
			Interfaces: make([]BatchInterfaceOutput, 0, 3),
			Flows:      make([]BatchFlowOutput, 0, 1000)}
		prev = time.Now()
		if monIfcOpt {
			jsonInterfaces(h, &entry, tick)
		}
		if monFlowsOpt {
			jsonFlows(h, &entry, tick)
		}
		batch[idx] = entry
		if accOpt {
			acc = updateAccOutput(acc, &entry, tick)
			acc.Time = elapsed.Seconds()
			accLog[idx] = acc.Copy()
		}
	}

	if accOpt {
		if out, err := json.Marshal(accLog); err != nil {
			log.Error("Failed to marshal json output. Reason:", err)
		} else {
			fmt.Printf("%s\n", out)
		}
	} else {
		if out, err := json.Marshal(batch); err != nil {
			log.Error("Failed to marshal json output. Reason:", err)
		} else {
			fmt.Printf("%s\n", out)
		}
	}
}

func handleTunnelAdd(h hal.DnHal) error {
	var err error

	if len(tunNameOpt) == 0 {
		return fmt.Errorf("empty tunnel name")
	}

	if len(tunSrcOpt) == 0 {
		return fmt.Errorf("empty tunnel source address")
	}
	src := net.ParseIP(tunSrcOpt)
	if src == nil {
		return fmt.Errorf("invalid tunnel source address: %s", tunSrcOpt)
	}
	if len(tunDstOpt) == 0 {
		return fmt.Errorf("empty tunnel destination address")
	}
	dst := net.ParseIP(tunDstOpt)
	if dst == nil {
		return fmt.Errorf("invalid tunnel destination address: %s", tunSrcOpt)
	}

	if tunTypeOpt != "rsvp" {
		return fmt.Errorf("unsupported tunnel type: %s (try 'rsvp' instead)", tunTypeOpt)
	}

	if len(tunHaloIfcOpt) == 0 {
		return fmt.Errorf("empty tunnel halo address & mask")
	}
	haloAddr, haloNet, err := net.ParseCIDR(tunHaloIfcOpt)
	if err != nil {
		return err
	}
	return h.AddTunnel(tunNameOpt, src, dst, hal.VXLAN, haloAddr, *haloNet)
}

func handleTunnelDelete(h hal.DnHal) error {
	if len(tunNameOpt) == 0 {
		return fmt.Errorf("empty tunnel name")
	}

	if tunTypeOpt != "rsvp" {
		return fmt.Errorf("unsupported tunnel type: %s (try 'rsvp' instead)", tunTypeOpt)
	}

	return h.DeleteTunnel(tunNameOpt, hal.VXLAN)
}

func handleInterfaceAddWan() error {
	var ifcNameOpt string
	if len(ifcUpperOpt) == 0 {
		return fmt.Errorf("empty upper interface name")
	}
	if len(ifcLowerOpt) == 0 {
		return fmt.Errorf("empty lower interface name")
	}
	if len(ifcNextHopOpt) == 0 {
		return fmt.Errorf("empty next hop")
	}
	if len(ifcTwampOpt) == 0 {
		return fmt.Errorf("empty twamp endpoint")
	}
	mgr := hal.GetMgmtClient()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tmo, _ := context.WithTimeout(ctx, time.Duration(1)*time.Second)
	status, err := mgr.AddWanInterface(tmo, &pb.AddWanInterfaceArgs{
		Upper:   ifcUpperOpt,
		Lower:   ifcLowerOpt,
		NextHop: ifcNextHopOpt,
		Twamp:   ifcTwampOpt,
	})
	if err != nil {
		return fmt.Errorf("failed to add interface: status=%v, err=%v", status, err)
	}
	log.Infof("interface %s added", ifcNameOpt)
	return nil
}

func handleInterfaceAddLan() error {
	var ifcNameOpt string
	if len(ifcUpperOpt) == 0 {
		return fmt.Errorf("empty upper interface name")
	}
	if len(ifcLowerOpt) == 0 {
		return fmt.Errorf("empty lower interface name")
	}
	mgr := hal.GetMgmtClient()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tmo, _ := context.WithTimeout(ctx, time.Duration(1)*time.Second)
	status, err := mgr.AddLanInterface(tmo, &pb.AddLanInterfaceArgs{
		Upper: ifcUpperOpt,
		Lower: ifcLowerOpt,
	})
	if err != nil {
		return fmt.Errorf("failed to add interface: status=%v, err=%v", status, err)
	}
	log.Infof("interface %s added", ifcNameOpt)
	return nil
}

func handleInterfaceDelete() error {
	if len(ifcNameOpt) == 0 {
		return fmt.Errorf("empty interface name")
	}
	mgr := hal.GetMgmtClient()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tmo, _ := context.WithTimeout(ctx, time.Duration(1)*time.Second)
	status, err := mgr.DeleteInterface(tmo, &pb.DeleteInterfaceArgs{
		Upper: ifcNameOpt,
	})
	if err != nil {
		return fmt.Errorf("failed to remove interface: status=%v, err=%v", status, err)
	}
	log.Infof("interface %s removed", ifcNameOpt)
	return nil
}

func main() {
	rootCmd := &cobra.Command{Use: "hal-client"}

	cmdMonitor := &cobra.Command{
		Use:   "monitor",
		Short: "Monitor interface and flows statistics",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !(monFlowsOpt || monIfcOpt) {
				return fmt.Errorf("nothing to monitor")
			}
			if noTwampOpt {
				os.Setenv("SKIP_TWAMP", "1")
			}
			os.Setenv("IFC_SAMPLE", strconv.FormatFloat(monInterval, 'f', 3, 64))
			opts := make([]hal.OptionHal, 0, 3)
			opts = append(opts, hal.OptionHalFlushSteer(false))
			opts = append(opts, hal.OptionHalStatsServer(serverOpt))
			opts = append(opts, hal.OptionHalFlowThreshold(flowThreshold))
			h := hal.NewDnHal(opts...)
			monitor(h)
			return nil
		},
	}
	cmdMonitor.Flags().BoolVarP(&monFlowsOpt, "flows", "f", false, "monitor flows")
	cmdMonitor.Flags().BoolVarP(&monIfcOpt, "interfaces", "i", false, "monitor interfaces")
	cmdMonitor.Flags().Float64VarP(&monInterval, "interval", "t", 1, "monitoring interval")
	cmdMonitor.Flags().BoolVarP(&noTwampOpt, "notw", "w", false, "do not start twamp measurements")
	cmdMonitor.Flags().BoolVarP(&noClsOpt, "nocls", "c", false, "do not clear screen")
	cmdMonitor.Flags().BoolVarP(&serverOpt, "server", "s", false, "start stats/management server")
	cmdMonitor.Flags().Uint64VarP(&flowThreshold, "threshold", "l", hal.DEFAULT_FLOW_THRESHOLD, "flow detection threshold (minimum rx or tx bandwidth)")
	rootCmd.AddCommand(cmdMonitor)

	cmdFlush := &cobra.Command{
		Use:   "flush",
		Short: "Flush steering rules",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			os.Setenv("SKIP_TWAMP", "1")
			opts := make([]hal.OptionHal, 0, 2)
			opts = append(opts, hal.OptionHalFlushSteer(true))
			opts = append(opts, hal.OptionHalStatsServer(false))
			hal.NewDnHal(opts...)
		},
	}
	rootCmd.AddCommand(cmdFlush)

	cmdSteer := &cobra.Command{
		Use:   "steer",
		Short: "Steer traffic flow",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			os.Setenv("SKIP_TWAMP", "1")
			opts := make([]hal.OptionHal, 0, 2)
			opts = append(opts, hal.OptionHalFlushSteer(false))
			opts = append(opts, hal.OptionHalStatsServer(false))
			h := hal.NewDnHal(opts...)
			handleSteer(h)
		},
	}
	cmdSteer.Flags().StringVarP(&steerFromOpt, "from", "f", "", "steer flow source ip:port")
	cmdSteer.Flags().StringVarP(&steerToOpt, "to", "t", "", "steer flow destination ip:port")
	cmdSteer.Flags().StringVarP(&steerProtoOpt, "proto", "p", "tcp", "steer flow protocol")
	cmdSteer.Flags().StringVarP(&steerNextHopOpt, "next-hop", "n", "", "steer to next-hop")
	rootCmd.AddCommand(cmdSteer)

	cmdBatch := &cobra.Command{
		Use:   "batch",
		Short: "machine-friendly monitor output",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !(monFlowsOpt || monIfcOpt) {
				return fmt.Errorf("nothing to monitor")
			}
			if noTwampOpt {
				os.Setenv("SKIP_TWAMP", "1")
			}
			os.Setenv("IFC_SAMPLE", strconv.FormatFloat(monInterval, 'f', 3, 64))
			opts := make([]hal.OptionHal, 0, 3)
			opts = append(opts, hal.OptionHalFlushSteer(false))
			opts = append(opts, hal.OptionHalStatsServer(false))
			opts = append(opts, hal.OptionHalFlowThreshold(flowThreshold))
			h := hal.NewDnHal(opts...)
			batch(h)
			return nil
		},
	}
	cmdBatch.Flags().BoolVarP(&monFlowsOpt, "flows", "f", false, "monitor flows")
	cmdBatch.Flags().BoolVarP(&monIfcOpt, "interfaces", "i", false, "monitor interfaces")
	cmdBatch.Flags().Float64VarP(&monInterval, "interval", "t", 1, "monitoring interval")
	cmdBatch.Flags().BoolVarP(&noTwampOpt, "notw", "w", false, "do not start twamp measurements")
	cmdBatch.Flags().IntVarP(&countOpt, "count", "c", 1, "how many intervals to run")
	cmdBatch.Flags().BoolVarP(&accOpt, "accumulate", "a", false, "accumulate flow statistics in batch mode")
	cmdBatch.Flags().Uint64VarP(&flowThreshold, "threshold", "l", hal.DEFAULT_FLOW_THRESHOLD, "flow detection threshold (minimum rx or tx bandwidth)")
	rootCmd.AddCommand(cmdBatch)

	cmdTunnel := &cobra.Command{
		Use:   "tunnel",
		Short: "Tunnel commands",
		Args:  cobra.NoArgs,
	}
	cmdTunnelAdd := &cobra.Command{
		Use:   "add",
		Short: "Add tunnel",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			os.Setenv("SKIP_TWAMP", "1")
			opts := make([]hal.OptionHal, 0, 2)
			opts = append(opts, hal.OptionHalFlushSteer(false))
			opts = append(opts, hal.OptionHalStatsServer(false))
			opts = append(opts, hal.OptionHalConfigRequired(false))
			h := hal.NewDnHal(opts...)
			return handleTunnelAdd(h)
		},
	}
	cmdTunnelAdd.Flags().StringVarP(&tunNameOpt, "name", "n", "", "Tunnel name")
	cmdTunnelAdd.MarkFlagRequired("name")
	cmdTunnelAdd.Flags().StringVarP(&tunTypeOpt, "type", "t", "rsvp", "Tunnel type")
	cmdTunnelAdd.Flags().StringVarP(&tunSrcOpt, "source", "s", "", "Tunnel source IP")
	cmdTunnelAdd.MarkFlagRequired("source")
	cmdTunnelAdd.Flags().StringVarP(&tunDstOpt, "destination", "d", "", "Tunnel destination IP")
	cmdTunnelAdd.MarkFlagRequired("destination")
	cmdTunnelAdd.Flags().StringVarP(&tunHaloIfcOpt, "halo", "i", "", "Halo interface IP & netmask")
	cmdTunnelAdd.MarkFlagRequired("halo")
	cmdTunnel.AddCommand(cmdTunnelAdd)
	cmdTunnelDelete := &cobra.Command{
		Use:   "delete",
		Short: "Delete tunnel",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			os.Setenv("SKIP_TWAMP", "1")
			opts := make([]hal.OptionHal, 0, 2)
			opts = append(opts, hal.OptionHalFlushSteer(false))
			opts = append(opts, hal.OptionHalStatsServer(false))
			opts = append(opts, hal.OptionHalConfigRequired(false))
			h := hal.NewDnHal(opts...)
			return handleTunnelDelete(h)
		},
	}
	cmdTunnelDelete.Flags().StringVarP(&tunNameOpt, "name", "n", "", "Tunnel name")
	cmdTunnelDelete.MarkFlagRequired("name")
	cmdTunnelDelete.Flags().StringVarP(&tunTypeOpt, "type", "t", "rsvp", "Tunnel type")
	cmdTunnel.AddCommand(cmdTunnelDelete)
	rootCmd.AddCommand(cmdTunnel)

	cmdInterface := &cobra.Command{
		Use:   "interface",
		Short: "Interface commands",
		Args:  cobra.NoArgs,
	}
	cmdInterfaceAdd := &cobra.Command{
		Use:   "add",
		Short: "Add interface",
		Args:  cobra.NoArgs,
	}
	cmdInterfaceAddWan := &cobra.Command{
		Use:   "wan",
		Short: "Add Wan interface",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleInterfaceAddWan()
		},
	}
	cmdInterfaceAddWan.Flags().StringVarP(&ifcUpperOpt, "upper", "u", "", "Interface upper name (inside container)")
	cmdInterfaceAddWan.MarkFlagRequired("upper")
	cmdInterfaceAddWan.Flags().StringVarP(&ifcLowerOpt, "lower", "l", "", "Interface lower name (DNOS attached)")
	cmdInterfaceAddWan.MarkFlagRequired("lower")
	cmdInterfaceAddWan.Flags().StringVarP(&ifcNextHopOpt, "next-hop", "n", "", "IP address of lower interface (as seen by flows)")
	cmdInterfaceAddWan.MarkFlagRequired("next-hop")
	cmdInterfaceAddWan.Flags().StringVarP(&ifcTwampOpt, "twamp", "t", "", "TWAMP endpoint for interface latency")
	cmdInterfaceAddWan.MarkFlagRequired("twamp")
	cmdInterfaceAdd.AddCommand(cmdInterfaceAddWan)
	cmdInterfaceAddLan := &cobra.Command{
		Use:   "lan",
		Short: "Add Lan interface",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleInterfaceAddLan()
		},
	}
	cmdInterfaceAddLan.Flags().StringVarP(&ifcUpperOpt, "upper", "u", "", "Interface upper name (inside container)")
	cmdInterfaceAddLan.MarkFlagRequired("upper")
	cmdInterfaceAddLan.Flags().StringVarP(&ifcLowerOpt, "lower", "l", "", "Interface lower name (DNOS attached)")
	cmdInterfaceAddLan.MarkFlagRequired("lower")
	cmdInterfaceAdd.AddCommand(cmdInterfaceAddLan)
	cmdInterfaceDelete := &cobra.Command{
		Use:   "delete",
		Short: "Delete interface",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleInterfaceDelete()
		},
	}
	cmdInterfaceDelete.Flags().StringVarP(&ifcNameOpt, "name", "n", "", "Interface name")
	cmdInterfaceDelete.MarkFlagRequired("name")
	cmdInterface.AddCommand(cmdInterfaceAdd)
	cmdInterface.AddCommand(cmdInterfaceDelete)
	rootCmd.AddCommand(cmdInterface)

	// see https://github.com/golang/glog/commit/fca8c8854093a154ff1eb580aae10276ad6b1b5f
	flag.CommandLine.Parse([]string{})
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
