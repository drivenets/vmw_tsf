package hal

import (
	"encoding/xml"
	"net"
)

// Reply
type Data struct {
	XMLName                         xml.Name           `xml:"data,omitempty" json:"data,omitempty"`
	AttrXmlnsdn_access_control_list string             `xml:"xmlns dn-access-control-list,attr"  json:",omitempty"`
	AttrXmlnsdn_top                 string             `xml:"xmlns dn-top,attr"  json:",omitempty"`
	DrivenetsTopReply               *DrivenetsTopReply `xml:"http://drivenets.com/ns/yang/dn-top drivenets-top,omitempty" json:"drivenets-top,omitempty"`
}

type AccessListsDnAccessControlListReply struct {
	XMLName xml.Name `xml:"access-lists,omitempty" json:"access-lists,omitempty"`
	Ipv4    *Ipv4    `xml:"ipv4,omitempty" json:"ipv4,omitempty"`
}

type DrivenetsTopReply struct {
	XMLName                             xml.Name                             `xml:"drivenets-top,omitempty" json:"drivenets-top,omitempty"`
	AccessListsDnAccessControlListReply *AccessListsDnAccessControlListReply `xml:"http://drivenets.com/ns/yang/dn-access-control-list access-lists,omitempty" json:"access-lists,omitempty"`
}

// Get (running) config
type GetConfig struct {
	XMLName xml.Name `xml:"get-config,omitempty" json:"get-config,omitempty"`
	Filter  *Filter  `xml:"filter,omitempty" json:"filter,omitempty"`
	Source  *Source  `xml:"source,omitempty" json:"source,omitempty"`
}

type Source struct {
	XMLName       xml.Name       `xml:"source,omitempty" json:"source,omitempty"`
	RunningConfig *RunningConfig `xml:"running,omitempty" json:"running,omitempty"`
}

type RunningConfig struct {
	XMLName xml.Name `xml:"running,omitempty" json:"running,omitempty"`
}

type Filter struct {
	XMLName      xml.Name      `xml:"filter,omitempty" json:"filter,omitempty"`
	DrivenetsTop *DrivenetsTop `xml:"drivenets-top,omitempty" json:"drivenets-top,omitempty"`
}

// Edit-config
type EditConfig struct {
	XMLName         xml.Name        `xml:"edit-config,omitempty" json:"edit-config,omitempty"`
	Config          Config          `xml:"config,omitempty" json:"config,omitempty"`
	TargetCandidate TargetCandidate `xml:"target,omitempty" json:"target,omitempty"`
}

type TargetCandidate struct {
	XMLName   xml.Name  `xml:"target,omitempty" json:"target,omitempty"`
	Candidate Candidate `xml:"candidate,omitempty" json:"candidate,omitempty"`
}

type Candidate struct {
	XMLName xml.Name `xml:"candidate,omitempty" json:"candidate,omitempty"`
}

type Config struct {
	XMLName      xml.Name     `xml:"config,omitempty" json:"config,omitempty"`
	DrivenetsTop DrivenetsTop `xml:"drivenets-top,omitempty" json:"drivenets-top,omitempty"`
}

type DrivenetsTop struct {
	XMLName                        xml.Name                       `xml:"drivenets-top,omitempty" json:"drivenets-top,omitempty"`
	AttrXmlnsdnAccessControlList   string                         `xml:"xmlns:dn-access-control-list,attr"`
	AccessListsDnAccessControlList AccessListsDnAccessControlList `xml:"http://drivenets.com/ns/yang/dn-access-control-list dn-access-control-list:access-lists,omitempty" json:"access-lists,omitempty"`
}

type AccessListsDnAccessControlList struct {
	XMLName xml.Name `xml:"dn-access-control-list:access-lists,omitempty" json:"access-lists,omitempty"`
	Ipv4    *Ipv4    `xml:"ipv4,omitempty" json:"ipv4,omitempty"`
}

// Access list ipv4 struct
type Ipv4 struct {
	XMLName    xml.Name   `xml:"ipv4,omitempty" json:"ipv4,omitempty"`
	AccessList AccessList `xml:"access-list,omitempty" json:"access-list,omitempty"`
}

type AccessList struct {
	XMLName     xml.Name    `xml:"access-list,omitempty" json:"access-list,omitempty"`
	ConfigItems ConfigItems `xml:"config-items,omitempty" json:"config-items,omitempty"`
	Name        string      `xml:"name,omitempty" json:"name,omitempty"`
	Rules       Rules       `xml:"rules,omitempty" json:"rules,omitempty"`
}

type ConfigItems struct {
	XMLName xml.Name `xml:"config-items,omitempty" json:"config-items,omitempty"`
	Name    string   `xml:"name,omitempty" json:"name,omitempty"`
}

type Rules struct {
	XMLName xml.Name `xml:"rules,omitempty" json:"rules,omitempty"`
	Rule    []Rule   `xml:"rule,omitempty" json:"rule,omitempty"`
}

// Access list rule
type Rule struct {
	XMLName         xml.Name        `xml:"rule,omitempty" json:"rule,omitempty"`
	RuleConfigItems RuleConfigItems `xml:"config-items,omitempty" json:"config-items,omitempty"`
	RuleId          int             `xml:"rule-id,omitempty" json:"rule-id,omitempty"`
}

type RuleConfigItems struct {
	XMLName     xml.Name    `xml:"config-items,omitempty" json:"config-items,omitempty"`
	Ipv4Matches Ipv4Matches `xml:"ipv4-matches,omitempty" json:"ipv4-matches,omitempty"`
	Matches     Matches     `xml:"matches,omitempty" json:"matches,omitempty"`
	Nexthops    Nexthops    `xml:"nexthops,omitempty" json:"nexthops,omitempty"`
	Protocol    string      `xml:"protocol,omitempty" json:"protocol,omitempty"`
	RuleType    string      `xml:"rule-type,omitempty" json:"rule-type,omitempty"`
}

type Ipv4Matches struct {
	XMLName      xml.Name     `xml:"ipv4-matches,omitempty" json:"ipv4-matches,omitempty"`
	Ipv4AclMatch Ipv4AclMatch `xml:"ipv4-acl-match,omitempty" json:"ipv4-acl-match,omitempty"`
}

type Ipv4AclMatch struct {
	XMLName         xml.Name `xml:"ipv4-acl-match,omitempty" json:"ipv4-acl-match,omitempty"`
	DestinationIpv4 string   `xml:"destination-ipv4" json:"destination-ipv4,omitempty"`
	SourceIpv4      string   `xml:"source-ipv4" json:"source-ipv4,omitempty"`
}

type Matches struct {
	XMLName    xml.Name   `xml:"matches,omitempty" json:"matches,omitempty"`
	L4AclMatch L4AclMatch `xml:"l4-acl-match,omitempty" json:"l4-acl-match,omitempty"`
}

type L4AclMatch struct {
	XMLName              xml.Name             `xml:"l4-acl-match,omitempty" json:"l4-acl-match,omitempty"`
	DestinationPortRange DestinationPortRange `xml:"destination-port-range,omitempty" json:"destination-port-range,omitempty"`
	SourcePortRange      SourcePortRange      `xml:"source-port-range,omitempty" json:"source-port-range,omitempty"`
}

type DestinationPortRange struct {
	XMLName   xml.Name `xml:"destination-port-range,omitempty" json:"destination-port-range,omitempty"`
	LowerPort uint16   `xml:"lower-port,omitempty" json:"lower-port,omitempty"`
}

type SourcePortRange struct {
	XMLName   xml.Name `xml:"source-port-range,omitempty" json:"source-port-range,omitempty"`
	LowerPort uint16   `xml:"lower-port,omitempty" json:"lower-port,omitempty"`
}

type Nexthops struct {
	XMLName  xml.Name `xml:"nexthops,omitempty" json:"nexthops,omitempty"`
	Nexthop1 Nexthop1 `xml:"nexthop1,omitempty" json:"nexthop1,omitempty"`
}

type Nexthop1 struct {
	XMLName xml.Name `xml:"nexthop1,omitempty" json:"nexthop1,omitempty"`
	Addr net.IP `xml:",chardata" json:",omitempty"`
}
