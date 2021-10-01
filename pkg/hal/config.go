package hal

import (
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type HalCfg struct {
	Netconf             NetconfCfg
	DnosAddr            string
	LocalIfname         string
	IfcSamplingInterval float64
	FlowThreshold       uint64
	SkipTwamp           bool
	FlushSteer          bool
	StatsServer         bool
	Debug               bool
	Lan                 InterfaceCfg
	Wan                 []InterfaceCfg
	Topo                TopoCfg
}

type NetconfCfg struct {
	User     string
	Password string
}

type TwampCfg struct {
	Host string
	Port uint16
}

type InterfaceCfg struct {
	Upper    string
	Lower    string
	HwAddr   string
	Ipv4Addr string
}

type TopoCfg struct {
	NextHop map[string]string
	Twamp   map[string]TwampCfg
	Peer    map[string]string
}

const DEFAULT_FLOW_THRESHOLD = 100000

func (hal *DnHalImpl) WorkaroundViperDictDotKeyIssue() {
	nhFixed := make(map[string]string)
	for key, value := range hal.config.Topo.NextHop {
		nhFixed[strings.ReplaceAll(key, "-", ".")] = value
	}
	hal.config.Topo.NextHop = nhFixed
	twampFixed := make(map[string]TwampCfg)
	for key, value := range hal.config.Topo.Twamp {
		twampFixed[strings.ReplaceAll(key, "-", ".")] = value
	}
	hal.config.Topo.Twamp = twampFixed
	peerFixed := make(map[string]string)
	for key, value := range hal.config.Topo.Peer {
		peerFixed[strings.ReplaceAll(key, "-", ".")] = value
	}
	hal.config.Topo.Peer = peerFixed
}

func (hal *DnHalImpl) LoadConfig() {
	viper.KeyDelimiter("::")
	viper.SetConfigName("halo.cfg")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("/etc/")
	viper.SetDefault("IfcSamplingInterval", 1)
	viper.SetDefault("FlowThreshold", DEFAULT_FLOW_THRESHOLD)
	viper.SetDefault("SkipTwamp", true)
	viper.SetDefault("Debug", false)
	viper.SetDefault("FlushSteer", true)
	viper.SetDefault("StatsServer", true)
	viper.SetDefault("Netconf.User", "dnroot")
	viper.SetDefault("Netconf.Password", "dnroot")
	err := viper.ReadInConfig()
	if err != nil {
		if configRequired {
			panic(err)
		} else {
			log.Warn("Failed to load config file:", viper.ConfigFileUsed())
		}
	}

	/* Override GRPC address with DNOS_ADDR environment variable
	 */
	var ok bool
	if hal.config.DnosAddr, ok = os.LookupEnv("DNOS_ADDR"); ok {
		log.Info("Load DNOS_ADDR from environment:", hal.config.DnosAddr)
	}

	viper.Unmarshal(&hal.config)
	hal.WorkaroundViperDictDotKeyIssue()
}

func (hal *DnHalImpl) LogConfig() {
	log.Info("HAL-API configuration")
	log.Info("======================")
	log.Info("netconf user: ", hal.config.Netconf.User)
	log.Info("netconf password: ", hal.config.Netconf.Password)
	log.Info("dnos addr: ", hal.config.DnosAddr)
	log.Info("local interface: ", hal.config.LocalIfname)
	log.Info("sampling interval: ", hal.config.IfcSamplingInterval)
	log.Info("flow threshold: ", hal.config.FlowThreshold)
	log.Info("skip twamp: ", hal.config.SkipTwamp)
	log.Info("debug: ", hal.config.Debug)
	log.Info("lan: ")
	log.Info("  upper:", hal.config.Lan.Upper)
	log.Info("  lower:", hal.config.Lan.Lower)
	log.Info("  hwAddr:", hal.config.Lan.HwAddr)
	log.Info("  ipv4Addr:", hal.config.Lan.Ipv4Addr)
	log.Info("wan: ")
	for _, ifc := range hal.config.Wan {
		log.Info("  interface:")
		log.Info("    upper:", ifc.Upper)
		log.Info("    lower:", ifc.Lower)
		log.Info("    hwAddr:", ifc.HwAddr)
		log.Info("    ipv4Addr:", ifc.Ipv4Addr)
	}
	log.Info("topo: ")
	log.Info("  nextHop:")
	for key, value := range hal.config.Topo.NextHop {
		log.Info("    ", key, ": ", value)
	}
	log.Info("  twamp:")
	for key, value := range hal.config.Topo.Twamp {
		log.Info("    ", key, ":")
		log.Info("      host: ", value.Host)
		log.Info("      port: ", value.Port)
	}
	log.Info("  peer:")
	for key, value := range hal.config.Topo.Peer {
		log.Info("    ", key, ": ", value)
	}
	log.Info()
}
