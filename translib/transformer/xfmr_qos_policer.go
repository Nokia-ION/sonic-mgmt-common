package transformer

////////////////////////////////////////////////////////////////////////////////
//
// Realization of the OpenConfig QoS classifier + scheduler-policy trees as
// SONiC ingress ACLs.
//
// SONiC has no classifier construct of its own. aclorch is the only component
// that can attach a policer or a traffic class to traffic selected by a packet
// match, so an OpenConfig classifier that is not a plain DSCP-to-TC map is
// realized as:
//
//	ACL_TABLE_TYPE  a custom type declaring the match fields in use plus
//	                PACKET_ACTION, POLICER_ACTION, SET_TC and COUNTER
//	ACL_TABLE       one ingress table per classifier, bound to the ports that
//	                bind the classifier or the scheduler-policy
//	ACL_RULE        one rule per classifier term, carrying the term's match
//	                fields and POLICER_ACTION / SET_TC
//	POLICER         one entry per policing scheduler, holding cir/cbs and the
//	                exceed action
//
// The classifier, the scheduler-policy and the interface binding arrive as
// independent requests, so neither side can assume the other is already in
// CONFIG_DB. The stitching is therefore recovered from the tables themselves:
// a rule's POLICER_ACTION names the policer, and the policer name encodes the
// scheduler-policy and sequence it came from. target-group is stored on the
// same rule as SET_TC (the SAI_ACL_ENTRY_ATTR_ACTION_SET_TC action).
//
// remark/set-dscp has no ingress ACL backend action yet, so it is rejected
// rather than silently dropped.
//
////////////////////////////////////////////////////////////////////////////////

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	lvl "github.com/Azure/sonic-mgmt-common/translib/log"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	log "github.com/golang/glog"
	"github.com/openconfig/ygot/ygot"
)

func init() {
	XlateFuncBind("YangToDb_qos_intf_input_sched_policy_xfmr", YangToDb_qos_intf_input_sched_policy_xfmr)
	XlateFuncBind("DbToYang_qos_intf_input_sched_policy_xfmr", DbToYang_qos_intf_input_sched_policy_xfmr)
}

const (
	qosAclTableTypeTbl = "ACL_TABLE_TYPE"
	qosAclTableTbl     = "ACL_TABLE"
	qosAclRuleTbl      = "ACL_RULE"
	qosPolicerTbl      = "POLICER"
	qosUmfClassifier   = "QOS_UMF_CLASSIFIER"

	qosAclTableTypeV4 = "QOS_POLICER_V4"
	qosAclTableTypeV6 = "QOS_POLICER_V6"

	/*
	 * Every action a classifier can ever need, declared up front rather than as
	 * each classifier turns out to use it. AclOrch::updateAclTable only carries
	 * the description and the port list over to an existing table, so a table
	 * keeps the table type it was created with: an action added to the type
	 * later never reaches the tables already standing, and their rules are
	 * rejected as "action is not supported on table". COUNTER is in the list
	 * even though nothing asks for it, because aclorch creates a counter per
	 * rule and builds the SAI action type list from this leaf-list.
	 */
	qosAclTableActions = "PACKET_ACTION,POLICER_ACTION,SET_TC,COUNTER"

	// Term ids count down from this base so that a numerically lower term id
	// is matched first, which is the order the OpenConfig term list implies.
	qosAclPrioBase = 60000
)

// Protocol numbers for the enum arm of the condition protocol union. The
// equivalent map in acl_app.go is not reachable from this package.
var qosIpProtocolNum = map[ocbinds.E_OpenconfigPacketMatchTypes_IP_PROTOCOL]uint8{
	ocbinds.OpenconfigPacketMatchTypes_IP_PROTOCOL_IP_ICMP: 1,
	ocbinds.OpenconfigPacketMatchTypes_IP_PROTOCOL_IP_IGMP: 2,
	ocbinds.OpenconfigPacketMatchTypes_IP_PROTOCOL_IP_TCP:  6,
	ocbinds.OpenconfigPacketMatchTypes_IP_PROTOCOL_IP_UDP:  17,
	ocbinds.OpenconfigPacketMatchTypes_IP_PROTOCOL_IP_RSVP: 46,
	ocbinds.OpenconfigPacketMatchTypes_IP_PROTOCOL_IP_GRE:  47,
	ocbinds.OpenconfigPacketMatchTypes_IP_PROTOCOL_IP_AUTH: 51,
	ocbinds.OpenconfigPacketMatchTypes_IP_PROTOCOL_IP_PIM:  103,
	ocbinds.OpenconfigPacketMatchTypes_IP_PROTOCOL_IP_L2TP: 115,
}

// Ethertype values for the enum arm of the L2 condition. sonic-acl.yang accepts
// only this set, spelled as uppercase hex. The equivalent map in acl_app.go is
// not reachable from this package.
var qosEthertypeNum = map[ocbinds.E_OpenconfigPacketMatchTypes_ETHERTYPE]uint16{
	ocbinds.OpenconfigPacketMatchTypes_ETHERTYPE_ETHERTYPE_LLDP: 0x88CC,
	ocbinds.OpenconfigPacketMatchTypes_ETHERTYPE_ETHERTYPE_VLAN: 0x8100,
	ocbinds.OpenconfigPacketMatchTypes_ETHERTYPE_ETHERTYPE_ROCE: 0x8915,
	ocbinds.OpenconfigPacketMatchTypes_ETHERTYPE_ETHERTYPE_ARP:  0x0806,
	ocbinds.OpenconfigPacketMatchTypes_ETHERTYPE_ETHERTYPE_IPV4: 0x0800,
	ocbinds.OpenconfigPacketMatchTypes_ETHERTYPE_ETHERTYPE_IPV6: 0x86DD,
	ocbinds.OpenconfigPacketMatchTypes_ETHERTYPE_ETHERTYPE_MPLS: 0x8847,
}

var qosTcpFlagBit = map[ocbinds.E_OpenconfigPacketMatchTypes_TCP_FLAGS]uint32{
	ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_FIN: 0x01,
	ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_SYN: 0x02,
	ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_RST: 0x04,
	ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_PSH: 0x08,
	ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_ACK: 0x10,
	ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_URG: 0x20,
	ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_ECE: 0x40,
	ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_CWR: 0x80,
}

// qosPolicerName derives the POLICER key for one scheduler of a policy. The
// POLICER name pattern disallows '@', which the SCHEDULER table uses.
func qosPolicerName(spName string, seq uint32) string {
	return fmt.Sprintf("%s_%d", spName, seq)
}

// qosPolicyOfPolicer inverts qosPolicerName to recover the scheduler-policy a
// policer belongs to. The policer name is the only record of that link, so a
// name not built by this module yields no policy.
func qosPolicyOfPolicer(policer string) string {
	i := strings.LastIndex(policer, "_")
	if i <= 0 || i == len(policer)-1 {
		return ""
	}
	if _, err := strconv.ParseUint(policer[i+1:], 10, 32); err != nil {
		return ""
	}
	return policer[:i]
}

func qosAclRuleName(termId string) string {
	return "TERM_" + termId
}

func qosAclTableType(family string) string {
	if family == "ipv6" {
		return qosAclTableTypeV6
	}
	return qosAclTableTypeV4
}

// qosClassifierFamily reports ipv4 or ipv6 for a classifier, preferring the
// configured type and falling back to whichever condition family is populated.
func qosClassifierFamily(cls *ocbinds.OpenconfigQos_Qos_Classifiers_Classifier) string {
	if cls.Config != nil {
		switch cls.Config.Type {
		case ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Config_Type_IPV6:
			return "ipv6"
		case ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Config_Type_IPV4:
			return "ipv4"
		}
	}
	if cls.Terms != nil {
		for _, term := range cls.Terms.Term {
			if term.Conditions != nil && term.Conditions.Ipv6 != nil && term.Conditions.Ipv6.Config != nil {
				return "ipv6"
			}
		}
	}
	return "ipv4"
}

// qosTermHasRemark reports whether a term asks to rewrite a packet field.
func qosTermHasRemark(term *ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term) bool {
	if term == nil || term.Actions == nil || term.Actions.Remark == nil ||
		term.Actions.Remark.Config == nil {
		return false
	}
	cfg := term.Actions.Remark.Config
	return cfg.SetDscp != nil || cfg.SetDot1P != nil || cfg.SetMplsTc != nil
}

// qosTermHasTargetGroup reports whether a term assigns a forwarding class.
func qosTermHasTargetGroup(term *ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term) bool {
	return term != nil && term.Actions != nil && term.Actions.Config != nil &&
		term.Actions.Config.TargetGroup != nil && *term.Actions.Config.TargetGroup != ""
}

// qosTermUsesAcl reports whether a term forces its classifier to be realized as
// an ACL rather than a DSCP_TO_TC_MAP entry. Richer-than-DSCP matches always do.
// So does a remark: DSCP_TO_TC_MAP derives a traffic class from the DSCP it
// reads, it cannot rewrite it.
func qosTermUsesAcl(term *ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term) bool {
	return qosTermIsPolicing(term) || qosTermHasRemark(term)
}

// qosTermIsAclRule reports whether a term of an ACL-backed classifier gets a
// rule of its own. Every term does, including a DSCP-only one that would have
// been a map entry had the classifier not been an ACL: skipping it would drop
// the operator's term without saying so. A term the request carries as a bare
// shell is the one exception, since it selects nothing.
func qosTermIsAclRule(term *ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term) bool {
	return term != nil && (term.Conditions != nil || term.Actions != nil)
}

// qosTermIsPolicing reports whether a term carries any match beyond a bare
// DSCP value. A DSCP-only term is a DSCP-to-traffic-class map entry, which
// xfmr_qos_classifier.go already owns; anything richer can only be realized
// as an ACL rule.
func qosTermIsPolicing(term *ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term) bool {
	if term == nil || term.Conditions == nil {
		return false
	}
	c := term.Conditions

	if c.Ipv4 != nil && c.Ipv4.Config != nil {
		cfg := c.Ipv4.Config
		if cfg.SourceAddress != nil || cfg.DestinationAddress != nil ||
			cfg.Protocol != nil || cfg.HopLimit != nil {
			return true
		}
	}
	if c.Ipv6 != nil && c.Ipv6.Config != nil {
		cfg := c.Ipv6.Config
		if cfg.SourceAddress != nil || cfg.DestinationAddress != nil ||
			cfg.Protocol != nil || cfg.HopLimit != nil {
			return true
		}
	}
	if c.Transport != nil && c.Transport.Config != nil {
		cfg := c.Transport.Config
		if cfg.SourcePort != nil || cfg.DestinationPort != nil || len(cfg.TcpFlags) > 0 {
			return true
		}
	}
	if c.L2 != nil && c.L2.Config != nil && c.L2.Config.Ethertype != nil {
		return true
	}
	return false
}

// qosClassifierIsPolicing reports whether a classifier is realized as an ACL.
// The request tree is authoritative when it carries the classifier; otherwise
// the bookkeeping written by an earlier request decides.
func qosClassifierIsPolicing(inParams XfmrParams, name string) bool {
	qosObj := getQosRoot(inParams.ygRoot)
	if qosObj != nil && qosObj.Classifiers != nil {
		if cls, ok := qosObj.Classifiers.Classifier[name]; ok && cls.Terms != nil {
			for _, term := range cls.Terms.Term {
				if qosTermUsesAcl(term) {
					return true
				}
			}
		}
	}

	// An earlier request leaves an ACL table of a QoS policer type named after
	// the classifier. Requiring the type matters: an ACL the operator
	// configured through /acl could share the name, and treating it as ours
	// would let a classifier delete tear down their table and rules.
	return qosPolicedClassifierInDb(inParams.d, name)
}

// qosAclMatchFields are the ACL_RULE fields that select traffic, as opposed to
// the ones that act on it. Used to carry a rule's matches over an update that
// only addresses its actions.
var qosAclMatchFields = map[string]bool{
	"IP_TYPE": true, "IP_PROTOCOL": true, "DSCP": true, "ETHER_TYPE": true,
	"SRC_IP": true, "DST_IP": true, "SRC_IPV6": true, "DST_IPV6": true,
	"L4_SRC_PORT": true, "L4_DST_PORT": true,
	"L4_SRC_PORT_RANGE": true, "L4_DST_PORT_RANGE": true,
	"TCP_FLAGS": true,
}

// qosAclRuleFields maps one classifier term's conditions onto ACL_RULE fields,
// returning the field set and the match names the table type must declare.
// existing is the rule already in the table, if any, and is what the term keeps
// matching on when the request does not restate its conditions.
func qosAclRuleFields(termId string, term *ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term,
	family string, existing db.Value) (db.Value, []string, error) {

	val := db.Value{Field: make(map[string]string)}
	matches := []string{"IP_TYPE"}

	id, err := strconv.ParseUint(termId, 10, 32)
	if err != nil {
		return val, nil, tlerr.InvalidArgsError{
			Format: "Classifier term id must be numeric for a policed classifier, got " + termId}
	}
	if id >= qosAclPrioBase {
		return val, nil, tlerr.InvalidArgsError{
			Format: fmt.Sprintf("Classifier term id must be below %d, got %s", qosAclPrioBase, termId),
		}
	}
	val.Field["PRIORITY"] = strconv.FormatUint(qosAclPrioBase-id, 10)

	// PACKET_ACTION is mandatory on every ACL rule. Matching traffic is
	// forwarded; it is the policer that drops whatever exceeds the rate.
	val.Field["PACKET_ACTION"] = "FORWARD"

	if family == "ipv6" {
		val.Field["IP_TYPE"] = "IPV6ANY"
	} else {
		val.Field["IP_TYPE"] = "IPV4ANY"
	}

	addMatch := func(name string) {
		for _, m := range matches {
			if m == name {
				return
			}
		}
		matches = append(matches, name)
	}

	/*
	 * A request addressing only a term's actions carries no conditions, as in
	 * a write to .../actions/remark/config/set-dscp on a term that already
	 * exists. The rule goes on matching what it already matched, so carry
	 * those fields over rather than refusing a term that was accepted when it
	 * was created.
	 */
	if term.Conditions == nil {
		for field, value := range existing.Field {
			if qosAclMatchFields[field] {
				val.Field[field] = value
				addMatch(field)
			}
		}
	}

	// A term with no conditions at all still has to reach the backstop below,
	// so that it is refused rather than turned into a rule matching everything.
	c := term.Conditions
	if c == nil {
		c = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions{}
	}

	// A condition with no ACL_RULE equivalent cannot be honoured, and ignoring
	// it would widen the rule to traffic the operator did not ask to police.
	if (c.Ipv4 != nil && c.Ipv4.Config != nil && c.Ipv4.Config.HopLimit != nil) ||
		(c.Ipv6 != nil && c.Ipv6.Config != nil && c.Ipv6.Config.HopLimit != nil) {
		return val, nil, tlerr.NotSupportedError{
			Format: "Classifier term " + termId + " hop-limit condition is not supported",
		}
	}

	if c.Ipv4 != nil && c.Ipv4.Config != nil {
		cfg := c.Ipv4.Config
		if cfg.SourceAddress != nil {
			val.Field["SRC_IP"] = *cfg.SourceAddress
			addMatch("SRC_IP")
		}
		if cfg.DestinationAddress != nil {
			val.Field["DST_IP"] = *cfg.DestinationAddress
			addMatch("DST_IP")
		}
		if cfg.Dscp != nil {
			val.Field["DSCP"] = strconv.Itoa(int(*cfg.Dscp))
			addMatch("DSCP")
		}
		if cfg.Protocol != nil {
			proto, err := qosIpv4Protocol(cfg.Protocol)
			if err != nil {
				return val, nil, err
			}
			val.Field["IP_PROTOCOL"] = proto
			addMatch("IP_PROTOCOL")
		}
	}

	if c.Ipv6 != nil && c.Ipv6.Config != nil {
		cfg := c.Ipv6.Config
		if cfg.SourceAddress != nil {
			val.Field["SRC_IPV6"] = *cfg.SourceAddress
			addMatch("SRC_IPV6")
		}
		if cfg.DestinationAddress != nil {
			val.Field["DST_IPV6"] = *cfg.DestinationAddress
			addMatch("DST_IPV6")
		}
		if cfg.Dscp != nil {
			val.Field["DSCP"] = strconv.Itoa(int(*cfg.Dscp))
			addMatch("DSCP")
		}
		if cfg.Protocol != nil {
			proto, err := qosIpv6Protocol(cfg.Protocol)
			if err != nil {
				return val, nil, err
			}
			val.Field["IP_PROTOCOL"] = proto
			addMatch("IP_PROTOCOL")
		}
	}

	if c.Transport != nil && c.Transport.Config != nil {
		cfg := c.Transport.Config
		if cfg.SourcePort != nil {
			field, value, err := qosTransportPort(cfg.SourcePort, "SRC")
			if err != nil {
				return val, nil, err
			}
			if field != "" {
				val.Field[field] = value
				addMatch(field)
			}
		}
		if cfg.DestinationPort != nil {
			field, value, err := qosTransportPort(cfg.DestinationPort, "DST")
			if err != nil {
				return val, nil, err
			}
			if field != "" {
				val.Field[field] = value
				addMatch(field)
			}
		}
		if len(cfg.TcpFlags) > 0 {
			var flags uint32
			for _, f := range cfg.TcpFlags {
				bit, ok := qosTcpFlagBit[f]
				if !ok {
					return val, nil, tlerr.InvalidArgsError{Format: "Unsupported TCP flag in classifier term " + termId}
				}
				flags |= bit
			}
			val.Field["TCP_FLAGS"] = fmt.Sprintf("0x%0.2x/0x%0.2x", flags, flags)
			addMatch("TCP_FLAGS")
		}
	}

	if c.L2 != nil && c.L2.Config != nil && c.L2.Config.Ethertype != nil {
		ethertype, err := qosEthertype(c.L2.Config.Ethertype)
		if err != nil {
			return val, nil, err
		}
		val.Field["ETHER_TYPE"] = ethertype
		addMatch("ETHER_TYPE")
	}

	// IP_TYPE on its own means nothing in the term selected any traffic, so the
	// rule would match every packet on the port and police or rewrite all of
	// it. Fail the request rather than program that.
	if len(matches) == 1 {
		return val, nil, tlerr.NotSupportedError{
			Format: "Classifier term " + termId + " has no condition that maps to an ACL match",
		}
	}

	return val, matches, nil
}

func qosIpv4Protocol(u ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_Ipv4_Config_Protocol_Union) (string, error) {
	switch v := u.(type) {
	case *ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_Ipv4_Config_Protocol_Union_Uint8:
		return strconv.Itoa(int(v.Uint8)), nil
	case *ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_Ipv4_Config_Protocol_Union_E_OpenconfigPacketMatchTypes_IP_PROTOCOL:
		num, ok := qosIpProtocolNum[v.E_OpenconfigPacketMatchTypes_IP_PROTOCOL]
		if !ok {
			return "", tlerr.InvalidArgsError{Format: "Unsupported IP protocol in classifier condition"}
		}
		return strconv.Itoa(int(num)), nil
	}
	return "", tlerr.InvalidArgsError{Format: "Unrecognized IP protocol value in classifier condition"}
}

func qosIpv6Protocol(u ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_Ipv6_Config_Protocol_Union) (string, error) {
	switch v := u.(type) {
	case *ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_Ipv6_Config_Protocol_Union_Uint8:
		return strconv.Itoa(int(v.Uint8)), nil
	case *ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_Ipv6_Config_Protocol_Union_E_OpenconfigPacketMatchTypes_IP_PROTOCOL:
		num, ok := qosIpProtocolNum[v.E_OpenconfigPacketMatchTypes_IP_PROTOCOL]
		if !ok {
			return "", tlerr.InvalidArgsError{Format: "Unsupported IP protocol in classifier condition"}
		}
		return strconv.Itoa(int(num)), nil
	}
	return "", tlerr.InvalidArgsError{Format: "Unrecognized IP protocol value in classifier condition"}
}

// qosTransportPort returns the ACL field name and value for a transport port
// condition. A range arrives as the string arm and maps to the _RANGE field.
// An empty field name means the condition matches any port and so needs no
// match field at all.
func qosTransportPort(u interface{}, dir string) (string, string, error) {
	switch v := u.(type) {
	case *ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_Transport_Config_SourcePort_Union_Uint16:
		return "L4_" + dir + "_PORT", strconv.Itoa(int(v.Uint16)), nil
	case *ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_Transport_Config_SourcePort_Union_String:
		return "L4_" + dir + "_PORT_RANGE", strings.Replace(v.String, "..", "-", 1), nil
	case *ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_Transport_Config_SourcePort_Union_E_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_SourcePort:
		return "", "", nil
	case *ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_Transport_Config_DestinationPort_Union_Uint16:
		return "L4_" + dir + "_PORT", strconv.Itoa(int(v.Uint16)), nil
	case *ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_Transport_Config_DestinationPort_Union_String:
		return "L4_" + dir + "_PORT_RANGE", strings.Replace(v.String, "..", "-", 1), nil
	case *ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_Transport_Config_DestinationPort_Union_E_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_DestinationPort:
		return "", "", nil
	}
	return "", "", tlerr.InvalidArgsError{Format: "Unsupported transport port condition"}
}

// qosEthertype renders an ethertype the way sonic-acl.yang wants it, which is
// uppercase hex behind a lowercase prefix. A named type has to be resolved to
// its value; the name itself does not pass validation.
func qosEthertype(u interface{}) (string, error) {
	switch v := u.(type) {
	case *ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_L2_Config_Ethertype_Union_Uint16:
		return fmt.Sprintf("0x%04X", v.Uint16), nil
	case *ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_L2_Config_Ethertype_Union_E_OpenconfigPacketMatchTypes_ETHERTYPE:
		num, ok := qosEthertypeNum[v.E_OpenconfigPacketMatchTypes_ETHERTYPE]
		if !ok {
			return "", tlerr.NotSupportedError{
				Format: "Unsupported ethertype " + strings.TrimPrefix(
					v.E_OpenconfigPacketMatchTypes_ETHERTYPE.ΛMap()["E_OpenconfigPacketMatchTypes_ETHERTYPE"][int64(v.E_OpenconfigPacketMatchTypes_ETHERTYPE)].Name,
					"ETHERTYPE_"),
			}
		}
		return fmt.Sprintf("0x%04X", num), nil
	}
	return "", tlerr.InvalidArgsError{Format: "Unsupported ethertype condition"}
}

// qosAclRuleInDb returns the rule already backing a classifier term, or an
// empty value when the term is new.
func qosAclRuleInDb(inParams XfmrParams, name, termId string) db.Value {
	if inParams.d == nil {
		return db.Value{Field: make(map[string]string)}
	}
	entry, err := inParams.d.GetEntry(&db.TableSpec{Name: qosAclRuleTbl},
		db.Key{Comp: []string{name, qosAclRuleName(termId)}})
	if err != nil || !entry.IsPopulated() {
		return db.Value{Field: make(map[string]string)}
	}
	return entry
}

// qosClassifierToAcl builds the ACL objects backing a policed classifier and
// merges them into resMap.
func qosClassifierToAcl(inParams XfmrParams, name string,
	cls *ocbinds.OpenconfigQos_Qos_Classifiers_Classifier,
	resMap map[string]map[string]db.Value) error {

	if cls.Terms == nil || len(cls.Terms.Term) == 0 {
		return nil
	}

	family := qosClassifierFamily(cls)
	tableType := qosAclTableType(family)

	// The policer may already be known from an earlier scheduler-policy write.
	policer := qosPolicerForClassifier(inParams, name)

	allMatches := []string{}
	ruleEntries := make(map[string]db.Value)

	for termId, term := range cls.Terms.Term {
		if !qosTermIsAclRule(term) {
			continue
		}
		ruleVal, matches, err := qosAclRuleFields(termId, term, family,
			qosAclRuleInDb(inParams, name, termId))
		if err != nil {
			return err
		}
		if err := qosApplyTermActions(inParams, term, &ruleVal); err != nil {
			return err
		}
		if policer != "" {
			ruleVal.Field["POLICER_ACTION"] = policer
		}
		ruleEntries[name+"|"+qosAclRuleName(termId)] = ruleVal

		for _, m := range matches {
			allMatches = qosAppendUnique(allMatches, m)
		}
	}

	if len(ruleEntries) == 0 {
		return nil
	}

	/*
	 * Every classifier of the same family shares one table type, so fold in the
	 * matches it already declares. Replacing the list would strip a match that
	 * another classifier's rule still uses, and aclorch builds its SAI match
	 * field list from this leaf-list.
	 */
	if inParams.d != nil {
		if entry, err := inParams.d.GetEntry(&db.TableSpec{Name: qosAclTableTypeTbl},
			db.Key{Comp: []string{tableType}}); err == nil {
			for _, m := range strings.Split(entry.Field["MATCHES@"], ",") {
				if m = strings.TrimSpace(m); m != "" {
					allMatches = qosAppendUnique(allMatches, m)
				}
			}
		}
	}

	// A replace has to drop the rules of terms the request no longer carries,
	// which an update must leave alone.
	if inParams.oper == REPLACE {
		qosPruneStaleRules(inParams, name, ruleEntries)
	}

	typeVal := db.Value{Field: make(map[string]string)}
	typeVal.Field["MATCHES@"] = strings.Join(allMatches, ",")
	typeVal.Field["ACTIONS@"] = qosAclTableActions
	typeVal.Field["BIND_POINTS@"] = "PORT"
	qosMergeTable(resMap, qosAclTableTypeTbl, map[string]db.Value{tableType: typeVal})

	tblVal := db.Value{Field: make(map[string]string)}
	tblVal.Field["type"] = tableType
	tblVal.Field["stage"] = "INGRESS"
	tblVal.Field["policy_desc"] = "OpenConfig QoS classifier " + name
	if ports := qosPortsForClassifier(inParams, name); len(ports) > 0 {
		tblVal.Field["ports@"] = strings.Join(ports, ",")
	}
	qosMergeTable(resMap, qosAclTableTbl, map[string]db.Value{name: tblVal})

	qosMergeTable(resMap, qosAclRuleTbl, ruleEntries)

	umfVal := db.Value{Field: make(map[string]string)}
	umfVal.Field["type"] = family
	qosMergeTable(resMap, qosUmfClassifier, map[string]db.Value{name: umfVal})

	log.V(lvl.DEBUG).Infof("qosClassifierToAcl: classifier %v family %v rules %v policer %q",
		name, family, len(ruleEntries), policer)
	return nil
}

// qosAppendUnique adds a value to a list if it is not already there, keeping the
// order the caller built it in.
func qosAppendUnique(list []string, val string) []string {
	for _, existing := range list {
		if existing == val {
			return list
		}
	}
	return append(list, val)
}

// qosTargetGroupToTc resolves a forwarding-group name to the traffic-class
// number stored as QOS_FWD_GROUP.priority / fabric-priority.
func qosTargetGroupToTc(inParams XfmrParams, name string) (string, error) {
	if name == "" {
		return "", tlerr.InvalidArgsError{Format: "target-group is empty"}
	}
	qosObj := getQosRoot(inParams.ygRoot)
	if qosObj != nil && qosObj.ForwardingGroups != nil {
		if fg, ok := qosObj.ForwardingGroups.ForwardingGroup[name]; ok &&
			fg.Config != nil && fg.Config.FabricPriority != nil {
			return strconv.Itoa(int(*fg.Config.FabricPriority)), nil
		}
	}
	if prio := get_fwd_group_priority(inParams, name); prio != "" {
		return prio, nil
	}
	return "", tlerr.NotFoundError{Format: "Forwarding group " + name + " not found"}
}

// qosApplyTermActions writes the SET_TC field from a classifier term onto the
// ACL rule that realizes it. target-group resolves to a traffic class, stored
// as the aclorch SET_TC action (SAI_ACL_ENTRY_ATTR_ACTION_SET_TC).
//
// remark actions (set-dscp, set-dot1p, set-mpls-tc) have no ingress ACL backend
// action, so they are rejected rather than silently dropped.
func qosApplyTermActions(inParams XfmrParams,
	term *ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term,
	ruleVal *db.Value) error {

	if term == nil || ruleVal == nil {
		return nil
	}
	if qosTermHasTargetGroup(term) {
		tc, err := qosTargetGroupToTc(inParams, *term.Actions.Config.TargetGroup)
		if err != nil {
			return err
		}
		ruleVal.Field["SET_TC"] = tc
	}
	if qosTermHasRemark(term) {
		return tlerr.NotSupportedError{
			Format: "Classifier remark actions (set-dscp, set-dot1p, set-mpls-tc) are not supported",
		}
	}
	return nil
}

// qosPruneStaleRules asks translib to delete the rules of a classifier that the
// replacing request no longer carries. A subtree transformer can only return
// entries for its own operation, so the removals ride along as a DELETE
// sub-operation.
func qosPruneStaleRules(inParams XfmrParams, name string, keep map[string]db.Value) {
	stale := 0
	for _, ruleKey := range qosAclRuleKeys(inParams, name) {
		if _, ok := keep[ruleKey]; ok {
			continue
		}
		qosStageDelete(inParams, qosAclRuleTbl, ruleKey, db.Value{Field: make(map[string]string)})
		stale++
	}
	log.V(lvl.DEBUG).Infof("qosPruneStaleRules: classifier %v stale rules %v", name, stale)
}

// qosStageDelete adds one entry to the request's DELETE sub-operation. A subtree
// transformer can only return entries for the operation it was called for, so a
// removal that has to happen as part of a write rides along this way.
func qosStageDelete(inParams XfmrParams, table string, key string, val db.Value) {
	if inParams.subOpDataMap == nil {
		return
	}
	if inParams.subOpDataMap[DELETE] == nil {
		subOpMap := make(map[db.DBNum]map[string]map[string]db.Value)
		inParams.subOpDataMap[DELETE] = &subOpMap
	}
	if (*inParams.subOpDataMap[DELETE])[db.ConfigDB] == nil {
		(*inParams.subOpDataMap[DELETE])[db.ConfigDB] = make(map[string]map[string]db.Value)
	}
	delMap := (*inParams.subOpDataMap[DELETE])[db.ConfigDB]
	if delMap[table] == nil {
		delMap[table] = make(map[string]db.Value)
	}
	delMap[table][key] = val
}

// qosMergeTable folds entries into resMap without dropping fields another
// caller already staged for the same key.
func qosMergeTable(resMap map[string]map[string]db.Value, table string, entries map[string]db.Value) {
	if _, ok := resMap[table]; !ok {
		resMap[table] = make(map[string]db.Value)
	}
	for key, val := range entries {
		existing, ok := resMap[table][key]
		if !ok || existing.Field == nil {
			resMap[table][key] = val
			continue
		}
		for f, v := range val.Field {
			existing.Field[f] = v
		}
		resMap[table][key] = existing
	}
}

// qosPolicerForClassifier finds the policer bound to a classifier, looking in
// the request tree first and then at the bookkeeping left by earlier writes.
func qosPolicerForClassifier(inParams XfmrParams, clsName string) string {
	qosObj := getQosRoot(inParams.ygRoot)
	if qosObj != nil && qosObj.SchedulerPolicies != nil {
		for spName, sp := range qosObj.SchedulerPolicies.SchedulerPolicy {
			if sp.Schedulers == nil {
				continue
			}
			for seq, sched := range sp.Schedulers.Scheduler {
				if sched.Inputs == nil {
					continue
				}
				for _, in := range sched.Inputs.Input {
					if in.Config != nil && in.Config.Id != nil && *in.Config.Id == clsName {
						return qosPolicerName(spName, seq)
					}
				}
			}
		}
	}

	if inParams.d == nil {
		return ""
	}
	// Otherwise the classifier's own rules record the policer they police with.
	for _, ruleKey := range qosAclRuleKeys(inParams, clsName) {
		entry, err := inParams.d.GetEntry(&db.TableSpec{Name: qosAclRuleTbl},
			db.Key{Comp: strings.Split(ruleKey, "|")})
		if err != nil {
			continue
		}
		if policer := entry.Field["POLICER_ACTION"]; policer != "" {
			return policer
		}
	}
	return ""
}

// qosPortsForClassifier returns the interfaces whose ingress scheduler-policy
// resolves to this classifier.
func qosPortsForClassifier(inParams XfmrParams, clsName string) []string {
	ports := []string{}
	if inParams.d == nil {
		return ports
	}

	entry, err := inParams.d.GetEntry(&db.TableSpec{Name: qosAclTableTbl}, db.Key{Comp: []string{clsName}})
	if err != nil {
		return ports
	}
	for _, p := range strings.Split(entry.Field["ports@"], ",") {
		if p != "" {
			ports = append(ports, p)
		}
	}
	return ports
}

// qosAclClassifiersOnIntf lists the ACL-backed classifiers bound to an
// interface through /qos/interfaces/interface/input/classifiers, as the
// interface being in the backing table's ports.
//
// A policed classifier is left out. Its interface list is owned by
// /input/scheduler-policy, which put the interface there when the policy was
// bound, so reporting it here would let a delete under /input/classifiers
// unbind a policy nobody asked to touch.
func qosAclClassifiersOnIntf(inParams XfmrParams, ifName string) []string {
	names := []string{}
	if inParams.d == nil || ifName == "" {
		return names
	}
	keys, err := inParams.d.GetKeysByPattern(&db.TableSpec{Name: qosAclTableTbl}, "*")
	if err != nil {
		return names
	}
	for _, key := range keys {
		if len(key.Comp) == 0 {
			continue
		}
		entry, err := inParams.d.GetEntry(&db.TableSpec{Name: qosAclTableTbl}, key)
		if err != nil {
			continue
		}
		if t := entry.Field["type"]; t != qosAclTableTypeV4 && t != qosAclTableTypeV6 {
			continue
		}
		if qosPolicerForClassifier(inParams, key.Comp[0]) != "" {
			continue
		}
		for _, p := range strings.Split(entry.Field["ports@"], ",") {
			if p == ifName {
				names = append(names, key.Comp[0])
				break
			}
		}
	}
	return names
}

// qosClassifierIsPolicedInDb reports whether a classifier's rules carry a
// policer, off a plain DB handle rather than an XfmrParams.
func qosClassifierIsPolicedInDb(d *db.DB, clsName string) bool {
	if d == nil || clsName == "" {
		return false
	}
	keys, err := d.GetKeysByPattern(&db.TableSpec{Name: qosAclRuleTbl}, clsName+"|*")
	if err != nil {
		return false
	}
	for _, key := range keys {
		entry, err := d.GetEntry(&db.TableSpec{Name: qosAclRuleTbl}, key)
		if err != nil {
			continue
		}
		if entry.Field["POLICER_ACTION"] != "" {
			return true
		}
	}
	return false
}

// qosAclClassifierBoundToIntf names an ACL-backed classifier of the given
// family bound to an interface, or "" for either family. The subscribe path
// has only a DB handle to work from, so it cannot use the lister above.
//
// A policed classifier is left out for the same reason it is there: its
// interface list belongs to /input/scheduler-policy, so reporting it as a
// binding under /input/classifiers would let a delete there act on an
// interface that has no such binding.
func qosAclClassifierBoundToIntf(d *db.DB, ifName, family string) string {
	if d == nil || ifName == "" {
		return ""
	}
	keys, err := d.GetKeysByPattern(&db.TableSpec{Name: qosAclTableTbl}, "*")
	if err != nil {
		return ""
	}
	for _, key := range keys {
		if len(key.Comp) == 0 {
			continue
		}
		entry, err := d.GetEntry(&db.TableSpec{Name: qosAclTableTbl}, key)
		if err != nil {
			continue
		}
		switch entry.Field["type"] {
		case qosAclTableTypeV4:
			if family == "ipv6" {
				continue
			}
		case qosAclTableTypeV6:
			if family == "ipv4" {
				continue
			}
		default:
			continue
		}
		if qosClassifierIsPolicedInDb(d, key.Comp[0]) {
			continue
		}
		for _, p := range strings.Split(entry.Field["ports@"], ",") {
			if p == ifName {
				return key.Comp[0]
			}
		}
	}
	return ""
}

// qosAclClassifierFamily reports whether the table backing a classifier is the
// v4 or the v6 one, for the type key of the interface classifier list.
func qosAclClassifierFamily(inParams XfmrParams, clsName string) string {
	if inParams.d == nil {
		return "ipv4"
	}
	entry, err := inParams.d.GetEntry(&db.TableSpec{Name: qosAclTableTbl},
		db.Key{Comp: []string{clsName}})
	if err == nil && entry.Field["type"] == qosAclTableTypeV6 {
		return "ipv6"
	}
	return "ipv4"
}

// qosSchedulerPolicerClassifier reports the classifier a scheduler polices, if
// any. A scheduler whose input names a policed classifier is an ingress
// policer rather than an egress queue scheduler.
func qosSchedulerPolicerClassifier(inParams XfmrParams,
	sched *ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler) string {

	if sched == nil || sched.Inputs == nil {
		return ""
	}
	for _, in := range sched.Inputs.Input {
		if in.Config == nil || in.Config.Id == nil {
			continue
		}
		if in.Config.InputType == ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler_Inputs_Input_Config_InputType_QUEUE {
			continue
		}
		if qosClassifierIsPolicing(inParams, *in.Config.Id) {
			return *in.Config.Id
		}
	}
	return ""
}

// qosSchedulerToPolicer builds the POLICER entry for a policing scheduler and
// attaches it to the backing classifier's rules.
func qosSchedulerToPolicer(inParams XfmrParams, spName string, seq uint32, clsName string,
	sched *ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler,
	resMap map[string]map[string]db.Value) error {

	policer := qosPolicerName(spName, seq)
	val := db.Value{Field: make(map[string]string)}
	val.Field["meter_type"] = "bytes"
	// Colour-blind: the rate limit is applied without regard to any marking the
	// packet arrives with.
	val.Field["color"] = "blind"

	var haveRate bool

	if sched.TwoRateThreeColor != nil && sched.TwoRateThreeColor.Config != nil {
		cfg := sched.TwoRateThreeColor.Config
		val.Field["mode"] = "tr_tcm"
		// OpenConfig states cir and pir in bits per second while a byte-mode
		// SONiC policer counts bytes per second.
		if cfg.Cir != nil {
			val.Field["cir"] = strconv.FormatUint(*cfg.Cir/8, 10)
			haveRate = true
		}
		if cfg.Pir != nil {
			val.Field["pir"] = strconv.FormatUint(*cfg.Pir/8, 10)
			haveRate = true
		}
		if cfg.Bc != nil {
			val.Field["cbs"] = strconv.FormatUint(uint64(*cfg.Bc), 10)
		}
		if cfg.Be != nil {
			val.Field["pbs"] = strconv.FormatUint(uint64(*cfg.Be), 10)
		}
		if err := qosApplyColorActions(sched.TwoRateThreeColor.ExceedAction, sched.TwoRateThreeColor.ConformAction, val); err != nil {
			return err
		}
	} else if sched.OneRateTwoColor != nil && sched.OneRateTwoColor.Config != nil {
		cfg := sched.OneRateTwoColor.Config
		val.Field["mode"] = "sr_tcm"
		if cfg.Cir != nil {
			val.Field["cir"] = strconv.FormatUint(*cfg.Cir/8, 10)
			haveRate = true
		}
		if cfg.Bc != nil {
			val.Field["cbs"] = strconv.FormatUint(uint64(*cfg.Bc), 10)
		}
		if err := qosApplyOneRateActions(sched.OneRateTwoColor, val); err != nil {
			return err
		}
	} else {
		return tlerr.InvalidArgsError{
			Format: "Policing scheduler " + spName + " needs one-rate-two-color or two-rate-three-color config",
		}
	}

	if !haveRate {
		return tlerr.InvalidArgsError{Format: "Policing scheduler " + spName + " needs a cir"}
	}

	qosMergeTable(resMap, qosPolicerTbl, map[string]db.Value{policer: val})

	// Point the classifier's rules at the policer. This is also the only record
	// of which classifier the policy polices, which the interface binding and
	// the read path both recover by inverting the policer name.
	for _, ruleKey := range qosAclRuleKeys(inParams, clsName) {
		ruleVal := db.Value{Field: map[string]string{"POLICER_ACTION": policer}}
		qosMergeTable(resMap, qosAclRuleTbl, map[string]db.Value{ruleKey: ruleVal})
	}

	log.V(lvl.DEBUG).Infof("qosSchedulerToPolicer: policy %v seq %v classifier %v policer %v",
		spName, seq, clsName, policer)
	return nil
}

// qosApplyOneRateActions maps the conform and exceed actions of a one-rate,
// two-colour policer onto the POLICER colour actions.
func qosApplyOneRateActions(orc *ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler_OneRateTwoColor,
	val db.Value) error {

	val.Field["green_packet_action"] = "forward"
	val.Field["red_packet_action"] = "forward"

	if orc.ExceedAction != nil && orc.ExceedAction.Config != nil {
		cfg := orc.ExceedAction.Config
		if cfg.SetDscp != nil || cfg.SetDot1P != nil || cfg.SetMplsTc != nil {
			return tlerr.NotSupportedError{
				Format: "Policer exceed-action remarking is not supported; only drop is",
			}
		}
		if cfg.Drop != nil && *cfg.Drop {
			val.Field["red_packet_action"] = "drop"
		}
	}
	if orc.ConformAction != nil && orc.ConformAction.Config != nil {
		cfg := orc.ConformAction.Config
		if cfg.SetDscp != nil || cfg.SetDot1P != nil || cfg.SetMplsTc != nil {
			return tlerr.NotSupportedError{
				Format: "Policer conform-action remarking is not supported",
			}
		}
	}
	return nil
}

// qosApplyColorActions handles the three-colour case, where exceeding maps to
// yellow and violating maps to red.
func qosApplyColorActions(exceed *ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler_TwoRateThreeColor_ExceedAction,
	conform *ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler_TwoRateThreeColor_ConformAction,
	val db.Value) error {

	val.Field["green_packet_action"] = "forward"
	val.Field["yellow_packet_action"] = "forward"
	val.Field["red_packet_action"] = "drop"

	if exceed != nil && exceed.Config != nil {
		cfg := exceed.Config
		if cfg.SetDscp != nil || cfg.SetDot1P != nil || cfg.SetMplsTc != nil {
			return tlerr.NotSupportedError{
				Format: "Policer exceed-action remarking is not supported; only drop is",
			}
		}
		if cfg.Drop != nil && *cfg.Drop {
			val.Field["yellow_packet_action"] = "drop"
		}
	}
	if conform != nil && conform.Config != nil {
		cfg := conform.Config
		if cfg.SetDscp != nil || cfg.SetDot1P != nil || cfg.SetMplsTc != nil {
			return tlerr.NotSupportedError{
				Format: "Policer conform-action remarking is not supported",
			}
		}
	}
	return nil
}

// qosPolicedClassifierNames lists the classifiers backed by a synthesized ACL
// table, optionally narrowed to one name.
func qosPolicedClassifierNames(inParams XfmrParams, name string) []string {
	names := []string{}
	if inParams.d == nil {
		return names
	}

	pattern := "*"
	if name != "" {
		pattern = name
	}
	keys, err := inParams.d.GetKeysByPattern(&db.TableSpec{Name: qosAclTableTbl}, pattern)
	if err != nil {
		return names
	}
	for _, key := range keys {
		if len(key.Comp) == 0 {
			continue
		}
		entry, err := inParams.d.GetEntry(&db.TableSpec{Name: qosAclTableTbl}, key)
		if err != nil {
			continue
		}
		switch entry.Field["type"] {
		case qosAclTableTypeV4, qosAclTableTypeV6:
			names = append(names, key.Comp[0])
		}
	}
	return names
}

// qosPolicedClassifierInDb reports whether a classifier is backed by a
// synthesized ACL table. It takes a raw DB handle so that the subscribe
// callback, which has no XfmrParams, can use it too.
func qosPolicedClassifierInDb(d *db.DB, name string) bool {
	if d == nil || name == "" {
		return false
	}
	entry, err := d.GetEntry(&db.TableSpec{Name: qosAclTableTbl}, db.Key{Comp: []string{name}})
	if err != nil || !entry.IsPopulated() {
		return false
	}
	switch entry.Field["type"] {
	case qosAclTableTypeV4, qosAclTableTypeV6:
		return true
	}
	return false
}

// qosFillClassifiersFromAcl rebuilds the OpenConfig view of policed classifiers
// from the ACL rules backing them, and reports how many it added. Only the
// config containers are populated; the parallel state containers are separate
// generated types and are not mirrored here.
func qosFillClassifiersFromAcl(inParams XfmrParams,
	classifiers *ocbinds.OpenconfigQos_Qos_Classifiers, name string) (int, error) {

	added := 0
	for _, clsName := range qosPolicedClassifierNames(inParams, name) {
		clsObj, ok := classifiers.Classifier[clsName]
		if !ok {
			var err error
			clsObj, err = classifiers.NewClassifier(clsName)
			if err != nil {
				return added, err
			}
		}
		ygot.BuildEmptyTree(clsObj)

		family := "ipv4"
		if entry, err := inParams.d.GetEntry(&db.TableSpec{Name: qosUmfClassifier}, db.Key{Comp: []string{clsName}}); err == nil {
			if entry.Field["type"] == "ipv6" {
				family = "ipv6"
			}
		}

		// A request below the classifier yields a partially built tree, so the
		// containers touched here are allocated rather than assumed.
		if clsObj.Config == nil {
			clsObj.Config = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Config{}
		}
		if clsObj.State == nil {
			clsObj.State = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_State{}
		}
		if clsObj.Terms == nil {
			clsObj.Terms = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms{}
		}

		nameCopy := clsName
		clsObj.Name = &nameCopy
		clsObj.Config.Name = &nameCopy
		clsObj.State.Name = &nameCopy
		if family == "ipv6" {
			clsObj.Config.Type = ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Config_Type_IPV6
			clsObj.State.Type = ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Config_Type_IPV6
		} else {
			clsObj.Config.Type = ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Config_Type_IPV4
			clsObj.State.Type = ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Config_Type_IPV4
		}

		ruleKeys, err := inParams.d.GetKeysByPattern(&db.TableSpec{Name: qosAclRuleTbl}, clsName+"|*")
		if err != nil {
			added++
			continue
		}
		for _, rk := range ruleKeys {
			if len(rk.Comp) < 2 {
				continue
			}
			termId := strings.TrimPrefix(rk.Comp[1], "TERM_")
			rule, err := inParams.d.GetEntry(&db.TableSpec{Name: qosAclRuleTbl}, rk)
			if err != nil {
				continue
			}
			if err := qosFillTermFromRule(inParams, clsObj, termId, family, rule); err != nil {
				return added, err
			}
		}
		added++
	}
	return added, nil
}

// qosFillTermFromRule maps one ACL rule back onto a classifier term.
func qosFillTermFromRule(inParams XfmrParams, clsObj *ocbinds.OpenconfigQos_Qos_Classifiers_Classifier,
	termId string, family string, rule db.Value) error {

	if clsObj.Terms == nil {
		clsObj.Terms = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms{}
	}
	termObj, ok := clsObj.Terms.Term[termId]
	if !ok {
		var err error
		termObj, err = clsObj.Terms.NewTerm(termId)
		if err != nil {
			return err
		}
	}
	ygot.BuildEmptyTree(termObj)

	idCopy := termId
	termObj.Id = &idCopy
	if termObj.Config == nil {
		termObj.Config = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Config{}
	}
	termObj.Config.Id = &idCopy
	if termObj.State == nil {
		termObj.State = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_State{}
	}
	termObj.State.Id = &idCopy

	/*
	 * When the request targets a node below the term, translib hands over a
	 * partially built tree, so the condition containers cannot be assumed to
	 * exist. Allocate each one on first use instead of trusting BuildEmptyTree.
	 */
	if termObj.Conditions == nil {
		termObj.Conditions = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions{}
	}
	c := termObj.Conditions

	ipv4Cfg := func() *ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_Ipv4_Config {
		if c.Ipv4 == nil {
			c.Ipv4 = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_Ipv4{}
		}
		if c.Ipv4.Config == nil {
			c.Ipv4.Config = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_Ipv4_Config{}
		}
		return c.Ipv4.Config
	}
	ipv6Cfg := func() *ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_Ipv6_Config {
		if c.Ipv6 == nil {
			c.Ipv6 = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_Ipv6{}
		}
		if c.Ipv6.Config == nil {
			c.Ipv6.Config = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_Ipv6_Config{}
		}
		return c.Ipv6.Config
	}
	transportCfg := func() *ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_Transport_Config {
		if c.Transport == nil {
			c.Transport = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_Transport{}
		}
		if c.Transport.Config == nil {
			c.Transport.Config = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_Transport_Config{}
		}
		return c.Transport.Config
	}

	if v, ok := rule.Field["IP_PROTOCOL"]; ok {
		if n, err := strconv.ParseUint(v, 10, 8); err == nil {
			if family == "ipv6" {
				ipv6Cfg().Protocol = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_Ipv6_Config_Protocol_Union_Uint8{Uint8: uint8(n)}
			} else {
				ipv4Cfg().Protocol = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_Ipv4_Config_Protocol_Union_Uint8{Uint8: uint8(n)}
			}
		}
	}
	if v, ok := rule.Field["SRC_IP"]; ok {
		addr := v
		ipv4Cfg().SourceAddress = &addr
	}
	if v, ok := rule.Field["DST_IP"]; ok {
		addr := v
		ipv4Cfg().DestinationAddress = &addr
	}
	if v, ok := rule.Field["SRC_IPV6"]; ok {
		addr := v
		ipv6Cfg().SourceAddress = &addr
	}
	if v, ok := rule.Field["DST_IPV6"]; ok {
		addr := v
		ipv6Cfg().DestinationAddress = &addr
	}
	if v, ok := rule.Field["DSCP"]; ok {
		if n, err := strconv.ParseUint(v, 10, 8); err == nil {
			dscp := uint8(n)
			if family == "ipv6" {
				ipv6Cfg().Dscp = &dscp
			} else {
				ipv4Cfg().Dscp = &dscp
			}
		}
	}
	if v, ok := rule.Field["L4_SRC_PORT"]; ok {
		if n, err := strconv.ParseUint(v, 10, 16); err == nil {
			transportCfg().SourcePort = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_Transport_Config_SourcePort_Union_Uint16{Uint16: uint16(n)}
		}
	}
	if v, ok := rule.Field["L4_DST_PORT"]; ok {
		if n, err := strconv.ParseUint(v, 10, 16); err == nil {
			transportCfg().DestinationPort = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_Transport_Config_DestinationPort_Union_Uint16{Uint16: uint16(n)}
		}
	}
	if v, ok := rule.Field["L4_SRC_PORT_RANGE"]; ok {
		r := strings.Replace(v, "-", "..", 1)
		transportCfg().SourcePort = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_Transport_Config_SourcePort_Union_String{String: r}
	}
	if v, ok := rule.Field["L4_DST_PORT_RANGE"]; ok {
		r := strings.Replace(v, "-", "..", 1)
		transportCfg().DestinationPort = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_Transport_Config_DestinationPort_Union_String{String: r}
	}
	if v, ok := rule.Field["TCP_FLAGS"]; ok {
		transportCfg().TcpFlags = qosTcpFlagsFromDb(v)
	}
	if v, ok := rule.Field["ETHER_TYPE"]; ok {
		if ethertype := qosEthertypeFromDb(v); ethertype != nil {
			if c.L2 == nil {
				c.L2 = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_L2{}
			}
			if c.L2.Config == nil {
				c.L2.Config = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_L2_Config{}
			}
			c.L2.Config.Ethertype = ethertype
		}
	}

	actions := func() *ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Actions {
		if termObj.Actions == nil {
			termObj.Actions = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Actions{}
		}
		return termObj.Actions
	}

	/*
	 * target-group is a leafref to a forwarding group, so it can only be
	 * reported once one carries this traffic class. Reporting the bare number
	 * instead would name a group that does not exist, which is worse than
	 * leaving the action out until the forwarding group is configured.
	 */
	if v, ok := rule.Field["SET_TC"]; ok {
		if fg := get_fwd_group_from_priority(inParams, v); fg != "" {
			a := actions()
			if a.Config == nil {
				a.Config = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Actions_Config{}
			}
			if a.State == nil {
				a.State = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Actions_State{}
			}
			a.Config.TargetGroup = &fg
			a.State.TargetGroup = &fg
		} else {
			log.V(lvl.DEBUG).Infof("qosFillTermFromRule: no forwarding group with priority %v", v)
		}
	}

	return nil
}

// qosEthertypeFromDb turns the ACL representation of an ethertype back into the
// OpenConfig union, which is either a named type or a raw hex value.
func qosEthertypeFromDb(val string) ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_L2_Config_Ethertype_Union {
	if strings.HasPrefix(strings.ToLower(val), "0x") {
		n, err := strconv.ParseUint(val[2:], 16, 16)
		if err != nil {
			return nil
		}
		return &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_L2_Config_Ethertype_Union_Uint16{
			Uint16: uint16(n),
		}
	}

	defs := ocbinds.E_OpenconfigPacketMatchTypes_ETHERTYPE(0).ΛMap()["E_OpenconfigPacketMatchTypes_ETHERTYPE"]
	for value, def := range defs {
		if def.Name == "ETHERTYPE_"+val {
			return &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_L2_Config_Ethertype_Union_E_OpenconfigPacketMatchTypes_ETHERTYPE{
				E_OpenconfigPacketMatchTypes_ETHERTYPE: ocbinds.E_OpenconfigPacketMatchTypes_ETHERTYPE(value),
			}
		}
	}
	return nil
}

// qosTcpFlagsFromDb turns the "0xNN/0xNN" ACL representation back into the
// OpenConfig flag list.
func qosTcpFlagsFromDb(val string) []ocbinds.E_OpenconfigPacketMatchTypes_TCP_FLAGS {
	flags := []ocbinds.E_OpenconfigPacketMatchTypes_TCP_FLAGS{}

	mask := strings.Split(val, "/")[0]
	n, err := strconv.ParseUint(strings.TrimPrefix(mask, "0x"), 16, 32)
	if err != nil {
		return flags
	}
	// Iterate the map's inverse in a fixed order so the list is stable.
	order := []ocbinds.E_OpenconfigPacketMatchTypes_TCP_FLAGS{
		ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_FIN,
		ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_SYN,
		ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_RST,
		ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_PSH,
		ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_ACK,
		ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_URG,
		ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_ECE,
		ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_CWR,
	}
	for _, f := range order {
		if uint32(n)&qosTcpFlagBit[f] != 0 {
			flags = append(flags, f)
		}
	}
	return flags
}

// qosClassifierAclDelete tears down the ACL objects backing a policed
// classifier. The table type is left alone because every classifier of the
// same family shares it.
func qosClassifierAclDelete(inParams XfmrParams, name string, termId string) (map[string]map[string]db.Value, error) {
	resMap := make(map[string]map[string]db.Value)
	empty := func() db.Value { return db.Value{Field: make(map[string]string)} }

	targetUriPath, err := getYangPathFromUri(inParams.uri)
	if err != nil {
		return resMap, err
	}
	const termsPath = "/openconfig-qos:qos/classifiers/classifier/terms"

	/*
	 * Mapping a single condition leaf back to the rule fields it produced is not
	 * implemented. Refusing is the safe answer, because the alternative the
	 * caller falls into is deleting the whole rule or the whole classifier.
	 */
	if strings.HasPrefix(targetUriPath, termsPath+"/term/") {
		return resMap, tlerr.NotSupportedError{
			Format: "Delete below a term of policed classifier " + name +
				" is not supported; delete the term instead",
		}
	}

	// One term is one rule, so a term delete leaves the rest of the classifier
	// standing.
	if termId != "" {
		qosMergeTable(resMap, qosAclRuleTbl,
			map[string]db.Value{name + "|" + qosAclRuleName(termId): empty()})
		log.V(lvl.DEBUG).Infof("qosClassifierAclDelete: classifier %v term %v", name, termId)
		return resMap, nil
	}

	for _, ruleKey := range qosAclRuleKeys(inParams, name) {
		qosMergeTable(resMap, qosAclRuleTbl, map[string]db.Value{ruleKey: empty()})
	}

	// Emptying the term list keeps the classifier itself, which still has a
	// name, a type and possibly an interface bound to it.
	if targetUriPath == termsPath {
		log.V(lvl.DEBUG).Infof("qosClassifierAclDelete: classifier %v all terms", name)
		return resMap, nil
	}

	qosMergeTable(resMap, qosAclTableTbl, map[string]db.Value{name: empty()})
	qosMergeTable(resMap, qosUmfClassifier, map[string]db.Value{name: empty()})

	// The policer belongs to the scheduler policy, not to the classifier, so
	// leave it alone while another classifier is still metered by it.
	if policer := qosPolicerForClassifier(inParams, name); policer != "" {
		others := false
		for _, cls := range qosPolicerClassifierRefs(inParams.d)[policer] {
			if cls != name {
				others = true
				break
			}
		}
		if !others {
			qosMergeTable(resMap, qosPolicerTbl, map[string]db.Value{policer: empty()})
		}
	}

	log.V(lvl.DEBUG).Infof("qosClassifierAclDelete: classifier %v", name)
	return resMap, nil
}

// qosSchedPolicyPolicerDelete removes the policers realizing a policing policy
// and detaches them from the classifier rules pointing at them. The classifier
// itself is left in place: it is a separate object with its own lifetime.
func qosSchedPolicyPolicerDelete(inParams XfmrParams, spName string, seq string) (map[string]map[string]db.Value, error) {
	resMap := make(map[string]map[string]db.Value)

	refs := qosPolicerClassifierRefs(inParams.d)
	for _, policer := range qosPolicerKeysForPolicyInDb(inParams.d, spName, seq) {
		// The POLICER table is shared with control-plane policing, whose entries
		// no classifier points at. Matching the "<policy>_<sequence>" name is
		// not enough to claim ownership of one.
		classifiers := refs[policer]
		if len(classifiers) == 0 {
			continue
		}

		qosMergeTable(resMap, qosPolicerTbl,
			map[string]db.Value{policer: {Field: make(map[string]string)}})

		// A rule keeps matching once the policer is gone, so drop just the
		// action field rather than the rule.
		for _, cls := range classifiers {
			for _, ruleKey := range qosAclRuleKeys(inParams, cls) {
				qosMergeTable(resMap, qosAclRuleTbl,
					map[string]db.Value{ruleKey: {Field: map[string]string{"POLICER_ACTION": ""}}})
			}
		}
	}

	log.V(lvl.DEBUG).Infof("qosSchedPolicyPolicerDelete: policy %v seq %q", spName, seq)
	return resMap, nil
}

// qosClassifiersForPolicy lists the classifiers policed by a scheduler-policy,
// preferring the request tree and falling back to what is already in CONFIG_DB.
func qosClassifiersForPolicy(inParams XfmrParams, spName string) []string {
	seen := map[string]bool{}
	names := []string{}

	qosObj := getQosRoot(inParams.ygRoot)
	if qosObj != nil && qosObj.SchedulerPolicies != nil {
		if sp, ok := qosObj.SchedulerPolicies.SchedulerPolicy[spName]; ok && sp.Schedulers != nil {
			for _, sched := range sp.Schedulers.Scheduler {
				if cls := qosSchedulerPolicerClassifier(inParams, sched); cls != "" && !seen[cls] {
					seen[cls] = true
					names = append(names, cls)
				}
			}
		}
	}

	if inParams.d == nil {
		return names
	}
	// A rule pointing at one of the policy's policers identifies its classifier.
	keys, err := inParams.d.GetKeysByPattern(&db.TableSpec{Name: qosAclRuleTbl}, "*")
	if err != nil {
		return names
	}
	for _, key := range keys {
		if len(key.Comp) < 2 || seen[key.Comp[0]] {
			continue
		}
		entry, err := inParams.d.GetEntry(&db.TableSpec{Name: qosAclRuleTbl}, key)
		if err != nil {
			continue
		}
		if qosPolicyOfPolicer(entry.Field["POLICER_ACTION"]) != spName {
			continue
		}
		seen[key.Comp[0]] = true
		names = append(names, key.Comp[0])
	}
	return names
}

// qosPolicyBoundToIntf reports which scheduler-policy an interface currently
// binds on ingress, which a delete needs since the request carries no value.
func qosPolicyBoundToIntf(inParams XfmrParams, ifName string) string {
	if inParams.d == nil {
		return ""
	}
	// The binding lives as the interface being in the backing ACL table's ports,
	// so find that table and walk back to the policy through its policer.
	keys, err := inParams.d.GetKeysByPattern(&db.TableSpec{Name: qosAclTableTbl}, "*")
	if err != nil {
		return ""
	}
	for _, key := range keys {
		if len(key.Comp) == 0 {
			continue
		}
		entry, err := inParams.d.GetEntry(&db.TableSpec{Name: qosAclTableTbl}, key)
		if err != nil {
			continue
		}
		if t := entry.Field["type"]; t != qosAclTableTypeV4 && t != qosAclTableTypeV6 {
			continue
		}
		for _, p := range strings.Split(entry.Field["ports@"], ",") {
			if p != ifName {
				continue
			}
			if sp := qosPolicyOfPolicer(qosPolicerForClassifier(inParams, key.Comp[0])); sp != "" {
				return sp
			}
		}
	}
	return ""
}

// YangToDb_qos_intf_input_sched_policy_xfmr binds or unbinds an ingress
// policing scheduler-policy on an interface. SONiC has no per interface policer
// binding, so the interface is added to the ports of the ACL table backing each
// classifier the policy polices.
var YangToDb_qos_intf_input_sched_policy_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	resMap := make(map[string]map[string]db.Value)

	ifName := NewPathInfo(inParams.uri).Var("interface-id")
	if ifName == "" {
		return resMap, nil
	}

	var spName string
	if inParams.oper == DELETE {
		spName = qosPolicyBoundToIntf(inParams, ifName)
	} else {
		qosIntfsObj := getQosIntfRoot(inParams.ygRoot)
		if qosIntfsObj == nil {
			return resMap, nil
		}
		intfObj, ok := qosIntfsObj.Interface[ifName]
		if !ok || intfObj.Input == nil || intfObj.Input.SchedulerPolicy == nil ||
			intfObj.Input.SchedulerPolicy.Config == nil ||
			intfObj.Input.SchedulerPolicy.Config.Name == nil {
			return resMap, nil
		}
		spName = *intfObj.Input.SchedulerPolicy.Config.Name
	}

	if spName == "" {
		return resMap, nil
	}

	classifiers := qosClassifiersForPolicy(inParams, spName)
	if len(classifiers) == 0 {
		return resMap, tlerr.NotFoundError{
			Format: "Scheduler policy " + spName + " does not police any classifier",
		}
	}

	add := inParams.oper != DELETE

	if add {
		/*
		 * The leaf holds one policy, so binding a new one unbinds whatever was
		 * there. Without this the interface stays in both port lists and its
		 * traffic is metered twice.
		 */
		if prev := qosPolicyBoundToIntf(inParams, ifName); prev != "" && prev != spName {
			qosUnbindIntfFromPolicy(inParams, prev, ifName)
		}
	}

	/*
	 * Name only the interface, never the whole list. Translib merges a leaf-list
	 * on update, so the interface is appended, and on delete it removes the
	 * members named here and leaves any other interface bound. Passing the
	 * remaining list would instead unbind the interfaces meant to stay.
	 */
	for _, cls := range classifiers {
		qosMergeTable(resMap, qosAclTableTbl,
			map[string]db.Value{cls: {Field: map[string]string{"ports@": ifName}}})
	}

	log.V(lvl.DEBUG).Infof("qos_intf_input_sched_policy: intf %v policy %v classifiers %v add %v",
		ifName, spName, classifiers, add)
	return resMap, nil
}

// qosUnbindIntfFromPolicy stages removal of an interface from the tables backing
// a policy's classifiers. It rides along as a DELETE sub-operation, which
// translib applies before the rest of the request, so a rebind in the same
// request lands after this.
func qosUnbindIntfFromPolicy(inParams XfmrParams, spName string, ifName string) {
	for _, cls := range qosClassifiersForPolicy(inParams, spName) {
		qosStageDelete(inParams, qosAclTableTbl, cls,
			db.Value{Field: map[string]string{"ports@": ifName}})
	}
}

// DbToYang_qos_intf_input_sched_policy_xfmr reports the ingress scheduler-policy
// bound to an interface.
var DbToYang_qos_intf_input_sched_policy_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	ifName := NewPathInfo(inParams.uri).Var("interface-id")
	if ifName == "" {
		return nil
	}

	spName := qosPolicyBoundToIntf(inParams, ifName)
	if spName == "" {
		return nil
	}

	qosIntfsObj := getQosIntfRoot(inParams.ygRoot)
	if qosIntfsObj == nil {
		return nil
	}
	intfObj, ok := qosIntfsObj.Interface[ifName]
	if !ok {
		intfObj, _ = qosIntfsObj.NewInterface(ifName)
	}
	ygot.BuildEmptyTree(intfObj)
	if intfObj.Input == nil {
		intfObj.Input = &ocbinds.OpenconfigQos_Qos_Interfaces_Interface_Input{}
	}
	if intfObj.Input.SchedulerPolicy == nil {
		intfObj.Input.SchedulerPolicy = &ocbinds.OpenconfigQos_Qos_Interfaces_Interface_Input_SchedulerPolicy{}
	}
	ygot.BuildEmptyTree(intfObj.Input.SchedulerPolicy)
	if intfObj.Input.SchedulerPolicy.Config == nil {
		intfObj.Input.SchedulerPolicy.Config = &ocbinds.OpenconfigQos_Qos_Interfaces_Interface_Input_SchedulerPolicy_Config{}
	}
	if intfObj.Input.SchedulerPolicy.State == nil {
		intfObj.Input.SchedulerPolicy.State = &ocbinds.OpenconfigQos_Qos_Interfaces_Interface_Input_SchedulerPolicy_State{}
	}
	intfObj.Input.SchedulerPolicy.Config.Name = &spName
	intfObj.Input.SchedulerPolicy.State.Name = &spName

	return nil
}

// qosAclRuleKeys lists the ACL_RULE keys backing a classifier, combining the
// rules already in CONFIG_DB with those the current request is creating.
func qosAclRuleKeys(inParams XfmrParams, clsName string) []string {
	seen := map[string]bool{}
	keys := []string{}

	if inParams.d != nil {
		dbKeys, err := inParams.d.GetKeysByPattern(&db.TableSpec{Name: qosAclRuleTbl}, clsName+"|*")
		if err == nil {
			for _, k := range dbKeys {
				if len(k.Comp) < 2 {
					continue
				}
				key := k.Comp[0] + "|" + k.Comp[1]
				if !seen[key] {
					seen[key] = true
					keys = append(keys, key)
				}
			}
		}
	}

	qosObj := getQosRoot(inParams.ygRoot)
	if qosObj != nil && qosObj.Classifiers != nil {
		if cls, ok := qosObj.Classifiers.Classifier[clsName]; ok && cls.Terms != nil {
			for termId, term := range cls.Terms.Term {
				if !qosTermIsAclRule(term) {
					continue
				}
				key := clsName + "|" + qosAclRuleName(termId)
				if !seen[key] {
					seen[key] = true
					keys = append(keys, key)
				}
			}
		}
	}
	return keys
}

// qosSeqOfPolicer recovers the scheduler sequence encoded in a policer name.
func qosSeqOfPolicer(policer string) (uint32, bool) {
	i := strings.LastIndex(policer, "_")
	if i <= 0 {
		return 0, false
	}
	seq, err := strconv.ParseUint(policer[i+1:], 10, 32)
	if err != nil {
		return 0, false
	}
	return uint32(seq), true
}

// qosPolicerKeysForPolicyInDb lists the POLICER entries a scheduler policy owns.
// It takes a raw DB handle so that the subscribe callback, which is not given
// XfmrParams, can use it too. An empty seq matches every sequence.
func qosPolicerKeysForPolicyInDb(d *db.DB, spName string, seq string) []string {
	names := []string{}
	if d == nil || spName == "" {
		return names
	}
	ts := &db.TableSpec{Name: qosPolicerTbl}

	if seq != "" {
		name := spName + "_" + seq
		if _, err := d.GetEntry(ts, db.Key{Comp: []string{name}}); err == nil {
			names = append(names, name)
		}
		return names
	}

	keys, err := d.GetKeysByPattern(ts, spName+"_*")
	if err != nil {
		return names
	}
	for _, key := range keys {
		// The pattern over-matches: for policy "POL" the name "POL_1_0" also
		// matches, so confirm the name inverts back to this policy.
		if len(key.Comp) > 0 && qosPolicyOfPolicer(key.Comp[0]) == spName {
			names = append(names, key.Comp[0])
		}
	}
	return names
}

// qosPolicerClassifierMap maps each policer referenced by a classifier's rules
// to that classifier. It doubles as the set of policers that belong to QoS at
// all: the POLICER table is shared with control-plane policing, whose entries
// no classifier points at.
func qosPolicerClassifierMap(d *db.DB) map[string]string {
	first := map[string]string{}
	for policer, refs := range qosPolicerClassifierRefs(d) {
		if len(refs) > 0 {
			first[policer] = refs[0]
		}
	}
	return first
}

// qosPolicerClassifierRefs maps each policer to every classifier whose rules
// reference it, which is what tells this feature's policers apart from the
// control-plane ones sharing the table, and says whether a policer is still
// wanted once one of its classifiers goes away.
func qosPolicerClassifierRefs(d *db.DB) map[string][]string {
	refs := map[string][]string{}
	if d == nil {
		return refs
	}
	keys, err := d.GetKeysByPattern(&db.TableSpec{Name: qosAclRuleTbl}, "*")
	if err != nil {
		return refs
	}
	for _, key := range keys {
		if len(key.Comp) < 2 {
			continue
		}
		entry, err := d.GetEntry(&db.TableSpec{Name: qosAclRuleTbl}, key)
		if err != nil {
			continue
		}
		if policer := entry.Field["POLICER_ACTION"]; policer != "" {
			refs[policer] = qosAppendUnique(refs[policer], key.Comp[0])
		}
	}
	return refs
}

// qosClassifierForPolicerInDb reports the classifier whose rules a policer
// meters, which is the scheduler's input.
func qosClassifierForPolicerInDb(d *db.DB, policer string) string {
	if policer == "" {
		return ""
	}
	return qosPolicerClassifierMap(d)[policer]
}

// qosRateFromDb converts a byte mode policer rate back to the bits per second
// that OpenConfig states.
func qosRateFromDb(val string) (uint64, bool) {
	if val == "" {
		return 0, false
	}
	rate, err := strconv.ParseUint(val, 10, 64)
	if err != nil {
		return 0, false
	}
	return rate * 8, true
}

func qosBurstFromDb(val string) (uint32, bool) {
	if val == "" {
		return 0, false
	}
	burst, err := strconv.ParseUint(val, 10, 32)
	if err != nil {
		return 0, false
	}
	return uint32(burst), true
}

// qosPolicerReadFlags selects which containers a scheduler policy read fills,
// mirroring the target the request asked for.
type qosPolicerReadFlags struct {
	policyConfig, policyState   bool
	schedConfig, schedState     bool
	oneRateConfig, oneRateState bool
	twoRateConfig, twoRateState bool
	inputConfig, inputState     bool
}

// qosFillSchedPoliciesFromPolicer rebuilds the scheduler policy view of the
// POLICER entries backing policing policies, and reports how many policies it
// added. A policing policy has no SCHEDULER entry, so without this a read
// reports it missing even though the policer is programmed.
func qosFillSchedPoliciesFromPolicer(inParams XfmrParams,
	policies *ocbinds.OpenconfigQos_Qos_SchedulerPolicies, spName string, seq string,
	f qosPolicerReadFlags) (int, error) {

	if policies == nil || inParams.d == nil {
		return 0, nil
	}

	inUse := qosPolicerClassifierMap(inParams.d)

	names := []string{}
	if spName != "" {
		// A named policy is reported even if no classifier points at its
		// policer any more, so that an orphaned policer stays visible and
		// therefore deletable.
		names = append(names, spName)
	} else {
		// Listing every policy has no name to go on, so consider only the
		// policers a classifier actually points at. The POLICER table is shared
		// with control-plane policing, and one of those named "<word>_<digits>"
		// would otherwise be mistaken for a scheduler policy.
		seen := map[string]bool{}
		for policer := range inUse {
			if policy := qosPolicyOfPolicer(policer); policy != "" && !seen[policy] {
				seen[policy] = true
				names = append(names, policy)
			}
		}
	}

	added := 0
	for _, name := range names {
		policers := qosPolicerKeysForPolicyInDb(inParams.d, name, seq)
		if len(policers) == 0 {
			continue
		}

		spObj, ok := policies.SchedulerPolicy[name]
		if !ok {
			var err error
			spObj, err = policies.NewSchedulerPolicy(name)
			if err != nil {
				return added, err
			}
		}
		ygot.BuildEmptyTree(spObj)

		nameCopy := name
		spObj.Name = &nameCopy
		if f.policyConfig {
			if spObj.Config == nil {
				spObj.Config = &ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Config{}
			}
			spObj.Config.Name = &nameCopy
		}
		if f.policyState {
			if spObj.State == nil {
				spObj.State = &ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_State{}
			}
			spObj.State.Name = &nameCopy
		}

		for _, policer := range policers {
			if err := qosFillSchedulerFromPolicer(inParams, spObj, policer, inUse, f); err != nil {
				return added, err
			}
		}
		added++
	}
	return added, nil
}

// qosFillSchedulerFromPolicer rebuilds one scheduler of a policing policy from
// the POLICER entry realizing it.
func qosFillSchedulerFromPolicer(inParams XfmrParams,
	spObj *ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy, policer string,
	inUse map[string]string, f qosPolicerReadFlags) error {

	seq, ok := qosSeqOfPolicer(policer)
	if !ok {
		return nil
	}
	entry, err := inParams.d.GetEntry(&db.TableSpec{Name: qosPolicerTbl}, db.Key{Comp: []string{policer}})
	if err != nil {
		return nil
	}

	// A request below the policy yields a partially built tree, so the
	// containers touched here are allocated rather than assumed.
	if spObj.Schedulers == nil {
		spObj.Schedulers = &ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers{}
	}
	schedObj, ok := spObj.Schedulers.Scheduler[seq]
	if !ok {
		schedObj, err = spObj.Schedulers.NewScheduler(seq)
		if err != nil {
			return err
		}
	}
	ygot.BuildEmptyTree(schedObj)

	twoRate := entry.Field["mode"] == "tr_tcm"
	schedType := ocbinds.OpenconfigQosTypes_QOS_SCHEDULER_TYPE_ONE_RATE_TWO_COLOR
	if twoRate {
		schedType = ocbinds.OpenconfigQosTypes_QOS_SCHEDULER_TYPE_TWO_RATE_THREE_COLOR
	}

	seqCopy := seq
	schedObj.Sequence = &seqCopy
	if f.schedConfig {
		if schedObj.Config == nil {
			schedObj.Config = &ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler_Config{}
		}
		schedObj.Config.Sequence = &seqCopy
		schedObj.Config.Type = schedType
	}
	if f.schedState {
		if schedObj.State == nil {
			schedObj.State = &ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler_State{}
		}
		schedObj.State.Sequence = &seqCopy
		schedObj.State.Type = schedType
	}

	if f.inputConfig || f.inputState {
		if cls := inUse[policer]; cls != "" {
			if schedObj.Inputs == nil {
				schedObj.Inputs = &ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler_Inputs{}
			}
			inObj, ok := schedObj.Inputs.Input[cls]
			if !ok {
				inObj, err = schedObj.Inputs.NewInput(cls)
				if err != nil {
					return err
				}
			}
			ygot.BuildEmptyTree(inObj)
			clsCopy := cls
			inObj.Id = &clsCopy
			if f.inputConfig {
				if inObj.Config == nil {
					inObj.Config = &ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler_Inputs_Input_Config{}
				}
				inObj.Config.Id = &clsCopy
				inObj.Config.InputType = ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler_Inputs_Input_Config_InputType_IN_PROFILE
			}
			if f.inputState {
				if inObj.State == nil {
					inObj.State = &ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler_Inputs_Input_State{}
				}
				inObj.State.Id = &clsCopy
				inObj.State.InputType = ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler_Inputs_Input_Config_InputType_IN_PROFILE
			}
		}
	}

	cir, haveCir := qosRateFromDb(entry.Field["cir"])
	pir, havePir := qosRateFromDb(entry.Field["pir"])
	cbs, haveCbs := qosBurstFromDb(entry.Field["cbs"])
	pbs, havePbs := qosBurstFromDb(entry.Field["pbs"])

	if twoRate {
		// Exceeding the committed rate is yellow; exceeding the peak rate is red.
		drop := entry.Field["yellow_packet_action"] == "drop"
		if schedObj.TwoRateThreeColor == nil {
			schedObj.TwoRateThreeColor = &ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler_TwoRateThreeColor{}
		}
		trc := schedObj.TwoRateThreeColor
		if f.twoRateConfig {
			if trc.Config == nil {
				trc.Config = &ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler_TwoRateThreeColor_Config{}
			}
			if haveCir {
				trc.Config.Cir = &cir
			}
			if havePir {
				trc.Config.Pir = &pir
			}
			if haveCbs {
				trc.Config.Bc = &cbs
			}
			if havePbs {
				trc.Config.Be = &pbs
			}
			if drop {
				if trc.ExceedAction == nil {
					trc.ExceedAction = &ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler_TwoRateThreeColor_ExceedAction{}
				}
				if trc.ExceedAction.Config == nil {
					trc.ExceedAction.Config = &ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler_TwoRateThreeColor_ExceedAction_Config{}
				}
				dropCopy := drop
				trc.ExceedAction.Config.Drop = &dropCopy
			}
		}
		if f.twoRateState {
			if trc.State == nil {
				trc.State = &ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler_TwoRateThreeColor_State{}
			}
			if haveCir {
				trc.State.Cir = &cir
			}
			if havePir {
				trc.State.Pir = &pir
			}
			if haveCbs {
				trc.State.Bc = &cbs
			}
			if havePbs {
				trc.State.Be = &pbs
			}
		}
		return nil
	}

	drop := entry.Field["red_packet_action"] == "drop"
	if schedObj.OneRateTwoColor == nil {
		schedObj.OneRateTwoColor = &ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler_OneRateTwoColor{}
	}
	orc := schedObj.OneRateTwoColor
	if f.oneRateConfig {
		if orc.Config == nil {
			orc.Config = &ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler_OneRateTwoColor_Config{}
		}
		if haveCir {
			orc.Config.Cir = &cir
		}
		if haveCbs {
			orc.Config.Bc = &cbs
		}
		if drop {
			if orc.ExceedAction == nil {
				orc.ExceedAction = &ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler_OneRateTwoColor_ExceedAction{}
			}
			if orc.ExceedAction.Config == nil {
				orc.ExceedAction.Config = &ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler_OneRateTwoColor_ExceedAction_Config{}
			}
			dropCopy := drop
			orc.ExceedAction.Config.Drop = &dropCopy
		}
	}
	if f.oneRateState {
		if orc.State == nil {
			orc.State = &ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler_OneRateTwoColor_State{}
		}
		if haveCir {
			orc.State.Cir = &cir
		}
		if haveCbs {
			orc.State.Bc = &cbs
		}
	}
	return nil
}
