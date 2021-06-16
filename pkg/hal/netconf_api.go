//package tsf_netconf
package hal

//var DeleteAclRuleByID2 = `
//<edit-config>
//    <config>
//        <drivenets-top xmlns:dn-access-control-list="http://drivenets.com/ns/yang/dn-access-control-list">
//            <dn-access-control-list:access-lists>
//                <ipv4>
//                    <access-list>
//                        <config-items>
//                            <name>Steering</name>
//                        </config-items>
//                        <name>Steering</name>
//                        <rules>
//                            <rule xmlns:_="urn:ietf:params:xml:ns:netconf:base:1.0" _:operation="delete">
//                                <rule-id>10</rule-id>
//                            </rule>
//                        </rules>
//                    </access-list>
//                </ipv4>
//            </dn-access-control-list:access-lists>
//        </drivenets-top>
//    </config>
//    <target>
//        <candidate></candidate>
//    </target>
//</edit-config>
//`
//
//var DeleteAclRuleByID = `
//<edit-config>
//    <target>
//      <candidate/>
//    </target>
//    <config>
//        <drivenets-top xmlns="http://drivenets.com/ns/yang/dn-top">
//            <access-lists xmlns="http://drivenets.com/ns/yang/dn-access-control-list">
//                <ipv4>
//                    <access-list>
//                        <name>Steering</name>
//                        <rules>
//                            <rule xmlns:_="urn:ietf:params:xml:ns:netconf:base:1.0" _:operation="delete">
//                                <rule-id>%d</rule-id>
//                            </rule>
//                        </rules>
//                    </access-list>
//                </ipv4>
//            </access-lists>
//        </drivenets-top>
//    </config>
//</edit-config>
//`
//
//var DeleteAllAclRules = `
//<edit-config>
//    <target>
//        <candidate/>
//    </target>
//    <config>
//        <drivenets-top xmlns="http://drivenets.com/ns/yang/dn-top">
//            <access-lists xmlns="http://drivenets.com/ns/yang/dn-access-control-list">
//                <ipv4>
//                    <access-list>
//                        <name>Steering</name>
//                        <rules xmlns:nc="urn:ietf:params:xml:ns:netconf:base:1.0" nc:operation="delete">
//                        </rules>
//                    </access-list>
//                </ipv4>
//            </access-lists>
//        </drivenets-top>
//    </config>
//</edit-config>
//`

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

//var CreateParameterizedAclRule = `
//<edit-config>
//    <target>
//        <candidate />
//    </target>
//    <config>
//        <drivenets-top xmlns:dn-access-control-list="http://drivenets.com/ns/yang/dn-access-control-list">
//            <dn-access-control-list:access-lists>
//                <ipv4>
//                    <access-list>
//                        <rules>
//                            <rule>
//                                <rule-id>%[1]d</rule-id>
//                                <config-items>
//                                    <ipv4-matches>
//                                        <ipv4-acl-match>
//                                            <source-ipv4>%[3]s/32</source-ipv4>
//                                            <destination-ipv4>%[5]s/32</destination-ipv4>
//                                        </ipv4-acl-match>
//                                    </ipv4-matches>
//                                    <matches>
//                                        <l4-acl-match>
//                                            <source-port-range><lower-port>%[4]d</lower-port></source-port-range>
//                                            <destination-port-range><lower-port>%[6]d</lower-port></destination-port-range>
//                                        </l4-acl-match>
//                                    </matches>
//                                    <rule-type>allow</rule-type>
//                                    <nexthops><nexthop1>%[7]s</nexthop1></nexthops>
//                                    <protocol>%[2]s</protocol>
//                                </config-items>
//                            </rule>
//                        </rules>
//                        <config-items>
//                            <name>Steering</name>
//                        </config-items>
//                        <name>Steering</name>
//                    </access-list>
//                </ipv4>
//            </dn-access-control-list:access-lists>
//        </drivenets-top>
//    </config>
//</edit-config>
//`
//
//var CreateDefaultAclRule = `
//<edit-config>
//    <target>
//        <candidate />
//    </target>
//    <config>
//        <drivenets-top xmlns:dn-access-control-list="http://drivenets.com/ns/yang/dn-access-control-list">
//            <dn-access-control-list:access-lists>
//                <ipv4>
//                    <access-list>
//                        <rules>
//                            <rule>
//                                <rule-id>65434</rule-id>
//                                <config-items>
//                                   <rule-type>allow</rule-type>
//                                  <ipv4-matches>
//                                       <ipv4-acl-match>
//                                           <source-ipv4></source-ipv4>
//                                           <destination-ipv4></destination-ipv4>
//                                       </ipv4-acl-match>
//                                   </ipv4-matches>
//                                   <matches>
//                                       <l4-acl-match>
//                                           <source-port-range></source-port-range>
//                                           <destination-port-range></destination-port-range>
//                                       </l4-acl-match>
//                                   </matches>
//                                </config-items>
//                            </rule>
//                        </rules>
//                        <config-items>
//                            <name>Steering</name>
//                        </config-items>
//                        <name>Steering</name>
//                    </access-list>
//                </ipv4>
//            </dn-access-control-list:access-lists>
//        </drivenets-top>
//    </config>
//</edit-config>
//`

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

var accessListID = 10
