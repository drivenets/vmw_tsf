package hal

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	pb "github.com/openconfig/gnmi/proto/gnmi"
	"github.com/openconfig/ygot/ygot"
	"google.golang.org/grpc"
)

type DnHalImpl struct {
	mutex       sync.Mutex
	initialized bool
	grpcAddr    string
	interfaces  struct {
		l2u   map[string]string
		u2l   map[string]string
		stats map[string]InterfaceTelemetry
	}
}

var hal = &DnHalImpl{}

func NewDnHal() DnHal {
	if !hal.initialized {
		hal.Init()
	}
	return hal
}

func (hal *DnHalImpl) Init() {
	hal.mutex.Lock()
	defer hal.mutex.Unlock()

	hal.grpcAddr = "localhost:50051"
	if grpcAddrEnv := os.Getenv("GRPC_ADDR"); grpcAddrEnv != "" {
		hal.grpcAddr = grpcAddrEnv
	}

	dnosInterface := "ge100-0/0/1"
	if dnosInterfaceEnv := os.Getenv("DNOS_IFACE"); dnosInterfaceEnv != "" {
		dnosInterface = dnosInterfaceEnv
	}

	halInterface := "halo1"
	if halInterfaceName := os.Getenv("HAL_IFACE"); halInterfaceName != "" {
		halInterface = halInterfaceName
	}

	hal.interfaces.l2u = make(map[string]string)
	hal.interfaces.l2u[dnosInterface] = halInterface

	hal.interfaces.u2l = make(map[string]string)
	hal.interfaces.u2l[halInterface] = dnosInterface

	hal.interfaces.stats = make(map[string]InterfaceTelemetry)
	hal.interfaces.stats[halInterface] = InterfaceTelemetry{}

	go monitorInterfaces()

	hal.initialized = true
}

func monitorInterfaces() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(30*time.Minute))
	defer cancel()

	conn, err := grpc.Dial(hal.grpcAddr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	c := pb.NewGNMIClient(conn)
	client, err := c.Subscribe(ctx)
	if err != nil {
		log.Fatalf("could not connect: %v", err)
	}

	for _, value := range hal.interfaces.u2l {
		path := fmt.Sprintf("/drivenets-top/interfaces/interface[name='%s']/oper-items/counters/ethernet-counters", value)
		pathStr := strings.Replace(path, "'", "", -1)
		gnmiPath, _ := ygot.StringToPath(pathStr, ygot.StructuredPath, ygot.StringSlicePath)
		log.Println("Subscription path: ", path)
		log.Println("Subscription gnmi path ", gnmiPath)

		client.Send(&pb.SubscribeRequest{
			Request: &pb.SubscribeRequest_Subscribe{
				Subscribe: &pb.SubscriptionList{
					Subscription: []*pb.Subscription{
						{
							Path:           gnmiPath,
							SampleInterval: uint64(time.Duration(15 * time.Second)),
						},
					},
					Mode:     pb.SubscriptionList_STREAM,
					Encoding: pb.Encoding_JSON,
				},
			},
		})
	}

	for {
		response, err := client.Recv()
		if err != nil {
			log.Fatalf("Failed to get response: %v", err)
		}

		for _, update := range response.GetUpdate().Update {
			// TODO: update interfaces structure
			var tmpTelemetry InterfaceTelemetry
			_ = json.Unmarshal(update.Val.GetJsonVal(), &tmpTelemetry)
			hal.interfaces.stats["halo1"] = tmpTelemetry  //InterfaceTelemetry{Speed: 1234}
			//log.Println(proto.MarshalTextString(update.Val))
		}
	}
}

func (*DnHalImpl) Steer(fk *FlowKey, nh string) error {
	log.Fatal("NOT IMPLEMENTED")
	return nil
}

func (hal *DnHalImpl) GetInterfaces(v InterfaceVisitor) error {
	for ifc, tl := range hal.interfaces.stats {
		err := v(ifc, &tl)
		if err != nil {
			return err
		}
	}
	return nil
}

func (*DnHalImpl) GetFlows(v FlowVisitor) error {
	log.Fatal("NOT IMPLEMENTED")
	return nil
}
