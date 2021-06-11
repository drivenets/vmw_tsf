package hal

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"github.com/Juniper/go-netconf/netconf"
	"github.com/openconfig/gnmi/proto/gnmi"
	"github.com/openconfig/ygot/ygot"
	"golang.org/x/crypto/ssh"
	"google.golang.org/grpc"
	//stdLog "log"
	"net"
	"sort"
	"strings"

	"os"
	"strconv"
	"sync"
	"time"

	gfu "github.com/cloudflare/goflow/v3/utils"
	log "github.com/sirupsen/logrus"

	flowmessage "github.com/cloudflare/goflow/v3/pb"

	twamp "github.com/drivenets/vmw_tsf/tsf-twamp"
)

type FlowAggregate struct {
	start time.Time

	key   *FlowKey
	inIf  string
	outIf string

	packets uint64
	bytes   uint64
}

func (agg *FlowAggregate) ToTelemetry() *FlowTelemetry {
	millis := uint64(time.Since(agg.start).Milliseconds())
	if millis < 1 {
		millis = 1
	}
	return &FlowTelemetry{
		// Rate
		RxRatePps: (((agg.packets + 1) * 1000) - 1) / millis,
		TxRatePps: 0,
		RxRateBps: (((agg.bytes + 1) * 1000) - 1) / millis,
		TxRateBps: 0,

		// Total counters
		RxTotalPkts:  agg.packets,
		TxTotalPkts:  0,
		RxTotalBytes: agg.bytes,
		TxTotalBytes: 0,

		// Interfaces
		IngressIf: agg.inIf,
		EgressIf:  agg.outIf,
	}
}

const NETFLOW_ID_INVALID = 0xFFFFFFFF

type Interface struct {
	Upper     string
	Lower     string
	NetFlowId uint32
	NextHop   net.IP
	Stats     InterfaceTelemetry
	Twamp     struct {
		Peer string
		Port uint16
	}
}

type DnHalImpl struct {
	mutex       sync.Mutex
	initialized bool
	initOptions struct {
		flushSteer bool
	}
	grpcAddr   string
	interfaces struct {
		UpdateInterval time.Duration
		Map            struct {
			Lock        sync.RWMutex
			UpperSorted struct {
				Lan []string
				Wan []string
			}
			Upper2Interface   map[string]*Interface
			Lower2Interface   map[string]*Interface
			NetFlow2Interface map[uint32]*Interface
			NextHop2Interface map[string]*Interface
		}
	}
	flows struct {
		state     *gfu.StateNetFlow
		aggregate map[string]*FlowAggregate
	}
	aclRules map[int]string
}

var hal = &DnHalImpl{}

type OptionHal func(*DnHalImpl) error

func OptionHalFlushSteer() OptionHal {
	return func(hal *DnHalImpl) error {
		hal.initOptions.flushSteer = true
		return nil
	}
}

func NewDnHal(options ...OptionHal) DnHal {
	for _, op := range options {
		err := op(hal)
		if err != nil {
			log.Fatalf("Failed to setup HAL instance. Reason: ", err)
		}
	}

	if !hal.initialized {
		hal.Init()
	}
	return hal
}

const DRIVENETS_DNOS_ADDR = "localhost"

func (hal *DnHalImpl) InitFlows() {
	hal.flows.state = &gfu.StateNetFlow{
		Transport: hal,
		Logger:    log.StandardLogger(),
	}

	hal.flows.aggregate = make(map[string]*FlowAggregate)
}

func (hal *DnHalImpl) InitNetConf() {
	log.Info("Initializing netconf client")
	session := NetConfConnector()

	obj := GetConfig{
		Filter: &Filter{
			DrivenetsTop: &DrivenetsTop{
				AttrXmlnsdnAccessControlList: "http://drivenets.com/ns/yang/dn-access-control-list",
				AccessListsDnAccessControlList: AccessListsDnAccessControlList{
					Ipv4: &Ipv4{
						AccessList: AccessList{Name: HALO_ACL_BUCKET_NAME},
					},
				},
			},
		},
		Source: &Source{RunningConfig: &RunningConfig{}},
	}

	xmlString, _ := xml.MarshalIndent(obj, "", "    ")
	data, err := session.Exec(netconf.RawMethod(xmlString))
	if err != nil {
		log.Errorf("Failed to retrieve ACL rules, err: %v", err)
		panic(err)
	}

	var response Data
	if err := xml.Unmarshal([]byte(data.Data), &response); err != nil {
		panic(err)
	}

	protocols := map[string]FlowProto{"tcp(0x06)": TCP, "udp(0x11)": UDP}
	for _, v := range response.DrivenetsTopReply.AccessListsDnAccessControlListReply.Ipv4.AccessList.Rules.Rule {
		srcIpv4Addr, _, err := net.ParseCIDR(v.RuleConfigItems.Ipv4Matches.Ipv4AclMatch.SourceIpv4)
		if err != nil {
			log.Warn(v, err)
			continue
		}

		DstIpv4Addr, _, err := net.ParseCIDR(v.RuleConfigItems.Ipv4Matches.Ipv4AclMatch.DestinationIpv4)
		if err != nil {
			log.Warn(err)
			continue
		}

		fk := FlowKey{
			Protocol: protocols[v.RuleConfigItems.Protocol],
			SrcAddr:  srcIpv4Addr,
			DstAddr:  DstIpv4Addr,
			SrcPort:  v.RuleConfigItems.Matches.L4AclMatch.SourcePortRange.LowerPort,
			DstPort:  v.RuleConfigItems.Matches.L4AclMatch.DestinationPortRange.LowerPort,
		}
		log.Printf("Loading rule to cache - id: %d, rule: %s, next-hop: %v", v.RuleId, fk.AsKey(), v.RuleConfigItems.Nexthops.Nexthop1.Addr)
		hal.AclRuleCacheAdd(&fk, v.RuleId)
	}

}

func (hal *DnHalImpl) Init() {
	hal.mutex.Lock()
	defer hal.mutex.Unlock()

	var ok bool
	if hal.grpcAddr, ok = os.LookupEnv("DNOS_ADDR"); !ok {
		hal.grpcAddr = DRIVENETS_DNOS_ADDR
	}
	hal.grpcAddr = hal.grpcAddr + ":50051"
	hal.aclRules = make(map[int]string)
	hal.InitInterfaces()

	if hal.initOptions.flushSteer {
		err := SteeringAclCleanup()
		if err != nil {
			log.Fatalf("failed to cleanup old access lists. Reason: %v", err)
		}
	}
	go monitorInterfaces()

	hal.InitFlows()
	hal.InitNetConf()

	go monitorFlows()

	hal.initialized = true
}

const DRIVENETS_INTERFACE_SAMPLE_INTERVAL = 5
const HALO_INTERFACES_COUNT = 2
const HALO_ACL_BUCKET_NAME = "Steering"

type OptionInterface func(*Interface) error

func OptionInterfaceUpper(name string) OptionInterface {
	return func(ifc *Interface) error {
		ifc.Upper = name
		return nil
	}
}

func OptionInterfaceLower(name string) OptionInterface {
	return func(ifc *Interface) error {
		ifc.Lower = name
		return nil
	}
}

func OptionInterfaceNetFlowId(nfid uint32) OptionInterface {
	return func(ifc *Interface) error {
		ifc.NetFlowId = nfid
		return nil
	}
}

func OptionInterfaceUpdateNetFlowId(client gnmi.GNMIClient) OptionInterface {
	return func(ifc *Interface) error {
		var err error

		if ifc.NetFlowId, err = getDnIfIndex(client, ifc.Lower); err != nil {
			log.Errorf("Failed to get interface index for %s", ifc.Lower)
			return err
		}
		return nil
	}
}

func OptionInterfaceNextHop(nh string) OptionInterface {
	return func(ifc *Interface) error {
		ifc.NextHop = net.ParseIP(nh)
		return nil
	}
}

func OptionInterfaceTwamp(peer string, port string) func(*Interface) error {
	return func(ifc *Interface) error {
		var port64 uint64
		port64, err := strconv.ParseUint(port, 10, 16)
		if err != nil {
			log.Error("Failed to parse TWAMP port %d", port)
			return err
		}
		port16 := uint16(port64)

		ifc.Twamp.Peer = peer
		ifc.Twamp.Port = port16
		return nil
	}
}

func NewInterface(options ...OptionInterface) (*Interface, error) {
	ifc := &Interface{
		NextHop:   net.ParseIP("0.0.0.0"),
		NetFlowId: NETFLOW_ID_INVALID,
	}
	for _, op := range options {
		err := op(ifc)
		if err != nil {
			return nil, err
		}
	}
	hal.interfaces.Map.Lock.Lock()
	defer hal.interfaces.Map.Lock.Unlock()

	hal.interfaces.Map.Lower2Interface[ifc.Lower] = ifc
	hal.interfaces.Map.Upper2Interface[ifc.Upper] = ifc
	if ifc.NetFlowId != NETFLOW_ID_INVALID {
		hal.interfaces.Map.NetFlow2Interface[ifc.NetFlowId] = ifc
	}
	wan := true
	if ip4 := ifc.NextHop.To4(); ip4 != nil {
		if ifc.NextHop.Equal(net.IPv4zero) {
			wan = false
		}
	} else if ifc.NextHop.Equal(net.IPv6zero) {
		wan = false
	}
	if wan {
		hal.interfaces.Map.NextHop2Interface[ifc.Upper] = ifc
		hal.interfaces.Map.UpperSorted.Wan = append(
			hal.interfaces.Map.UpperSorted.Wan, ifc.Upper)
		sort.Strings(hal.interfaces.Map.UpperSorted.Wan)
	} else {
		hal.interfaces.Map.UpperSorted.Lan = append(
			hal.interfaces.Map.UpperSorted.Lan, ifc.Upper)
		sort.Strings(hal.interfaces.Map.UpperSorted.Lan)
	}
	return ifc, nil
}

func (hal *DnHalImpl) InitInterfaces() {
	var haloIf string
	var dnIf string
	var peer string
	var port string
	var ok bool

	hal.interfaces.Map.UpperSorted.Lan = make([]string, 0)
	hal.interfaces.Map.UpperSorted.Wan = make([]string, 0)
	hal.interfaces.Map.Upper2Interface = make(map[string]*Interface)
	hal.interfaces.Map.Lower2Interface = make(map[string]*Interface)
	hal.interfaces.Map.NetFlow2Interface = make(map[uint32]*Interface)
	hal.interfaces.Map.NextHop2Interface = make(map[string]*Interface)

	conn, err := grpc.Dial(hal.grpcAddr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Fatal("failed to connect to gRPC server: %s. Reason: %w", hal.grpcAddr, err)
	}
	defer conn.Close()
	client := gnmi.NewGNMIClient(conn)

	if haloIf, ok = os.LookupEnv("HALO_LOCAL"); ok {
		if dnIf, ok = os.LookupEnv("HALO_LOCAL_IFACE"); !ok {
			log.Fatal("Missing DN interface for HALO_LOCAL")
		}
		_, err := NewInterface(
			OptionInterfaceUpper(haloIf),
			OptionInterfaceLower(dnIf),
			OptionInterfaceUpdateNetFlowId(client),
		)
		if err != nil {
			log.Fatal("Failed to create local interface. Reason:", err)
		}
	}

	var sample string
	var interval uint64 = DRIVENETS_INTERFACE_SAMPLE_INTERVAL
	if sample, ok = os.LookupEnv("IFC_SAMPLE"); ok {
		var err error
		if interval, err = strconv.ParseUint(sample, 10, 64); err != nil {
			log.Fatalf("Failed to parse interface sampling interval: %s", interval)
		}
	}
	hal.interfaces.UpdateInterval = time.Duration(interval) * time.Second

	for idx := 0; idx < HALO_INTERFACES_COUNT; idx++ {
		if dnIf, ok = os.LookupEnv(fmt.Sprintf("HALO%d_IFACE", idx)); !ok {
			dnIf = fmt.Sprintf("ge100-0/0/%d", idx)
		}

		var nextHop1 string
		if nextHop1, ok = os.LookupEnv(fmt.Sprintf("HALO%d_NEXT_HOP", idx)); !ok {
			log.Fatalf("Can not find nexthop in HALO%d_IFACE", idx)
		}

		if peer, ok = os.LookupEnv(fmt.Sprintf("HALO%d_TWAMP", idx)); !ok {
			peer = "0.0.0.0:862"
			log.Error("Failed to get TWAMP server for: ", dnIf)
		}
		if port, ok = os.LookupEnv(fmt.Sprintf("HALO%d_TWAMP_PORT", idx)); !ok {
			port = "10001"
			log.Error("Failed to get TWAMP server UDP ports for: ", dnIf)
		}

		haloIf = fmt.Sprintf("halo%d", idx)
		_, err := NewInterface(
			OptionInterfaceUpper(haloIf),
			OptionInterfaceLower(dnIf),
			OptionInterfaceTwamp(peer, port),
			OptionInterfaceNextHop(nextHop1),
			OptionInterfaceUpdateNetFlowId(client),
		)
		if err != nil {
			log.Fatal("Failed to create interface. Reason:", err)
		}
	}
}

const DRIVENETS_IFINDEX_PATH_TEMPLATE = "/drivenets-top/interfaces/interface[name=%s]/oper-items/if-index"

func getDnIfIndex(client gnmi.GNMIClient, lower string) (uint32, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	var err error

	var pathList []*gnmi.Path
	idxPath := fmt.Sprintf(DRIVENETS_IFINDEX_PATH_TEMPLATE, lower)
	pbPath, err := ygot.StringToPath(idxPath, ygot.StructuredPath, ygot.StringSlicePath)
	if err != nil {
		return 0, fmt.Errorf("failed to convert %q to gNMI Path. Reason: %w", idxPath, err)
	}
	pathList = append(pathList, pbPath)
	resp, err := client.Get(ctx, &gnmi.GetRequest{
		Path:     pathList,
		Encoding: gnmi.Encoding_JSON,
	})
	if err != nil {
		return 0, fmt.Errorf("request failed for path %s", pathList)
	}
	notifs := resp.GetNotification()
	if len(notifs) != 1 {
		return 0, fmt.Errorf("unexpected notifications count: want 1, got %d, path=%s", len(notifs), idxPath)
	}
	updates := notifs[0].GetUpdate()
	if len(updates) != 1 {
		return 0, fmt.Errorf("unexpected updates count: want 1, got %d, path=%s", len(updates), idxPath)
	}
	val := updates[0].GetVal()
	var ifIndex uint32
	if err := json.Unmarshal(val.GetJsonVal(), &ifIndex); err != nil {
		log.Fatalf("failed to convert value: %q. Reason: %w", val, err)
	}
	return ifIndex, nil
}

const DRIVENETS_IFOPER_COUNTERS_PATH_TEMPLATE = "/drivenets-top/interfaces/interface[name=%s]/oper-items/counters/ethernet-counters"
const DRIVENETS_IFOPER_SPEED_PATH_TEMPLATE = "/drivenets-top/interfaces/interface[name=%s]/oper-items/interface-speed"

func interfaceOperPaths(lower string) (*gnmi.Path, *gnmi.Path) {
	speed, _ := ygot.StringToPath(
		fmt.Sprintf(DRIVENETS_IFOPER_SPEED_PATH_TEMPLATE, lower),
		ygot.StructuredPath, ygot.StringSlicePath)
	counters, _ := ygot.StringToPath(
		fmt.Sprintf(DRIVENETS_IFOPER_COUNTERS_PATH_TEMPLATE, lower),
		ygot.StructuredPath, ygot.StringSlicePath)
	return speed, counters
}

func copyLanInterfaces() []*Interface {
	hal.interfaces.Map.Lock.RLock()
	defer hal.interfaces.Map.Lock.RUnlock()

	wan := make([]*Interface, len(hal.interfaces.Map.UpperSorted.Lan))
	for idx, ifc := range hal.interfaces.Map.UpperSorted.Lan {
		wan[idx] = hal.interfaces.Map.Upper2Interface[ifc]
	}
	return wan
}

func copyWanInterfaces() []*Interface {
	hal.interfaces.Map.Lock.RLock()
	defer hal.interfaces.Map.Lock.RUnlock()

	wan := make([]*Interface, len(hal.interfaces.Map.UpperSorted.Wan))
	for idx, ifc := range hal.interfaces.Map.UpperSorted.Wan {
		wan[idx] = hal.interfaces.Map.Upper2Interface[ifc]
	}
	return wan
}

func findInterfaceByLower(lower string) *Interface {
	var ifc *Interface = nil
	var ok bool

	hal.interfaces.Map.Lock.RLock()
	defer hal.interfaces.Map.Lock.RUnlock()

	if ifc, ok = hal.interfaces.Map.Lower2Interface[lower]; ok {
		return ifc
	}
	return nil
}

func findInterfaceByNetFlowId(nfid uint32) *Interface {
	var ifc *Interface = nil
	var ok bool

	hal.interfaces.Map.Lock.RLock()
	defer hal.interfaces.Map.Lock.RUnlock()

	if ifc, ok = hal.interfaces.Map.NetFlow2Interface[nfid]; ok {
		return ifc
	}
	return nil
}

func findInterfaceByNextHop(nh string) *Interface {
	var ifc *Interface = nil
	var ok bool

	hal.interfaces.Map.Lock.RLock()
	defer hal.interfaces.Map.Lock.RUnlock()

	if ifc, ok = hal.interfaces.Map.NextHop2Interface[nh]; ok {
		return ifc
	}
	return nil
}

func updateInterfaceDelayJitter(upper string, delay float64, jitter float64) {
	hal.interfaces.Map.Lock.Lock()
	defer hal.interfaces.Map.Lock.Unlock()

	if ifc, ok := hal.interfaces.Map.Upper2Interface[upper]; ok {
		ifc.Stats.Link.Delay = delay
		ifc.Stats.Link.Jitter = jitter
	}
}

const DRIVENETS_GNMI_UPDATE_LIMIT = 5

func monitorInterfaces() {
	conn, err := grpc.Dial(hal.grpcAddr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	interval := time.Duration(DRIVENETS_GNMI_UPDATE_LIMIT) * time.Second
	if hal.interfaces.UpdateInterval < interval {
		log.Warnf("gNMI can not update faster than %[1]d seconds. Update interface statistics every %[1]d seconds.",
			DRIVENETS_GNMI_UPDATE_LIMIT)
	} else {
		interval = hal.interfaces.UpdateInterval
	}
	im, err := NewInterfaceMonitor(hal.grpcAddr, interval)
	if err != nil {
		log.Fatalf("Failed to start. Reason: %s", err)
	}

	for lower := range hal.interfaces.Map.Lower2Interface {
		im.Add(lower)
	}

	twampTests := make(map[string]*twamp.TwampTest)

	var ok bool
	var skipTwamp string
	var twampConn *twamp.TwampConnection
	if skipTwamp, ok = os.LookupEnv("SKIP_TWAMP"); !ok {
		skipTwamp = "0"
	}

	if skipTwamp != "1" {

		for _, ifc := range copyWanInterfaces() {
			twampClient := twamp.NewClient()
			twampConn, err = twampClient.Connect(ifc.Twamp.Peer)
			if err != nil {
				log.Errorf("Failed to connect TWAMP %s. Reason: %s", ifc.Twamp.Peer, err)
				continue
			}
			defer twampConn.Close()

			twampSession, err := twampConn.CreateSession(
				twamp.TwampSessionConfig{
					SenderPort:   int(ifc.Twamp.Port),
					ReceiverPort: int(ifc.Twamp.Port) + 1,
					Timeout:      1,
					Padding:      42,
					TOS:          0,
				},
			)
			if err != nil {
				log.Error(err)
				continue
			}
			defer twampSession.Stop()

			twampTest, ok := twampSession.CreateTest()
			if ok != nil {
				log.Error(ok)
				continue
			}
			twampTests[ifc.Upper] = twampTest
		}
	}

	djt := time.NewTicker(hal.interfaces.UpdateInterval)
	for {
		select {
		case u := <-im.Speed():
			if ifc := findInterfaceByLower(u.Name); ifc == nil {
				log.Warn("No such interface %s. Skip speed update: %v", u.Name, u.Stats)
				continue
			} else {
				ifc.Stats.Speed = u.Stats.Speed
			}
		case u := <-im.Stats():
			if ifc := findInterfaceByLower(u.Name); ifc == nil {
				log.Warn("No such interface %s. Skip speed update: %v", u.Name, u.Stats)
				continue
			} else {
				ifc.Stats.RxBytes = u.Stats.RxBytes
				ifc.Stats.RxBps = u.Stats.RxBps
				ifc.Stats.TxBytes = u.Stats.TxBytes
				ifc.Stats.TxBps = u.Stats.TxBps
			}
		case <-djt.C:
			if skipTwamp != "1" {
				for upper, tt := range twampTests {
					results := tt.RunX(5)
					updateInterfaceDelayJitter(upper,
						float64(results.Stat.Avg)/float64(time.Millisecond),
						float64(results.Stat.Jitter)/float64(time.Millisecond),
					)
				}
			}
		}
	}
}

func monitorFlows() {
	err := hal.flows.state.FlowRoutine(1, "0.0.0.0", 2055, true)
	if err != nil {
		log.Fatalf("Fatal error: could not monitor flows (%v)", err)
	}
}

func (hal *DnHalImpl) GetInterfaces(v InterfaceVisitor) error {
	var err error
	if err = hal.GetLanInterfaces(v); err != nil {
		return err
	}
	if err = hal.GetWanInterfaces(v); err != nil {
		return err
	}
	return nil
}

func (hal *DnHalImpl) GetLanInterfaces(v InterfaceVisitor) error {
	for _, ifc := range copyLanInterfaces() {
		err := v(ifc.Upper, &ifc.Stats)
		if err != nil {
			return err
		}
	}
	return nil
}

func (hal *DnHalImpl) GetWanInterfaces(v InterfaceVisitor) error {
	for _, ifc := range copyWanInterfaces() {
		err := v(ifc.Upper, &ifc.Stats)
		if err != nil {
			return err
		}
	}
	return nil
}

func (fk *FlowKey) AsKey() string {
	return fmt.Sprintf(
		"%d:%s:%s:%d:%d", fk.Protocol,
		fk.SrcAddr, fk.DstAddr,
		fk.SrcPort, fk.DstPort)
}

func (hal *DnHalImpl) Publish(update []*flowmessage.FlowMessage) {
	for _, msg := range update {
		fk := &FlowKey{
			Protocol: FlowProto(msg.Proto),
			SrcAddr:  msg.SrcAddr,
			DstAddr:  msg.DstAddr,
			SrcPort:  uint16(msg.SrcPort),
			DstPort:  uint16(msg.DstPort),
		}

		var inIf string
		var outIf string
		var ok bool

		if ifc := findInterfaceByNetFlowId(msg.InIf); ifc == nil {
			inIf = "N/A"
		} else {
			inIf = ifc.Upper
		}
		if ifc := findInterfaceByNetFlowId(msg.OutIf); ifc == nil {
			outIf = "N/A"
		} else {
			outIf = ifc.Upper
		}

		// Update flows aggregate
		var agg *FlowAggregate
		key := fk.AsKey()
		aggregate := hal.flows.aggregate
		if agg, ok = aggregate[key]; ok {
			agg.bytes += msg.Bytes
			agg.packets += msg.Packets
		} else {
			agg = &FlowAggregate{
				start:   time.Now(),
				key:     fk,
				inIf:    inIf,
				outIf:   outIf,
				bytes:   msg.Bytes,
				packets: msg.Packets,
			}
			aggregate[key] = agg
		}
	}
}

func (*DnHalImpl) GetFlows(v FlowVisitor) error {
	aggregate := make(map[string]*FlowAggregate)
	// Swap current active with empty one
	aggregate, hal.flows.aggregate = hal.flows.aggregate, aggregate
	for _, agg := range aggregate {
		v(agg.key, agg.ToTelemetry())
	}
	return nil
}

func SetAclRuleIndex(idx int) {
	accessListInitId = idx
}

func commitChanges() error {
	session := NetConfConnector()

	log.Info("Committing changes")
	_, err := session.Exec(netconf.RawMethod(Commit))
	if err != nil {
		if strings.Contains(err.Error(), "Commit failed: empty commit") {
			log.Println(err.Error())
		} else {
			return err
		}
	}

	return nil
}

func getAclXml(rules []Rule) EditConfig {
	return EditConfig{
		Config: Config{
			DrivenetsTop: DrivenetsTop{
				AttrXmlnsdnAccessControlList: "http://drivenets.com/ns/yang/dn-access-control-list",
				AccessListsDnAccessControlList: AccessListsDnAccessControlList{
					Ipv4: &Ipv4{
						AccessList: AccessList{
							ConfigItems: ConfigItems{Name: HALO_ACL_BUCKET_NAME},
							Name:        HALO_ACL_BUCKET_NAME,
							Rules:       Rules{Rule: rules},
						},
					},
				},
			},
		},
		TargetCandidate: TargetCandidate{Candidate: Candidate{}},
	}
}

func (hal *DnHalImpl) Steer(rules []*SteerItem) error {
	ruleIdxMap := make(map[int]string)
	rulesList := make([]Rule, 0, len(rules))
	session := NetConfConnector()

	for _, rule := range rules {
		fk := rule.Rule
		nh := rule.NextHop

		var nextHop1 net.IP
		if nh != "" {
			ifc := findInterfaceByNextHop(nh)
			nextHop1 = ifc.NextHop
			if ifc == nil {
				err := fmt.Errorf("no interface with next-hop: %s", nh)
				log.Warn(err)
				return err
			}
		} else {
			nextHop1 = nil
		}

		ruleAsKey := fk.AsKey()
		currentRuleId := -1
		for k, v := range hal.aclRules {
			if v == ruleAsKey {
				currentRuleId = k
				break
			}
		}
		if currentRuleId == -1 {
			currentRuleId = accessListInitId
			accessListInitId += 10
		}

		ruleIdxMap[currentRuleId] = ruleAsKey

		rulesList = append(rulesList, Rule{
			RuleId: currentRuleId,
			RuleConfigItems: RuleConfigItems{
				Ipv4Matches: Ipv4Matches{
					Ipv4AclMatch: Ipv4AclMatch{
						SourceIpv4:      fmt.Sprintf("%s/32", fk.SrcAddr),
						DestinationIpv4: fmt.Sprintf("%s/32", fk.DstAddr),
					},
				},
				Matches: Matches{
					L4AclMatch: L4AclMatch{
						DestinationPortRange: DestinationPortRange{LowerPort: fk.DstPort},
						SourcePortRange:      SourcePortRange{LowerPort: fk.SrcPort},
					},
				},
				Nexthops: Nexthops{
					Nexthop1: Nexthop1{Addr: nextHop1}},
				Protocol: fmt.Sprintf("%s", fk.Protocol),
				RuleType: "allow",
			},
		})
	}

	xmlStruct := getAclXml(rulesList)
	xmlString, _ := xml.MarshalIndent(xmlStruct, "", "    ")
	_, err := session.Exec(netconf.RawMethod(xmlString))
	if err != nil {
		return err
	}

	err = commitChanges()
	if err != nil {
		return err
	}

	for k, v := range ruleIdxMap {
		hal.aclRules[k] = v
	}

	return nil
}

func removeSteerRule(fk *FlowKey) (int, error) {
	ruleId := -1
	target := fk.AsKey()
	for k, v := range hal.aclRules {
		if v == target {
			ruleId = k
		}
	}
	if ruleId == -1 {
		return ruleId, fmt.Errorf("no such a rule in cache: %v", fk)
	}

	session := NetConfConnector()
	log.Printf("Removing acl: %s:%d -> %s:%d, rule-id: %d",
		fk.SrcAddr, fk.SrcPort, fk.DstAddr, fk.DstPort, ruleId)
	_, err := session.Exec(netconf.RawMethod(fmt.Sprintf(DeleteAclRuleByID, ruleId)))
	if err != nil {
		return ruleId, err
	}
	return ruleId, nil
}

func (hal *DnHalImpl) RemoveSteer(fk *FlowKey) error {
	idx, err := removeSteerRule(fk)
	if err != nil {
		return err
	}

	err = commitChanges()
	if err != nil {
		return err
	}

	delete(hal.aclRules, idx)
	return nil
}

func (hal *DnHalImpl) RemoveSteerBulk(rules []*FlowKey) error {
	var ruleIdxs []int
	for _, rule := range rules {
		idx, err := removeSteerRule(rule)
		if err != nil {
			return err
		}
		ruleIdxs = append(ruleIdxs, idx)
	}

	err := commitChanges()
	if err != nil {
		return err
	}

	for _, idx := range ruleIdxs {
		delete(hal.aclRules, idx)
	}

	return nil
}

//func DeleteAcl(dnIf string) error {
//	session, err := netconf.DialSSH(
//		nc.netconfHost,
//		netconf.SSHConfigPassword(nc.netconfUser, nc.netconfPassword))
//	if err != nil {
//		return err
//	}
//	defer session.Close()
//
//	a := fmt.Sprintf(XMLAclDetach, dnIf)
//	_, err = session.Exec(netconf.RawMethod(InterfaceConfig))
//	if err != nil {
//		return err
//	}
//
//	_, err = session.Exec(netconf.RawMethod(DeleteAccessListByName))
//	if err != nil {
//		return err
//	}
//
//	_, err = session.Exec(netconf.RawMethod(Commit))
//	if err != nil {
//		return err
//	}
//	return nil
//}

func SteeringAclCleanup() error {
	session := NetConfConnector()
	defaultRuleId := 65434

	log.Info("Removing all Steering rules")
	_, err := session.Exec(netconf.RawMethod(DeleteAllAclRules))
	if err != nil {
		return err
	}

	log.Info("Creating default ACL rule")
	rulesList := make([]Rule, 0, 1)
	rulesList = append(rulesList, Rule{
		RuleId: defaultRuleId, RuleConfigItems: RuleConfigItems{RuleType: "allow"}})
	xmlStruct := getAclXml(rulesList)
	xmlString, _ := xml.MarshalIndent(xmlStruct, "", "    ")
	_, err = session.Exec(netconf.RawMethod(xmlString))
	if err != nil {
		return err
	}

	log.Printf("Committing changes")
	_, err = session.Exec(netconf.RawMethod(Commit))
	if err != nil {
		return err
	}

	return nil
}

var netconfSession *netconf.Session

func NetConfConnector() *netconf.Session {
	var err error
	var ok bool
	//defaultLog := stdLog.New(os.Stderr, "netconf ", 1)
	//netconf.SetLog(netconf.NewStdLog(defaultLog, netconf.LogDebug))

	if nc.netconfUser, ok = os.LookupEnv("NETCONF_USER"); !ok {
		nc.netconfUser = "dnroot"
	}
	if nc.netconfPassword, ok = os.LookupEnv("NETCONF_PASSWORD"); !ok {
		nc.netconfPassword = "dnroot"
	}
	if nc.netconfHost, ok = os.LookupEnv("DNOS_ADDR"); !ok {
		nc.netconfHost = DRIVENETS_DNOS_ADDR
	}

	if netconfSession == nil {
		sshConfig := &ssh.ClientConfig{
			User:            nc.netconfUser,
			Auth:            []ssh.AuthMethod{ssh.Password(nc.netconfPassword)},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		}
		netconfSession, err = netconf.DialSSH(nc.netconfHost, sshConfig)
		if err != nil {
			log.Fatal(err)
		}
	}
	//defer netconfSession.Close()
	return netconfSession
}

func (h *DnHalImpl) AclRuleCacheAdd(fk *FlowKey, idx int) {
	h.aclRules[idx] = fk.AsKey()
}
