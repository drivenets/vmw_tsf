package hal

import (
	"context"
	"fmt"
	"log"
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
	grpc_addr   string
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

	hal.grpc_addr = "localhost:50051"
	hal.interfaces.l2u["ge100-0/0/1"] = "halo1"
	hal.interfaces.u2l["halo1"] = "ge100-0/0/1"
	hal.interfaces.stats["halo1"] = InterfaceTelemetry{}

	go monitorInterfaces()

	hal.initialized = true
}

func monitorInterfaces() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(30*time.Minute))
	defer cancel()

	conn, err := grpc.Dial(hal.grpc_addr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	c := pb.NewGNMIClient(conn)
	client, err := c.Subscribe(ctx)
	if err != nil {
		log.Fatalf("could not connect: %v", err)
	}

	for key, _ := range hal.interfaces.stats {
		path := fmt.Sprintf("/drivenets-top/interfaces/interface[name='%s']/oper-items/counters/ethernet-counters", key)
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

		log.Println("Received response:")
		for _, update := range response.GetUpdate().Update {
			log.Println(update.Val)
			//log.Println(proto.MarshalTextString(update.Val))

			// TODO: update interfaces structure
		}
	}
}

func (*DnHalImpl) Steer(fk *FlowKey, nh string) error {
	log.Fatal("NOT IMPLEMENTED")
	return nil
}

func (*DnHalImpl) GetInterfaces(v InterfaceVisitor) error {
	log.Fatal("NOT IMPLEMENTED")
	return nil
}

func (*DnHalImpl) GetFlows(v FlowVisitor) error {
	log.Fatal("NOT IMPLEMENTED")
	return nil
}
