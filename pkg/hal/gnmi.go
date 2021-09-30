package hal

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/openconfig/gnmi/connection"
	"github.com/openconfig/gnmi/manager"
	"github.com/openconfig/gnmi/proto/gnmi"
	"github.com/openconfig/gnmi/proto/target"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
)

type InterfaceUpdate struct {
	Name  string
	Stats InterfaceTelemetry
}

type InterfaceMonitorOps func(*InterfaceMonitor)

type InterfaceMonitor struct {
	UpdateInterval time.Duration
	Target         target.Target
	ConMgr         *connection.Manager
	Manager        *manager.Manager
	SpeedChan      chan InterfaceUpdate
	StatsChan      chan InterfaceUpdate
}

type DnCredentials struct {
	user     string
	password string
}

func NewDnCredentials() DnCredentials {
	dnc := DnCredentials{
		user:     hal.config.Netconf.User,
		password: hal.config.Netconf.Password,
	}
	return dnc
}

func (dn DnCredentials) Lookup(ctx context.Context, user string) (string, error) {
	if user == dn.user {
		return dn.password, nil
	}
	return "", fmt.Errorf("unknown user %s", user)
}

const NETCONF_DIAL_TIMEOUT = 10

func NewInterfaceMonitor(addr string, upd time.Duration) (*InterfaceMonitor, error) {
	var err error

	cred := NewDnCredentials()

	im := &InterfaceMonitor{
		UpdateInterval: upd,
		Target: target.Target{
			Addresses: []string{addr},
			Credentials: &target.Credentials{
				Username: cred.user,
				Password: cred.password,
			},
		},
		SpeedChan: make(chan InterfaceUpdate, 1),
		StatsChan: make(chan InterfaceUpdate, 1),
	}

	im.ConMgr, err = connection.NewManager(
		grpc.WithConnectParams(
			grpc.ConnectParams{
				Backoff: backoff.Config{
					MaxDelay: 5 * time.Second},
				MinConnectTimeout: 1 * time.Second,
			}),
		grpc.WithInsecure(),
		grpc.WithBlock())
	if err != nil {
		log.Errorf("Failed to create gNMI connection manager")
	}

	im.Manager, err = manager.NewManager(
		manager.Config{
			Credentials:       cred,
			ConnectionManager: im.ConMgr,
			Timeout:           NETCONF_DIAL_TIMEOUT * time.Second,
			ConnectError: func(name string, err error) {
				log.Warnf("Failed to connect gNMI: interface=%s, reason=%v", name, err)
			},
			Update: func(g *gnmi.Notification) {
				im.Update(g)
			},
		},
	)
	if err != nil {
		log.Errorf("Failed to create gNMI manager")
		return nil, err
	}

	return im, nil
}

func (im *InterfaceMonitor) Update(n *gnmi.Notification) {
	var err error

	for _, update := range n.GetUpdate() {
		var ifname string
		for _, elm := range update.Path.GetElem() {
			if elm.Name == "interface" {
				ifname = elm.Key["name"]
				break
			}
		}
		if ifname == "" {
			continue
		}

		lastPathElement := update.Path.GetElem()[len(update.Path.GetElem())-1].Name
		if lastPathElement == "interface-speed" {
			speed, err := strconv.ParseUint(string(update.Val.GetJsonVal()), 10, 64)
			if err != nil {
				log.Fatal("Failed to parse interface speed update: %s. Reason: %s", update, err)
			}
			im.SpeedChan <- InterfaceUpdate{
				Name: ifname,
				Stats: InterfaceTelemetry{
					Speed: speed},
			}
		} else {
			stats := InterfaceTelemetry{}
			err = json.Unmarshal(update.Val.GetJsonVal(), &stats)
			if err != nil {
				log.Fatal("Failed to unpack interface stats update: %s. Reason: %s", update, err)
			}
			im.StatsChan <- InterfaceUpdate{
				Name:  ifname,
				Stats: stats}
		}
	}
}

func (im *InterfaceMonitor) Add(name string) {
	speed, counters := interfaceOperPaths(name)
	im.Manager.Add(
		name, &im.Target,
		&gnmi.SubscribeRequest{
			Request: &gnmi.SubscribeRequest_Subscribe{
				Subscribe: &gnmi.SubscriptionList{
					Subscription: []*gnmi.Subscription{{
						Path:           speed,
						SampleInterval: uint64(im.UpdateInterval),
					}, {
						Path:           counters,
						SampleInterval: uint64(im.UpdateInterval),
					}},
					Mode:     gnmi.SubscriptionList_STREAM,
					Encoding: gnmi.Encoding_JSON,
				},
			}})
}

func (im *InterfaceMonitor) Remove(name string) error {
	return im.Manager.Remove(name)
}

func (im *InterfaceMonitor) Speed() chan InterfaceUpdate {
	return im.SpeedChan
}

func (im *InterfaceMonitor) Stats() chan InterfaceUpdate {
	return im.StatsChan
}
