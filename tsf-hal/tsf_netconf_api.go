//package tsf_netconf
package hal

var DeleteAclRuleByID = `
<edit-config>
    <target>
      <candidate/>
    </target>
    <config>
        <drivenets-top xmlns="http://drivenets.com/ns/yang/dn-top">
            <access-lists xmlns="http://drivenets.com/ns/yang/dn-access-control-list">
                <ipv4>
                    <access-list>
                        <name>Steering</name>
                        <rules>
                            <rule xmlns:nc="urn:ietf:params:xml:ns:netconf:base:1.0" nc:operation="delete">
                                <rule-id>%d</rule-id>
                            </rule>
                        </rules>
                    </access-list>
                </ipv4>
            </access-lists>
        </drivenets-top>
    </config>
</edit-config>
`

var DeleteAllAclRules = `
<edit-config>
    <target>
        <candidate/>
    </target>
    <config>
        <drivenets-top xmlns="http://drivenets.com/ns/yang/dn-top">
            <access-lists xmlns="http://drivenets.com/ns/yang/dn-access-control-list">
                <ipv4>
                    <access-list>
                        <name>Steering</name>
                        <rules xmlns:nc="urn:ietf:params:xml:ns:netconf:base:1.0" nc:operation="delete">
                        </rules>
                    </access-list>
                </ipv4>
            </access-lists>
        </drivenets-top>
    </config>
</edit-config>
`

//var XMLAclDelete = `
//<edit-config>
//    <target>
//        <candidate/>
//    </target>
//    <config>
//        <drivenets-top xmlns="http://drivenets.com/ns/yang/dn-top">
//            <access-lists xmlns="http://drivenets.com/ns/yang/dn-access-control-list">
//                <ipv4>
//                    <access-list xmlns:nc="urn:ietf:params:xml:ns:netconf:base:1.0" nc:operation="delete">
//                        <name>%s</name>
//                    </access-list>
//                </ipv4>
//            </access-lists>
//        </drivenets-top>
//    </config>
//</edit-config>
//`

//var XMLAclDetach = `
//<edit-config>
//    <target>
//        <candidate />
//    </target>
//    <config>
//        <drivenets-top xmlns="http://drivenets.com/ns/yang/dn-top" xmlns:dn-if="http://drivenets.com/ns/yang/dn-interfaces">
//           <dn-if:interfaces>
//               <dn-if:interface>
//                    <dn-if:name>%s</dn-if:name>
//                    <acl-attached>
//                        <interface-ipv4-access-lists xmlns:nc="urn:ietf:params:xml:ns:netconf:base:1.0" nc:operation="delete">
//                        </interface-ipv4-access-lists>
//                    </acl-attached>
//               </dn-if:interface>
//           </dn-if:interfaces>
//        </drivenets-top>
//    </config>
//</edit-config>
//`

var CreateParameterizedAclRule = `
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
                                            <source-ipv4>%[3]s/32</source-ipv4>
                                            <destination-ipv4>%[5]s/32</destination-ipv4>
                                        </ipv4-acl-match>
                                    </ipv4-matches>
                                    <matches>
                                        <l4-acl-match>
                                            <source-port-range><lower-port>%[4]d</lower-port></source-port-range>
                                            <destination-port-range><lower-port>%[6]d</lower-port></destination-port-range>
                                        </l4-acl-match>
                                    </matches>
                                    <rule-type>allow</rule-type>
                                    <nexthops><nexthop1>%[7]s</nexthop1></nexthops>
                                    <protocol>%[2]s</protocol>
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

var CreateDefaultAclRule = `
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
                                <rule-id>65434</rule-id>
                                <config-items>
                                   <rule-type>allow</rule-type>
                                  <ipv4-matches>
                                       <ipv4-acl-match>
                                           <source-ipv4></source-ipv4>
                                           <destination-ipv4></destination-ipv4>
                                       </ipv4-acl-match>
                                   </ipv4-matches>
                                   <matches>
                                       <l4-acl-match>
                                           <source-port-range></source-port-range>
                                           <destination-port-range></destination-port-range>
                                       </l4-acl-match>
                                   </matches>
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

//var ShowSystemRequest = `
//<show-system xmlns="http://drivenets.com/ns/yang/dn-rpc">
//  <result />
//</show-system>
//`

var Commit = "<commit />"

type ServerConf struct {
	netconfUser     string
	netconfPassword string
	netconfHost     string
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
//	createAcl := fmt.Sprintf(CreateParameterizedAclRule,
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
