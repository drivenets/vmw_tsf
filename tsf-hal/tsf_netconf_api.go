//package tsf_netconf
package hal

//var GetAccessListByName = `
//<get-config>
//    <source>
//        <running />
//    </source>
//    <filter>
//        <drivenets-top xmlns:dn-access-control-list="http://drivenets.com/ns/yang/dn-access-control-list">
//            <dn-access-control-list:access-lists>
//                <ipv4>
//                    <access-list>
//                        <name>%s</name>
//                    </access-list>
//                </ipv4>
//            </dn-access-control-list:access-lists>
//        </drivenets-top>
//    </filter>
//</get-config>
//`
//
//var GetInterfaceByName = `
//<get-config>
//    <source>
//        <running />
//    </source>
//    <filter>
//        <drivenets-top xmlns="http://drivenets.com/ns/yang/dn-top" xmlns:dn-if="http://drivenets.com/ns/yang/dn-interfaces">
//            <dn-if:interfaces>
//                <dn-if:interface dn-if:name="%s" />
//            </dn-if:interfaces>
//        </drivenets-top>
//    </filter>
//</get-config>
//`

var AccessListConfig = `
<edit-config>
    <target>
        <candidate />
    </target>
    <config>
        <drivenets-top xmlns:dn-access-control-list="http://drivenets.com/ns/yang/dn-access-control-list">
            <dn-access-control-list:access-lists>
                <ipv4>
                    <access-list>
                        <rules>
                            <rule>
                                <rule-id>%[1]d</rule-id>
                                <config-items>
                                    <ipv4-matches>
                                        <ipv4-acl-match>
                                            <destination-ipv4>%[2]s</destination-ipv4>
                                            <source-ipv4>%[3]s</source-ipv4>
                                        </ipv4-acl-match>
                                    </ipv4-matches>
                                    <matches>
                                        <l4-acl-match>
                                            <source-port-range></source-port-range>
                                            <destination-port-range></destination-port-range>
                                        </l4-acl-match>
                                    </matches>
                                    <rule-type>allow</rule-type>
                                    <protocol>%[4]s</protocol>
                                </config-items>
                            </rule>
                        </rules>
                        <config-items>
                            <name>Steering</name>
                        </config-items>
                        <name>Steering</name>
                    </access-list>
                </ipv4>
            </dn-access-control-list:access-lists>
        </drivenets-top>
    </config>
</edit-config>
`

var InterfaceConfig = `
<edit-config>
    <target>
        <candidate />
    </target>
    <config>
        <drivenets-top xmlns="http://drivenets.com/ns/yang/dn-top" xmlns:dn-if="http://drivenets.com/ns/yang/dn-interfaces">
           <dn-if:interfaces>
               <dn-if:interface>
                    <dn-if:name>%s</dn-if:name>
                    <config-items>
                        <description>halo again</description>
                    </config-items>
                    <acl-attached><interface-ipv4-access-lists><interface-ipv4-access-list><global-acl>false</global-acl><config-items><global-acl>false</global-acl><direction>in</direction><name>Steering</name></config-items><direction>in</direction></interface-ipv4-access-list></interface-ipv4-access-lists></acl-attached>
               </dn-if:interface>
           </dn-if:interfaces>
        </drivenets-top>
    </config>
</edit-config>
`

//var ShowSystemRequest = `
//<show-system xmlns="http://drivenets.com/ns/yang/dn-rpc">
//  <result />
//</show-system>
//`

var Commit = "<commit />"

type ServerConf struct {
	netconfUser     string
	netconfPassword string
	netconfHost   string
}

var nc = &ServerConf{}

var accessListInitId = 10

//func main() {
//	haloInterface0 := "ge100-0/0/39"
//	fk := FlowKey{
//		Protocol: "any",
//		SrcAddr:  []byte("10.0.0.1/32"),
//		DstAddr:  []byte("200.200.200.1/32"),
//		SrcPort:  0,
//		DstPort:  0,
//	}
//
//	err := Steer(fk, haloInterface0)
//	if err != nil {
//		panic(err)
//	}
//}

//func (hal *DnHalImpl) Steer2(fk FlowKey, ifName string) error {
//	var ok bool
//	if nc.netconfUser, ok = os.LookupEnv("NETCONF_USER"); !ok {
//		nc.netconfUser = "dnroot"
//	}
//	if nc.netconfPassword, ok = os.LookupEnv("NETCONF_PASSWORD"); !ok {
//		nc.netconfPassword = "dnroot"
//	}
//	if nc.netconfHost, ok = os.LookupEnv("NETCONF_HOST"); !ok {
//		nc.netconfHost = "localhost"
//	}
//	log := log.New(os.Stderr, "netconf ", 1)
//	netconf.SetLog(netconf.NewStdLog(log, netconf.LogDebug))
//	session, err := netconf.DialSSH(
//		nc.netconfHost,
//		netconf.SSHConfigPassword(nc.netconfUser, nc.netconfPassword))
//	if err != nil {
//		return err
//	}
//	defer session.Close()
//
//	log.Println(session.SessionID)
//	createAcl := fmt.Sprintf(AccessListConfig,
//		accessListInitId,
//		string(fk.SrcAddr),
//		string(fk.DstAddr),
//		"any")
//	_, err = session.Exec(netconf.RawMethod(createAcl))
//	if err != nil {
//		return err
//	}
//
//	attachAclToIface := fmt.Sprintf(InterfaceConfig, ifName)
//	_, err = session.Exec(netconf.RawMethod(attachAclToIface))
//	if err != nil {
//		return err
//	}
//
//	_, err = session.Exec(netconf.RawMethod(Commit))
//	if err != nil {
//		return err
//	}
//
//	accessListInitId += 10
//	return nil
//}
