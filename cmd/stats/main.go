package main

import (
	"fmt"
	"io"
	"time"

	log "github.com/sirupsen/logrus"

	pb "github.com/drivenets/vmw_tsf/pkg/hal/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
)

type IfcStats struct {
	name   string
	rx_bps []float64
	tx_bps []float64
}

const STATS_LEN = 48

func NewIfcStats(name string, len int) IfcStats {
	return IfcStats{
		name:   name,
		rx_bps: make([]float64, len),
		tx_bps: make([]float64, len),
	}
}

type HostStats struct {
	name   string
	url    string
	conn   *grpc.ClientConn
	client pb.StatsClient
	ifc    map[string]IfcStats
	acl    []float64
}

func NewHostStats(name string, url string) HostStats {
	h := HostStats{
		name: name,
		url:  url,
		ifc:  make(map[string]IfcStats, 3),
		acl:  make([]float64, STATS_LEN),
	}
	h.ifc["local"] = NewIfcStats("local", STATS_LEN)
	h.ifc["halo0"] = NewIfcStats("halo0", STATS_LEN)
	h.ifc["halo1"] = NewIfcStats("halo1", STATS_LEN)
	conn, err := grpc.Dial(h.url, grpc.WithInsecure())
	if err != nil {
		log.Errorf("Failed to connect to %s. Reason: %v", url, err)
	}
	h.client = pb.NewStatsClient(conn)
	return h
}

func shift(s []float64, e float64) {
	n := len(s)
	for i := 0; i < n-1; i++ {
		s[i] = s[i+1]
	}
	s[n-1] = e
}

func (h HostStats) Update(ctx context.Context) {
	for {
		tmo, _ := context.WithTimeout(ctx, time.Second)
		stream, err := h.client.GetInterfaces(tmo, &pb.Empty{})
		if err != nil {
			//log.Warnf("Failed to get interfaces. Reason: %v", err)
		} else {
			for {
				ifc, err := stream.Recv()
				if err != nil {
					if err == io.EOF {
						break
					}
					//log.Fatalf("Failed to get interface entry. Reason: %v", err)
				}
				if stats, ok := h.ifc[ifc.GetName()]; !ok {
					log.Warnf("Got unknown interface %s", ifc.GetName)
					continue
				} else {
					shift(stats.rx_bps, float64(ifc.RxBps))
					shift(stats.tx_bps, float64(ifc.TxBps))
				}
			}
		}

		tmp, _ := context.WithTimeout(ctx, time.Second)
		acl, err := h.client.GetAclCacheSize(tmp, &pb.Empty{})
		if err != nil {
			//log.Warnf("Failed to get acl cache size. Reason: %v", err)
		} else {
			shift(h.acl, float64(acl.Size))
		}
		time.Sleep(time.Second)
	}
}

func (h HostStats) Close() {
	h.conn.Close()
}

type ScreenRect struct {
	x1 int
	y1 int
	x2 int
	y2 int
}

func (h HostStats) Widget(r ScreenRect) ui.Drawable {

	lrx := widgets.NewSparkline()
	lrx.Title = "local_rx"
	lrx.Data = h.ifc["local"].rx_bps
	lrx.MaxVal = 200 * 1e6
	lrx.MaxHeight = 4

	ltx := widgets.NewSparkline()
	ltx.Title = "local_tx"
	ltx.Data = h.ifc["local"].tx_bps
	ltx.MaxVal = 200 * 1e6
	ltx.MaxHeight = 4

	h0rx := widgets.NewSparkline()
	h0rx.Title = "halo0_rx"
	h0rx.Data = h.ifc["halo0"].rx_bps
	h0rx.MaxVal = 200 * 1e6
	h0rx.MaxHeight = 4

	h0tx := widgets.NewSparkline()
	h0tx.Title = "halo0_tx"
	h0tx.Data = h.ifc["halo0"].tx_bps
	h0tx.MaxVal = 200 * 1e6
	h0tx.MaxHeight = 4

	h1rx := widgets.NewSparkline()
	h1rx.Title = "halo1_rx"
	h1rx.Data = h.ifc["halo1"].rx_bps
	h1rx.MaxVal = 200 * 1e6
	h1rx.MaxHeight = 4

	h1tx := widgets.NewSparkline()
	h1tx.Title = "halo1_tx"
	h1tx.Data = h.ifc["halo1"].tx_bps
	h1tx.MaxVal = 200 * 1e6
	h1tx.MaxHeight = 4

	acl := widgets.NewSparkline()
	acl.Title = "acl_size"
	acl.Data = h.acl
	acl.MaxVal = 100
	acl.MaxHeight = 4

	w := widgets.NewSparklineGroup(lrx, ltx, h0rx, h0tx, h1rx, h1tx, acl)
	w.Title = fmt.Sprintf("Node: %s", h.name)
	w.SetRect(r.x1, r.y1, r.x2, r.y2)

	return w
}

const STATS_PORT = 7732

func HostStatsUrl(host string) string {
	return fmt.Sprintf("%s:%d", host, STATS_PORT)
}

func cleanup(h []HostStats) {
	for _, s := range h {
		s.Close()
	}
}

func main() {

	h := []HostStats{
		NewHostStats("halo-a", HostStatsUrl("200.200.201.11")),
		NewHostStats("halo-b", HostStatsUrl("200.200.200.12")),
		NewHostStats("halo-c", HostStatsUrl("200.200.201.12")),
	}
	defer cleanup(h)

	if err := ui.Init(); err != nil {
		log.Fatalf("Failed to initialize termui. Reason: %v", err)
	}
	defer ui.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, s := range h {
		go s.Update(ctx)
	}

	w := []ui.Drawable{
		h[0].Widget(ScreenRect{0, 0, 50, 40}),
		h[1].Widget(ScreenRect{52, 0, 102, 40}),
		h[2].Widget(ScreenRect{104, 0, 154, 40}),
	}

	ui.Render(w...)
	uiEvents := ui.PollEvents()
	tick := time.NewTicker(time.Duration(1) * time.Second)
	for {
		select {
		case <-tick.C:
			ui.Render(w...)
		case e := <-uiEvents:
			switch e.ID {
			case "q", "<C-c>":
				tick.Stop()
				return
			}
		}
	}
}
