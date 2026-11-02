////////////////////////////////////////////////////////////////////////////////
//                                                                            //
//  Copyright 2019 Broadcom. The term Broadcom refers to Broadcom Inc. and/or //
//  its subsidiaries.                                                         //
//                                                                            //
//  Licensed under the Apache License, Version 2.0 (the "License");           //
//  you may not use this file except in compliance with the License.          //
//  You may obtain a copy of the License at                                   //
//                                                                            //
//     http://www.apache.org/licenses/LICENSE-2.0                             //
//                                                                            //
//  Unless required by applicable law or agreed to in writing, software       //
//  distributed under the License is distributed on an "AS IS" BASIS,         //
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.  //
//  See the License for the specific language governing permissions and       //
//  limitations under the License.                                            //
//                                                                            //
////////////////////////////////////////////////////////////////////////////////

package translib

import (
	"bytes"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/path"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"

	log "github.com/golang/glog"
	"github.com/openconfig/gnmi/proto/gnmi"
	"github.com/openconfig/ygot/util"
	"github.com/openconfig/ygot/ygot"
)

const (
	TABLE_SEPARATOR          = "|"
	KEY_SEPARATOR            = "|"
	ACL_TABLE                = "ACL_TABLE"
	RULE_TABLE               = "ACL_RULE"
	ACL_TYPE                 = "type"
	ACL_DESCRIPTION          = "policy_desc"
	ACL_OPENCONFIG_SET_NAME  = "oc_set_name"
	ACL_SERVICES             = "services"
	SONIC_ACL_TYPE_L2        = "L2"
	SONIC_ACL_TYPE_IPV4      = "L3"
	SONIC_ACL_TYPE_IPV6      = "L3V6"
	SONIC_ACL_TYPE_CTRLPLANE = "CTRLPLANE"
	OPENCONFIG_ACL_TYPE_IPV4 = "ACL_IPV4"
	OPENCONFIG_ACL_TYPE_IPV6 = "ACL_IPV6"
	OPENCONFIG_ACL_TYPE_L2   = "ACL_L2"
	OC_ACL_APP_MODULE_NAME   = "/openconfig-acl:acl"
	OC_ACL_YANG_PATH_PREFIX  = "/device/acl"

	// Rule counters are published by the flex counter service in COUNTERS_DB.
	// ACL_COUNTER_RULE_MAP maps "<acl table>:<rule>" to the counter object id,
	// whose packet and byte counts live in the COUNTERS table.
	ACL_COUNTER_RULE_MAP     = "ACL_COUNTER_RULE_MAP"
	COUNTERS_TABLE           = "COUNTERS"
	ACL_COUNTER_ATTR_PACKETS = "SAI_ACL_COUNTER_ATTR_PACKETS"
	ACL_COUNTER_ATTR_BYTES   = "SAI_ACL_COUNTER_ATTR_BYTES"

	MIN_PRIORITY = 1
	MAX_PRIORITY = 65536
)

var IP_PROTOCOL_MAP = map[ocbinds.E_OpenconfigPacketMatchTypes_IP_PROTOCOL]uint8{
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

// ICMPV6_TYPE_MAP maps the standard openconfig-icmpv6-types TYPE identities to
// their numeric ICMPv6 message type (SONiC ACL_RULE ICMPV6_TYPE). The codes of
// a type are in ICMPV6_CODE_MAP below.
var ICMPV6_TYPE_MAP = map[ocbinds.E_OpenconfigIcmpv6Types_TYPE]uint8{
	ocbinds.OpenconfigIcmpv6Types_TYPE_DESTINATION_UNREACHABLE:              1,
	ocbinds.OpenconfigIcmpv6Types_TYPE_PACKET_TOO_BIG:                       2,
	ocbinds.OpenconfigIcmpv6Types_TYPE_TIME_EXCEEDED:                        3,
	ocbinds.OpenconfigIcmpv6Types_TYPE_PARAMETER_PROBLEM:                    4,
	ocbinds.OpenconfigIcmpv6Types_TYPE_ECHO_REQUEST:                         128,
	ocbinds.OpenconfigIcmpv6Types_TYPE_ECHO_REPLY:                           129,
	ocbinds.OpenconfigIcmpv6Types_TYPE_MULTICAST_LISTENER_QUERY:             130,
	ocbinds.OpenconfigIcmpv6Types_TYPE_MULTICAST_LISTENER_REPORT:            131,
	ocbinds.OpenconfigIcmpv6Types_TYPE_MULTICAST_LISTENER_DONE:              132,
	ocbinds.OpenconfigIcmpv6Types_TYPE_ROUTER_SOLICITATION:                  133,
	ocbinds.OpenconfigIcmpv6Types_TYPE_ROUTER_ADVERTISEMENT:                 134,
	ocbinds.OpenconfigIcmpv6Types_TYPE_NEIGHBOR_SOLICITATION:                135,
	ocbinds.OpenconfigIcmpv6Types_TYPE_NEIGHBOR_ADVERTISEMENT:               136,
	ocbinds.OpenconfigIcmpv6Types_TYPE_REDIRECT:                             137,
	ocbinds.OpenconfigIcmpv6Types_TYPE_RENUNBERING:                          138,
	ocbinds.OpenconfigIcmpv6Types_TYPE_NODE_INFORMATION_QUERY:               139,
	ocbinds.OpenconfigIcmpv6Types_TYPE_NODE_INFORMATION_RESPONSE:            140,
	ocbinds.OpenconfigIcmpv6Types_TYPE_INVERSE_NEIGHBOR_SOLICITATION:        141,
	ocbinds.OpenconfigIcmpv6Types_TYPE_INVERSE_NEIGHBOR_ADVERTISEMENT:       142,
	ocbinds.OpenconfigIcmpv6Types_TYPE_VERSION2_MULTICAST_LISTENER:          143,
	ocbinds.OpenconfigIcmpv6Types_TYPE_HOME_AGENT_ADDRESS_DISCOVERY_REQUEST: 144,
	ocbinds.OpenconfigIcmpv6Types_TYPE_HOME_AGENT_ADDRESS_DISCOVERY_REPLY:   145,
	ocbinds.OpenconfigIcmpv6Types_TYPE_MOBILE_PREFIX_SOLICITATION:           146,
	ocbinds.OpenconfigIcmpv6Types_TYPE_MOBILE_PREFIX_ADVERTISEMENT:          147,
	ocbinds.OpenconfigIcmpv6Types_TYPE_CERTIFICATION_PATH_SOLICITATION:      148,
	ocbinds.OpenconfigIcmpv6Types_TYPE_CERTIFICATION_PATH_ADVERTISEMENT:     149,
	ocbinds.OpenconfigIcmpv6Types_TYPE_MULTICAST_ROUTER_ADVERTISEMENT:       151,
	ocbinds.OpenconfigIcmpv6Types_TYPE_MULTICAST_ROUTER_SOLICITATION:        152,
	ocbinds.OpenconfigIcmpv6Types_TYPE_MULTICAST_ROUTER_TERMINATION:         153,
	ocbinds.OpenconfigIcmpv6Types_TYPE_FMIPV6:                               154,
	ocbinds.OpenconfigIcmpv6Types_TYPE_RPL_CONTROL:                          155,
	ocbinds.OpenconfigIcmpv6Types_TYPE_ILNPV6_LOCATOR_UPDATE:                156,
	ocbinds.OpenconfigIcmpv6Types_TYPE_DUPLICATE_ADDRESS_REQUEST:            157,
	ocbinds.OpenconfigIcmpv6Types_TYPE_DUPLICATE_ADDRESS_CONFIRMATION:       158,
	ocbinds.OpenconfigIcmpv6Types_TYPE_MPL_CONTROL:                          159,
	ocbinds.OpenconfigIcmpv6Types_TYPE_EXT_ECHO_REQUEST:                     160,
	ocbinds.OpenconfigIcmpv6Types_TYPE_EXT_ECHO_REPLY:                       161,
}

// icmpv6Code is what an openconfig-icmpv6-types CODE identity stands for: the
// number carried in the ICMPv6 header, and the message type it belongs to. An
// ICMPv6 code only has a meaning within its type, so the type is what makes
// the number unambiguous: code 1 reads as "administratively prohibited" under
// DESTINATION_UNREACHABLE but as "fragment reassembly time exceeded" under
// TIME_EXCEEDED.
type icmpv6Code struct {
	num uint8
	typ ocbinds.E_OpenconfigIcmpv6Types_TYPE
}

// ICMPV6_CODE_MAP maps the standard openconfig-icmpv6-types CODE identities to
// their numeric ICMPv6 code (SONiC ACL_RULE ICMPV6_CODE) and owning type.
//
// Only the leaves of the identity tree are here. The per-type identities the
// codes are grouped under have no number of their own; they are in
// ICMPV6_CODE_GROUP_MAP.
var ICMPV6_CODE_MAP = map[ocbinds.E_OpenconfigIcmpv6Types_CODE]icmpv6Code{
	// Type 1, destination unreachable.
	ocbinds.OpenconfigIcmpv6Types_CODE_DST_UNREACHABLE_NO_ROUTE_TO_DST:        {0, ocbinds.OpenconfigIcmpv6Types_TYPE_DESTINATION_UNREACHABLE},
	ocbinds.OpenconfigIcmpv6Types_CODE_DST_UNREACHABLE_DST_ADMIN_PROHIBITED:   {1, ocbinds.OpenconfigIcmpv6Types_TYPE_DESTINATION_UNREACHABLE},
	ocbinds.OpenconfigIcmpv6Types_CODE_DST_UNREACHABLE_BEYOND_SCOPE_OF_SRC:    {2, ocbinds.OpenconfigIcmpv6Types_TYPE_DESTINATION_UNREACHABLE},
	ocbinds.OpenconfigIcmpv6Types_CODE_DST_UNREACHABLE_ADDR:                   {3, ocbinds.OpenconfigIcmpv6Types_TYPE_DESTINATION_UNREACHABLE},
	ocbinds.OpenconfigIcmpv6Types_CODE_DST_UNREACHABLE_PORT:                   {4, ocbinds.OpenconfigIcmpv6Types_TYPE_DESTINATION_UNREACHABLE},
	ocbinds.OpenconfigIcmpv6Types_CODE_DST_UNREACHABLE_SRC_ADDR_FAILED_POLICY: {5, ocbinds.OpenconfigIcmpv6Types_TYPE_DESTINATION_UNREACHABLE},
	ocbinds.OpenconfigIcmpv6Types_CODE_DST_UNREACHABLE_REJECT_ROUTE_TO_DST:    {6, ocbinds.OpenconfigIcmpv6Types_TYPE_DESTINATION_UNREACHABLE},
	ocbinds.OpenconfigIcmpv6Types_CODE_DST_UNREACHABLE_ERR_IN_SRC_ROUTING_HDR: {7, ocbinds.OpenconfigIcmpv6Types_TYPE_DESTINATION_UNREACHABLE},
	ocbinds.OpenconfigIcmpv6Types_CODE_DST_UNREACHABLE_HDRS_TOO_LONG:          {8, ocbinds.OpenconfigIcmpv6Types_TYPE_DESTINATION_UNREACHABLE},

	// Type 2, packet too big. Type 3, time exceeded.
	ocbinds.OpenconfigIcmpv6Types_CODE_PACKET_TOO_BIG_NO_CODE:            {0, ocbinds.OpenconfigIcmpv6Types_TYPE_PACKET_TOO_BIG},
	ocbinds.OpenconfigIcmpv6Types_CODE_TIME_EXCEEDED_HOP_LIMIT:           {0, ocbinds.OpenconfigIcmpv6Types_TYPE_TIME_EXCEEDED},
	ocbinds.OpenconfigIcmpv6Types_CODE_TIME_EXCEEDED_FRAGMENT_REASSEMBLY: {1, ocbinds.OpenconfigIcmpv6Types_TYPE_TIME_EXCEEDED},

	// Type 4, parameter problem.
	ocbinds.OpenconfigIcmpv6Types_CODE_PARAM_PROBLEM_ERR_HDR_FIELD:              {0, ocbinds.OpenconfigIcmpv6Types_TYPE_PARAMETER_PROBLEM},
	ocbinds.OpenconfigIcmpv6Types_CODE_PARAM_PROBLEM_UNRECOGNIZED_NET_HDR_TYPE:  {1, ocbinds.OpenconfigIcmpv6Types_TYPE_PARAMETER_PROBLEM},
	ocbinds.OpenconfigIcmpv6Types_CODE_PARAM_PROBLEM_UNRECOGNIZED_IPV6_OPT:      {2, ocbinds.OpenconfigIcmpv6Types_TYPE_PARAMETER_PROBLEM},
	ocbinds.OpenconfigIcmpv6Types_CODE_PARAM_PROBLEM_INCOMPLETE_HDR_CHAIN:       {3, ocbinds.OpenconfigIcmpv6Types_TYPE_PARAMETER_PROBLEM},
	ocbinds.OpenconfigIcmpv6Types_CODE_PARAM_PROBLEM_SR_UPPER_HDR_ERR:           {4, ocbinds.OpenconfigIcmpv6Types_TYPE_PARAMETER_PROBLEM},
	ocbinds.OpenconfigIcmpv6Types_CODE_PARAM_PROBLEM_UNRECOGNIZED_NEXT_HDR_TYPE: {5, ocbinds.OpenconfigIcmpv6Types_TYPE_PARAMETER_PROBLEM},
	ocbinds.OpenconfigIcmpv6Types_CODE_PARAM_PROBLEM_EXT_HDR_TOO_BIG:            {6, ocbinds.OpenconfigIcmpv6Types_TYPE_PARAMETER_PROBLEM},
	ocbinds.OpenconfigIcmpv6Types_CODE_PARAM_PROBLEM_EXT_HDR_CHAIN_TOO_LONG:     {7, ocbinds.OpenconfigIcmpv6Types_TYPE_PARAMETER_PROBLEM},
	ocbinds.OpenconfigIcmpv6Types_CODE_PARAM_PROBLEM_TOO_MANY_EXT_HDRS:          {8, ocbinds.OpenconfigIcmpv6Types_TYPE_PARAMETER_PROBLEM},
	ocbinds.OpenconfigIcmpv6Types_CODE_PARAM_PROBLEM_TOO_MANY_OPTS:              {9, ocbinds.OpenconfigIcmpv6Types_TYPE_PARAMETER_PROBLEM},
	ocbinds.OpenconfigIcmpv6Types_CODE_PARAM_PROBLEM_OPT_TOO_BIG:                {10, ocbinds.OpenconfigIcmpv6Types_TYPE_PARAMETER_PROBLEM},

	// Types that carry no code, so only the value 0 is defined for them.
	ocbinds.OpenconfigIcmpv6Types_CODE_ECHO_REQUEST_NO_CODE:                         {0, ocbinds.OpenconfigIcmpv6Types_TYPE_ECHO_REQUEST},
	ocbinds.OpenconfigIcmpv6Types_CODE_ECHO_REPLY_NO_CODE:                           {0, ocbinds.OpenconfigIcmpv6Types_TYPE_ECHO_REPLY},
	ocbinds.OpenconfigIcmpv6Types_CODE_MULTICAST_LISTENER_QUERY_NO_CODE:             {0, ocbinds.OpenconfigIcmpv6Types_TYPE_MULTICAST_LISTENER_QUERY},
	ocbinds.OpenconfigIcmpv6Types_CODE_MULTICAST_LISTENER_REPORT_NO_CODE:            {0, ocbinds.OpenconfigIcmpv6Types_TYPE_MULTICAST_LISTENER_REPORT},
	ocbinds.OpenconfigIcmpv6Types_CODE_MULTICAST_LISTENER_DONE_NO_CODE:              {0, ocbinds.OpenconfigIcmpv6Types_TYPE_MULTICAST_LISTENER_DONE},
	ocbinds.OpenconfigIcmpv6Types_CODE_ROUTER_SOLICITATION_NO_CODE:                  {0, ocbinds.OpenconfigIcmpv6Types_TYPE_ROUTER_SOLICITATION},
	ocbinds.OpenconfigIcmpv6Types_CODE_ROUTER_ADVERTISEMENT_NO_CODE:                 {0, ocbinds.OpenconfigIcmpv6Types_TYPE_ROUTER_ADVERTISEMENT},
	ocbinds.OpenconfigIcmpv6Types_CODE_NEIGHBOR_SOLICITATION_NO_CODE:                {0, ocbinds.OpenconfigIcmpv6Types_TYPE_NEIGHBOR_SOLICITATION},
	ocbinds.OpenconfigIcmpv6Types_CODE_NEIGHBOR_ADVERTISEMENT_NO_CODE:               {0, ocbinds.OpenconfigIcmpv6Types_TYPE_NEIGHBOR_ADVERTISEMENT},
	ocbinds.OpenconfigIcmpv6Types_CODE_REDIRECT_NO_CODE:                             {0, ocbinds.OpenconfigIcmpv6Types_TYPE_REDIRECT},
	ocbinds.OpenconfigIcmpv6Types_CODE_INVERSE_NEIGHBOR_SOLICITATION_NO_CODE:        {0, ocbinds.OpenconfigIcmpv6Types_TYPE_INVERSE_NEIGHBOR_SOLICITATION},
	ocbinds.OpenconfigIcmpv6Types_CODE_INVERSE_NEIGHBOR_ADVERTISEMENT_NO_CODE:       {0, ocbinds.OpenconfigIcmpv6Types_TYPE_INVERSE_NEIGHBOR_ADVERTISEMENT},
	ocbinds.OpenconfigIcmpv6Types_CODE_HOME_AGENT_ADDRESS_DISCOVERY_REQUEST_NO_CODE: {0, ocbinds.OpenconfigIcmpv6Types_TYPE_HOME_AGENT_ADDRESS_DISCOVERY_REQUEST},
	ocbinds.OpenconfigIcmpv6Types_CODE_HOME_AGENT_ADDRESS_DISCOVERY_REPLY_NO_CODE:   {0, ocbinds.OpenconfigIcmpv6Types_TYPE_HOME_AGENT_ADDRESS_DISCOVERY_REPLY},
	ocbinds.OpenconfigIcmpv6Types_CODE_MOBILE_PREFIX_SOLICITATION_NO_CODE:           {0, ocbinds.OpenconfigIcmpv6Types_TYPE_MOBILE_PREFIX_SOLICITATION},
	ocbinds.OpenconfigIcmpv6Types_CODE_MOBILE_PREFIX_ADVERTISEMENT_NO_CODE:          {0, ocbinds.OpenconfigIcmpv6Types_TYPE_MOBILE_PREFIX_ADVERTISEMENT},

	// Type 138, router renumbering. The sequence number reset is code 255, not
	// the 2 its position in the identity list would suggest.
	ocbinds.OpenconfigIcmpv6Types_CODE_RENUMBERING_COMMAND:       {0, ocbinds.OpenconfigIcmpv6Types_TYPE_RENUNBERING},
	ocbinds.OpenconfigIcmpv6Types_CODE_RENUNBERING_RESULT:        {1, ocbinds.OpenconfigIcmpv6Types_TYPE_RENUNBERING},
	ocbinds.OpenconfigIcmpv6Types_CODE_RENUNBERING_SEQ_NUM_RESET: {255, ocbinds.OpenconfigIcmpv6Types_TYPE_RENUNBERING},

	// Types 139 and 140, node information query and response.
	ocbinds.OpenconfigIcmpv6Types_CODE_NODE_INFORMATION_QUERY_IPV6_ADDR:  {0, ocbinds.OpenconfigIcmpv6Types_TYPE_NODE_INFORMATION_QUERY},
	ocbinds.OpenconfigIcmpv6Types_CODE_NODE_INFORMATION_QUERY_NAME:       {1, ocbinds.OpenconfigIcmpv6Types_TYPE_NODE_INFORMATION_QUERY},
	ocbinds.OpenconfigIcmpv6Types_CODE_NODE_INFORMATION_QUERY_IPV4_ADDR:  {2, ocbinds.OpenconfigIcmpv6Types_TYPE_NODE_INFORMATION_QUERY},
	ocbinds.OpenconfigIcmpv6Types_CODE_NODE_INFORMATION_RESPONSE_SUCCESS: {0, ocbinds.OpenconfigIcmpv6Types_TYPE_NODE_INFORMATION_RESPONSE},
	ocbinds.OpenconfigIcmpv6Types_CODE_NODE_INFORMATION_RESPONSE_REFUSED: {1, ocbinds.OpenconfigIcmpv6Types_TYPE_NODE_INFORMATION_RESPONSE},
	ocbinds.OpenconfigIcmpv6Types_CODE_NODE_INFORMATION_RESPONSE_UNKNOWN: {2, ocbinds.OpenconfigIcmpv6Types_TYPE_NODE_INFORMATION_RESPONSE},

	// Types 157 and 158, duplicate address request and confirmation. The model
	// names the confirmation codes after "reply" while the type identity is
	// DUPLICATE_ADDRESS_CONFIRMATION.
	ocbinds.OpenconfigIcmpv6Types_CODE_DUPLICATE_ADDRESS_REQUEST_DAR:          {0, ocbinds.OpenconfigIcmpv6Types_TYPE_DUPLICATE_ADDRESS_REQUEST},
	ocbinds.OpenconfigIcmpv6Types_CODE_DUPLICATE_ADDRESS_REQUEST_EDAR_ROVR64:  {1, ocbinds.OpenconfigIcmpv6Types_TYPE_DUPLICATE_ADDRESS_REQUEST},
	ocbinds.OpenconfigIcmpv6Types_CODE_DUPLICATE_ADDRESS_REQUEST_EDAR_ROVR128: {2, ocbinds.OpenconfigIcmpv6Types_TYPE_DUPLICATE_ADDRESS_REQUEST},
	ocbinds.OpenconfigIcmpv6Types_CODE_DUPLICATE_ADDRESS_REQUEST_EDAR_ROVR192: {3, ocbinds.OpenconfigIcmpv6Types_TYPE_DUPLICATE_ADDRESS_REQUEST},
	ocbinds.OpenconfigIcmpv6Types_CODE_DUPLICATE_ADDRESS_REQUEST_EDAR_ROVR256: {4, ocbinds.OpenconfigIcmpv6Types_TYPE_DUPLICATE_ADDRESS_REQUEST},
	ocbinds.OpenconfigIcmpv6Types_CODE_DUPLICATE_ADDRESS_REPLY_DAC:            {0, ocbinds.OpenconfigIcmpv6Types_TYPE_DUPLICATE_ADDRESS_CONFIRMATION},
	ocbinds.OpenconfigIcmpv6Types_CODE_DUPLICATE_ADDRESS_REPLY_EDAC_ROVR64:    {1, ocbinds.OpenconfigIcmpv6Types_TYPE_DUPLICATE_ADDRESS_CONFIRMATION},
	ocbinds.OpenconfigIcmpv6Types_CODE_DUPLICATE_ADDRESS_REPLY_EDAC_ROVR128:   {2, ocbinds.OpenconfigIcmpv6Types_TYPE_DUPLICATE_ADDRESS_CONFIRMATION},
	ocbinds.OpenconfigIcmpv6Types_CODE_DUPLICATE_ADDRESS_REPLY_EDAC_ROVR192:   {3, ocbinds.OpenconfigIcmpv6Types_TYPE_DUPLICATE_ADDRESS_CONFIRMATION},
	ocbinds.OpenconfigIcmpv6Types_CODE_DUPLICATE_ADDRESS_REPLY_EDAC_ROVR256:   {4, ocbinds.OpenconfigIcmpv6Types_TYPE_DUPLICATE_ADDRESS_CONFIRMATION},

	// Types 160 and 161, extended echo request and reply.
	ocbinds.OpenconfigIcmpv6Types_CODE_EXT_ECHO_REQUEST_NO_ERROR:                  {0, ocbinds.OpenconfigIcmpv6Types_TYPE_EXT_ECHO_REQUEST},
	ocbinds.OpenconfigIcmpv6Types_CODE_EXT_ECHO_REPLY_NO_ERROR:                    {0, ocbinds.OpenconfigIcmpv6Types_TYPE_EXT_ECHO_REPLY},
	ocbinds.OpenconfigIcmpv6Types_CODE_EXT_ECHO_REPLY_MALFORMED_QUERY:             {1, ocbinds.OpenconfigIcmpv6Types_TYPE_EXT_ECHO_REPLY},
	ocbinds.OpenconfigIcmpv6Types_CODE_EXT_ECHO_REPLY_NO_SUCH_INTERFACE:           {2, ocbinds.OpenconfigIcmpv6Types_TYPE_EXT_ECHO_REPLY},
	ocbinds.OpenconfigIcmpv6Types_CODE_EXT_ECHO_REPLY_NO_SUCH_TABLE_ENTRY:         {3, ocbinds.OpenconfigIcmpv6Types_TYPE_EXT_ECHO_REPLY},
	ocbinds.OpenconfigIcmpv6Types_CODE_EXT_ECHO_REPLY_MULTIPLE_INTF_SATISFY_QUERY: {4, ocbinds.OpenconfigIcmpv6Types_TYPE_EXT_ECHO_REPLY},
}

// ICMPV6_CODE_GROUP_MAP maps the per-type identities the model groups the codes
// of a type under (DST_UNREACHABLE_CODE and friends) to their type. They are
// derived from CODE too, so a client can name one as a code, and one stands for
// every code of its type. SONiC has no number for that: it is a rule matching
// on the type alone, with no ICMPV6_CODE.
var ICMPV6_CODE_GROUP_MAP = map[ocbinds.E_OpenconfigIcmpv6Types_CODE]ocbinds.E_OpenconfigIcmpv6Types_TYPE{
	ocbinds.OpenconfigIcmpv6Types_CODE_DST_UNREACHABLE_CODE:                      ocbinds.OpenconfigIcmpv6Types_TYPE_DESTINATION_UNREACHABLE,
	ocbinds.OpenconfigIcmpv6Types_CODE_PACKET_TOO_BIG_CODE:                       ocbinds.OpenconfigIcmpv6Types_TYPE_PACKET_TOO_BIG,
	ocbinds.OpenconfigIcmpv6Types_CODE_TIME_EXCEEDED_CODE:                        ocbinds.OpenconfigIcmpv6Types_TYPE_TIME_EXCEEDED,
	ocbinds.OpenconfigIcmpv6Types_CODE_PARAM_PROBLEM_CODE:                        ocbinds.OpenconfigIcmpv6Types_TYPE_PARAMETER_PROBLEM,
	ocbinds.OpenconfigIcmpv6Types_CODE_ECHO_REQUEST_CODE:                         ocbinds.OpenconfigIcmpv6Types_TYPE_ECHO_REQUEST,
	ocbinds.OpenconfigIcmpv6Types_CODE_ECHO_REPLY_CODE:                           ocbinds.OpenconfigIcmpv6Types_TYPE_ECHO_REPLY,
	ocbinds.OpenconfigIcmpv6Types_CODE_MULTICAST_LISTENER_QUERY_CODE:             ocbinds.OpenconfigIcmpv6Types_TYPE_MULTICAST_LISTENER_QUERY,
	ocbinds.OpenconfigIcmpv6Types_CODE_MULTICAST_LISTENER_REPORT_CODE:            ocbinds.OpenconfigIcmpv6Types_TYPE_MULTICAST_LISTENER_REPORT,
	ocbinds.OpenconfigIcmpv6Types_CODE_MULTICAST_LISTENER_DONE_CODE:              ocbinds.OpenconfigIcmpv6Types_TYPE_MULTICAST_LISTENER_DONE,
	ocbinds.OpenconfigIcmpv6Types_CODE_ROUTER_SOLICITATION_CODE:                  ocbinds.OpenconfigIcmpv6Types_TYPE_ROUTER_SOLICITATION,
	ocbinds.OpenconfigIcmpv6Types_CODE_ROUTER_ADVERTISEMENT_CODE:                 ocbinds.OpenconfigIcmpv6Types_TYPE_ROUTER_ADVERTISEMENT,
	ocbinds.OpenconfigIcmpv6Types_CODE_NEIGHBOR_SOLICITATION_CODE:                ocbinds.OpenconfigIcmpv6Types_TYPE_NEIGHBOR_SOLICITATION,
	ocbinds.OpenconfigIcmpv6Types_CODE_NEIGHBOR_ADVERTISEMENT_CODE:               ocbinds.OpenconfigIcmpv6Types_TYPE_NEIGHBOR_ADVERTISEMENT,
	ocbinds.OpenconfigIcmpv6Types_CODE_REDIRECT_CODE:                             ocbinds.OpenconfigIcmpv6Types_TYPE_REDIRECT,
	ocbinds.OpenconfigIcmpv6Types_CODE_RENUMBERING_CODE:                          ocbinds.OpenconfigIcmpv6Types_TYPE_RENUNBERING,
	ocbinds.OpenconfigIcmpv6Types_CODE_NODE_INFORMATION_QUERY_CODE:               ocbinds.OpenconfigIcmpv6Types_TYPE_NODE_INFORMATION_QUERY,
	ocbinds.OpenconfigIcmpv6Types_CODE_NDDE_INFORMATION_RESPONSE_CODE:            ocbinds.OpenconfigIcmpv6Types_TYPE_NODE_INFORMATION_RESPONSE,
	ocbinds.OpenconfigIcmpv6Types_CODE_INVERSE_NEIGHBOR_SOLICITATION_CODE:        ocbinds.OpenconfigIcmpv6Types_TYPE_INVERSE_NEIGHBOR_SOLICITATION,
	ocbinds.OpenconfigIcmpv6Types_CODE_INVERSE_NEIGHBOR_ADVERTISEMENT_CODE:       ocbinds.OpenconfigIcmpv6Types_TYPE_INVERSE_NEIGHBOR_ADVERTISEMENT,
	ocbinds.OpenconfigIcmpv6Types_CODE_HOME_AGENT_ADDRESS_DISCOVERY_REQUEST_CODE: ocbinds.OpenconfigIcmpv6Types_TYPE_HOME_AGENT_ADDRESS_DISCOVERY_REQUEST,
	ocbinds.OpenconfigIcmpv6Types_CODE_HOME_AGENT_ADDRESS_DISCOVERY_REPLY_CODE:   ocbinds.OpenconfigIcmpv6Types_TYPE_HOME_AGENT_ADDRESS_DISCOVERY_REPLY,
	ocbinds.OpenconfigIcmpv6Types_CODE_MOBILE_PREFIX_SOLICITATION_CODE:           ocbinds.OpenconfigIcmpv6Types_TYPE_MOBILE_PREFIX_SOLICITATION,
	ocbinds.OpenconfigIcmpv6Types_CODE_MOBILE_PREFIX_ADVERTISEMENT_CODE:          ocbinds.OpenconfigIcmpv6Types_TYPE_MOBILE_PREFIX_ADVERTISEMENT,
	ocbinds.OpenconfigIcmpv6Types_CODE_DUPLICATE_ADDRESS_REQUEST_CODE:            ocbinds.OpenconfigIcmpv6Types_TYPE_DUPLICATE_ADDRESS_REQUEST,
	ocbinds.OpenconfigIcmpv6Types_CODE_DUPLICATE_ADDRESS_REPLY_CODE:              ocbinds.OpenconfigIcmpv6Types_TYPE_DUPLICATE_ADDRESS_CONFIRMATION,
	ocbinds.OpenconfigIcmpv6Types_CODE_EXT_ECHO_REQUEST_CODE:                     ocbinds.OpenconfigIcmpv6Types_TYPE_EXT_ECHO_REQUEST,
	ocbinds.OpenconfigIcmpv6Types_CODE_EXT_ECHO_REPLY_CODE:                       ocbinds.OpenconfigIcmpv6Types_TYPE_EXT_ECHO_REPLY,
}

var ETHERTYPE_MAP = map[ocbinds.E_OpenconfigPacketMatchTypes_ETHERTYPE]uint32{
	ocbinds.OpenconfigPacketMatchTypes_ETHERTYPE_ETHERTYPE_LLDP: 0x88CC,
	ocbinds.OpenconfigPacketMatchTypes_ETHERTYPE_ETHERTYPE_VLAN: 0x8100,
	ocbinds.OpenconfigPacketMatchTypes_ETHERTYPE_ETHERTYPE_ROCE: 0x8915,
	ocbinds.OpenconfigPacketMatchTypes_ETHERTYPE_ETHERTYPE_ARP:  0x0806,
	ocbinds.OpenconfigPacketMatchTypes_ETHERTYPE_ETHERTYPE_IPV4: 0x0800,
	ocbinds.OpenconfigPacketMatchTypes_ETHERTYPE_ETHERTYPE_IPV6: 0x86DD,
	ocbinds.OpenconfigPacketMatchTypes_ETHERTYPE_ETHERTYPE_MPLS: 0x8847,
}

type AclApp struct {
	pathInfo   *PathInfo
	ygotRoot   *ygot.GoStruct
	ygotTarget *interface{}
	appOptions

	aclTs  *db.TableSpec
	ruleTs *db.TableSpec
	refTs  *db.TableSpec

	aclTableMap  map[string]db.Value
	ruleTableMap map[string]map[string]db.Value

	// Port set references of the rules in the request, keyed by acl name and
	// rule name. On a write these describe what the payload asked for; on a
	// read they hold the ACL_RULE_PORT_SET rows.
	rulePortSetMap map[string]map[string]*rulePortSetInfo
	rulePortSetRef map[string]map[string]db.Value

	// Rules in the request whose icmpv6 code is a per-type group, keyed by acl
	// name and rule name. They match every code of the type, so a code already
	// stored on them has to go.
	ruleIcmpv6CodeGroup map[string]map[string]bool

	// Counters are read outside the ConfigDB handle passed to processCommon.
	countersDb  *db.DB
	counterOids db.Value
	oidsRead    bool
}

func init() {

	err := register("/openconfig-acl:acl",
		&appInfo{appType: reflect.TypeOf(AclApp{}),
			ygotRootType:  reflect.TypeOf(ocbinds.OpenconfigAcl_Acl{}),
			isNative:      false,
			tablesToWatch: []*db.TableSpec{&db.TableSpec{Name: ACL_TABLE}, &db.TableSpec{Name: RULE_TABLE}, &db.TableSpec{Name: RULE_PORT_SET_TABLE}}})

	if err != nil {
		log.Fatal("Register ACL app module with App Interface failed with error=", err)
	}

	err = addModel(&ModelData{Name: "openconfig-acl",
		Org: "OpenConfig working group",
		Ver: "1.3.3"})
	if err != nil {
		log.Fatal("Adding model data to appinterface failed with error=", err)
	}
}

func (app *AclApp) initialize(data appData) {
	log.Info("initialize:acl:path =", data.path)
	pathInfo := NewPathInfo(data.path)
	*app = AclApp{pathInfo: pathInfo, ygotRoot: data.ygotRoot, ygotTarget: data.ygotTarget, appOptions: data.appOptions}

	app.aclTs = &db.TableSpec{Name: ACL_TABLE}
	app.ruleTs = &db.TableSpec{Name: RULE_TABLE}
	app.refTs = &db.TableSpec{Name: RULE_PORT_SET_TABLE}

	app.aclTableMap = make(map[string]db.Value)
	app.ruleTableMap = make(map[string]map[string]db.Value)
	app.rulePortSetMap = make(map[string]map[string]*rulePortSetInfo)
	app.rulePortSetRef = make(map[string]map[string]db.Value)
	app.ruleIcmpv6CodeGroup = make(map[string]map[string]bool)
}

func (app *AclApp) getAppRootObject() *ocbinds.OpenconfigAcl_Acl {
	deviceObj := (*app.ygotRoot).(*ocbinds.Device)
	return deviceObj.Acl
}

func (app *AclApp) translateCreate(d *db.DB) ([]db.WatchKeys, error) {
	return app.translateCRUCommon(d, CREATE)
}

func (app *AclApp) translateUpdate(d *db.DB) ([]db.WatchKeys, error) {
	return app.translateCRUCommon(d, UPDATE)
}

func (app *AclApp) translateReplace(d *db.DB) ([]db.WatchKeys, error) {
	return app.translateCRUCommon(d, REPLACE)
}

func (app *AclApp) translateDelete(d *db.DB) ([]db.WatchKeys, error) {
	return nil, nil
}

func (app *AclApp) translateGet(dbs [db.MaxDB]*db.DB) error {
	return nil
}

func (app *AclApp) translateAction(dbs [db.MaxDB]*db.DB) error {
	return tlerr.NotSupported("unsupported")
}

func (app *AclApp) processCreate(d *db.DB) (SetResponse, error) {
	var err error
	var resp SetResponse

	if err = app.processCommon(d, CREATE); err != nil {
		log.Error(err)
		resp = SetResponse{ErrSrc: AppErr}
	}
	return resp, err
}

func (app *AclApp) processUpdate(d *db.DB) (SetResponse, error) {
	var err error
	var resp SetResponse

	if err = app.processCommon(d, UPDATE); err != nil {
		log.Error(err)
		resp = SetResponse{ErrSrc: AppErr}
	}
	return resp, err
}

func (app *AclApp) processReplace(d *db.DB) (SetResponse, error) {
	var err error
	var resp SetResponse

	if err = app.processCommon(d, REPLACE); err != nil {
		log.Error(err)
		resp = SetResponse{ErrSrc: AppErr}
	}
	return resp, err
}

func (app *AclApp) processDelete(d *db.DB) (SetResponse, error) {
	var err error
	var resp SetResponse

	if err = app.processCommon(d, DELETE); err != nil {
		log.Error(err)
		resp = SetResponse{ErrSrc: AppErr}
	}
	return resp, err
}

func (app *AclApp) processGet(dbs [db.MaxDB]*db.DB, fmtType TranslibFmtType) (GetResponse, error) {
	var err error
	var payload []byte

	configDb := dbs[db.ConfigDB]
	app.countersDb = dbs[db.CountersDB]
	err = app.processCommon(configDb, GET)
	if err != nil {
		return GetResponse{Payload: payload, ErrSrc: AppErr}, err
	}

	if err = filterGetContent(app.pathInfo.Template, app.content, app.ygotRoot); err != nil {
		return GetResponse{Payload: payload, ErrSrc: AppErr}, err
	}

	return generateGetResponse(app.pathInfo.Path, app.ygotRoot, fmtType)
}

func (app *AclApp) processAction(dbs [db.MaxDB]*db.DB) (ActionResponse, error) {
	return ActionResponse{}, tlerr.New("not implemented")
}

func (app *AclApp) translateCRUCommon(d *db.DB, opcode int) ([]db.WatchKeys, error) {
	var err error
	var keys []db.WatchKeys
	log.Info("translateCRUCommon:acl:path =", app.pathInfo.Template)

	app.convertOCAclsToInternal()
	if err = app.convertOCAclRulesToInternal(d, opcode); err != nil {
		return keys, err
	}
	app.convertOCAclBindingsToInternal()

	return keys, err
}

func (app *AclApp) processCommon(d *db.DB, opcode int) error {
	var err error
	var topmostPath bool = false
	acl := app.getAppRootObject()

	log.Infof("processCommon--Path Received: %s", app.pathInfo.Template)
	targetType := reflect.TypeOf(*app.ygotTarget)
	if !util.IsValueScalar(reflect.ValueOf(*app.ygotTarget)) && util.IsValuePtr(reflect.ValueOf(*app.ygotTarget)) {
		log.Infof("processCommon: Target object is a <%s> of Type: %s", targetType.Kind().String(), targetType.Elem().Name())
		if targetType.Elem().Name() == "OpenconfigAcl_Acl" {
			topmostPath = true
		}
	}

	targetUriPath, _ := getYangPathFromUri(app.pathInfo.Path)
	if isSubtreeRequest(app.pathInfo.Template, "/openconfig-acl:acl/acl-sets") {
		if isSubtreeRequest(app.pathInfo.Template, "/openconfig-acl:acl/acl-sets/acl-set{}{}") {
			for aclSetKey, _ := range acl.AclSets.AclSet {
				aclSet := acl.AclSets.AclSet[aclSetKey]
				aclKey := getAclKeyStrFromOCKey(aclSetKey.Name, aclSetKey.Type)

				if isSubtreeRequest(app.pathInfo.Template, "/openconfig-acl:acl/acl-sets/acl-set{}{}/acl-entries/acl-entry{}") {
					// Subtree of one Rule
					for seqId, _ := range aclSet.AclEntries.AclEntry {
						ruleKey := "RULE_" + strconv.Itoa(int(seqId))
						entrySet := aclSet.AclEntries.AclEntry[seqId]

						ruleNodeYangPath := getYangPathFromYgotStruct(entrySet, OC_ACL_YANG_PATH_PREFIX, OC_ACL_APP_MODULE_NAME)
						isRuleNodeSubtree := len(targetUriPath) > len(ruleNodeYangPath)
						switch opcode {
						case CREATE:
							if isRuleNodeSubtree {
								err = app.setAclRuleDataInConfigDb(d, app.ruleTableMap, false)
							} else if *app.ygotTarget == entrySet {
								err = app.setAclRuleDataInConfigDb(d, app.ruleTableMap, true)
							} else {
								log.Errorf("processCommon: Given CREATE path %s not handled", targetUriPath)
							}
						case REPLACE:
							if isRuleNodeSubtree {
								if err = app.clearStaleRulePortMatch(d, aclKey, ruleKey, app.ruleTableMap[aclKey][ruleKey]); err != nil {
									return err
								}
								if err = app.clearStaleRuleIcmpv6Code(d, aclKey, ruleKey); err != nil {
									return err
								}
								err = d.ModEntry(app.ruleTs, db.Key{Comp: []string{aclKey, ruleKey}}, app.ruleTableMap[aclKey][ruleKey])
							} else {
								if err = app.clearStaleRulePortMatch(d, aclKey, ruleKey, app.ruleTableMap[aclKey][ruleKey]); err != nil {
									return err
								}
								err = d.SetEntry(app.ruleTs, db.Key{Comp: []string{aclKey, ruleKey}}, app.ruleTableMap[aclKey][ruleKey])
							}
						case UPDATE:
							if err = app.clearStaleRulePortMatch(d, aclKey, ruleKey, app.ruleTableMap[aclKey][ruleKey]); err != nil {
								return err
							}
							if err = app.clearStaleRuleIcmpv6Code(d, aclKey, ruleKey); err != nil {
								return err
							}
							err = d.ModEntry(app.ruleTs, db.Key{Comp: []string{aclKey, ruleKey}}, app.ruleTableMap[aclKey][ruleKey])
						case DELETE:
							if *app.ygotTarget == entrySet {
								if err = app.deleteRulePortSetRefs(d, aclKey, ruleKey); err != nil {
									return err
								}
								err = d.DeleteEntry(app.ruleTs, db.Key{Comp: []string{aclKey, ruleKey}})
							} else if isRuleNodeSubtree {
								err = app.handleRuleFieldsDeletion(d, aclKey, ruleKey)
								if err != nil {
									return err
								}
								//err = d.SetEntry(app.ruleTs, db.Key{Comp: []string{aclKey, ruleKey}}, app.ruleTableMap[aclKey][ruleKey])
							} else {
								log.Errorf("processCommon: Given DELETE path %s not handled", targetUriPath)
							}
						case GET:
							err = app.convertDBAclRulesToInternal(d, aclKey, int64(seqId), db.Key{})
							ygot.BuildEmptyTree(entrySet)
							app.convertInternalToOCAclRule(aclKey, aclSetKey.Type, int64(seqId), nil, entrySet)
						}
					}
				} else {
					isAclEntriesSubtree := isSubtreeRequest(app.pathInfo.Template, "/openconfig-acl:acl/acl-sets/acl-set{}{}/acl-entries")
					switch opcode {
					case CREATE:
						if *app.ygotTarget == aclSet {
							err = app.setAclDataInConfigDb(d, app.aclTableMap, true)
							if err != nil {
								return err
							}
							err = app.setAclRuleDataInConfigDb(d, app.ruleTableMap, true)
						} else if isAclEntriesSubtree {
							err = app.setAclRuleDataInConfigDb(d, app.ruleTableMap, true)
						} else {
							err = d.SetEntry(app.aclTs, db.Key{Comp: []string{aclKey}}, app.aclTableMap[aclKey])
						}
					case REPLACE:
						// ACL_RULE:aclname is a leafref to ACL_TABLE:aclname and CVL
						// validates every write as it is added to the transaction, so
						// the table row has to be written before its rules. An empty
						// table map carries no table level fields to write; skipping it
						// avoids ModEntry falling back to deleting the ACL_TABLE row
						// out from under the rules being created here.
						if !isAclEntriesSubtree && len(app.aclTableMap[aclKey].Field) > 0 {
							existingAcl, _ := d.GetEntry(app.aclTs, asKey(aclKey))
							if existingAcl.IsPopulated() {
								// Merge instead of overwrite: ports and stage belong
								// to the interface binding, not to this payload.
								err = d.ModEntry(app.aclTs, asKey(aclKey), app.aclTableMap[aclKey])
							} else {
								// CVL rejects an update against a key that does not
								// exist yet, so a brand new ACL has to be created.
								err = d.CreateEntry(app.aclTs, asKey(aclKey), app.aclTableMap[aclKey])
							}
							if err != nil {
								return err
							}
						}
						if *app.ygotTarget == aclSet || isAclEntriesSubtree {
							// The rules are recreated below, so their references
							// are rebuilt by syncRulePortSetRefs.
							if err = app.deleteAclPortSetRefs(d, aclKey); err != nil {
								return err
							}
							err = d.DeleteKeys(app.ruleTs, db.Key{Comp: []string{aclKey + TABLE_SEPARATOR + "RULE_*"}})
							if err != nil {
								return err
							}
							err = app.setAclRuleDataInConfigDb(d, app.ruleTableMap, true)
							if err != nil {
								return err
							}
						}
					case UPDATE:
						if !isAclEntriesSubtree {
							err = app.setAclDataInConfigDb(d, app.aclTableMap, false)
							//err = d.ModEntry(app.aclTs, db.Key{Comp: []string{aclKey}}, app.aclTableMap[aclKey])
							if err != nil {
								return err
							}
						}
						if *app.ygotTarget == aclSet || isAclEntriesSubtree {
							err = app.setAclRuleDataInConfigDb(d, app.ruleTableMap, false)
						}
					case DELETE:
						if *app.ygotTarget == aclSet {
							if err = app.deleteAclPortSetRefs(d, aclKey); err != nil {
								return err
							}
							err = d.DeleteKeys(app.ruleTs, db.Key{Comp: []string{aclKey + TABLE_SEPARATOR + "*"}})
							if err != nil {
								return err
							}
							err = d.DeleteEntry(app.aclTs, db.Key{Comp: []string{aclKey}})
						} else if isAclEntriesSubtree {
							if err = app.deleteAclPortSetRefs(d, aclKey); err != nil {
								return err
							}
							err = d.DeleteKeys(app.ruleTs, db.Key{Comp: []string{aclKey + TABLE_SEPARATOR + "RULE_*"}})
						} else {
							nodeInfo, err := getTargetNodeYangSchema(app.pathInfo.Path, (*app.ygotRoot).(*ocbinds.Device))
							if err != nil {
								return err
							}
							if nodeInfo != nil && nodeInfo.IsLeaf() && nodeInfo.Name == "description" {
								err = d.DeleteEntryFields(app.aclTs, asKey(aclKey), createEmptyDbValue(ACL_DESCRIPTION))
							}
							//err = d.SetEntry(app.aclTs, db.Key{Comp: []string{aclKey}}, app.aclTableMap[aclKey])
						}
					case GET:
						err = app.convertDBAclToInternal(d, db.Key{Comp: []string{aclKey}})
						if err != nil {
							return err
						}
						ygot.BuildEmptyTree(aclSet)
						app.convertInternalToOCAcl(aclKey, acl.AclSets, aclSet)
					}
				}
			}
		} else {
			// All Acls and their rules
			err = app.processCommonToplevelPath(d, acl, opcode, false)
		}
	} else if isSubtreeRequest(app.pathInfo.Template, "/openconfig-acl:acl/interfaces") {
		switch opcode {
		case CREATE, REPLACE, UPDATE:
			if opcode == REPLACE {
				if err = app.replaceInterfaceAclBindings(d); err != nil {
					return err
				}
			}
			err = app.setAclBindDataInConfigDb(d, app.aclTableMap, opcode)
		case DELETE:
			err = app.handleBindingsDeletion(d)
		case GET:
			if isSubtreeRequest(app.pathInfo.Template, "/openconfig-acl:acl/interfaces/interface{}") {
				for intfId := range acl.Interfaces.Interface {
					intfData := acl.Interfaces.Interface[intfId]
					ygot.BuildEmptyTree(intfData)
					if isSubtreeRequest(targetUriPath, "/openconfig-acl:acl/interfaces/interface/ingress-acl-sets") {
						err = app.getAclBindingInfoForInterfaceData(d, intfData, intfId, "INGRESS")
					} else if isSubtreeRequest(targetUriPath, "/openconfig-acl:acl/interfaces/interface/egress-acl-sets") {
						err = app.getAclBindingInfoForInterfaceData(d, intfData, intfId, "EGRESS")
					} else {
						// Direction unknown. Check ACL Table for binding information.
						err = app.getAclBindingInfoForInterfaceData(d, intfData, intfId, "INGRESS")
						if err != nil {
							return err
						}
						err = app.getAclBindingInfoForInterfaceData(d, intfData, intfId, "EGRESS")
					}
				}
			} else {
				err = app.getAllBindingsInfo(d)
			}
		}
	} else {
		err = app.processCommonToplevelPath(d, acl, opcode, true)
	}

	if !topmostPath && !isSubtreeRequest(targetUriPath, "/openconfig-acl:acl/acl-sets") && !isSubtreeRequest(targetUriPath, "/openconfig-acl:acl/interfaces") {
		err = tlerr.NotSupported("URL %s is not supported", app.pathInfo.Template)
	}

	// The rules are in the transaction by now, so their port set references can
	// be brought in line with them.
	if err == nil && (opcode == CREATE || opcode == REPLACE || opcode == UPDATE) {
		err = app.syncRulePortSetRefs(d, opcode)
	}

	return err
}

func (app *AclApp) processCommonToplevelPath(d *db.DB, acl *ocbinds.OpenconfigAcl_Acl, opcode int, isTopmostPath bool) error {
	var err error
	switch opcode {
	case CREATE:
		err = app.setAclDataInConfigDb(d, app.aclTableMap, true)
		if err != nil {
			return err
		}
		err = app.setAclRuleDataInConfigDb(d, app.ruleTableMap, true)
	case REPLACE:
		err = d.DeleteTable(app.aclTs)
		if err != nil {
			return err
		}
		err = d.DeleteTable(app.ruleTs)
		if err != nil {
			return err
		}
		// Every rule is gone, so no reference can still be pointing at a port
		// set. The rules created below get theirs from syncRulePortSetRefs.
		err = d.DeleteTable(app.refTs)
		if err != nil {
			return err
		}
		err = app.setAclDataInConfigDb(d, app.aclTableMap, true)
		if err != nil {
			return err
		}
		err = app.setAclRuleDataInConfigDb(d, app.ruleTableMap, true)
	case UPDATE:
		err = app.setAclDataInConfigDb(d, app.aclTableMap, false)
		if err != nil {
			return err
		}
		err = app.setAclRuleDataInConfigDb(d, app.ruleTableMap, false)
	case DELETE:
		err = d.DeleteTable(app.refTs)
		if err != nil {
			return err
		}
		err = d.DeleteTable(app.ruleTs)
		if err != nil {
			return err
		}
		err = d.DeleteTable(app.aclTs)
	case GET:
		ygot.BuildEmptyTree(acl)
		err = app.convertDBAclToInternal(d, db.Key{})
		if err != nil {
			return err
		}
		app.convertInternalToOCAcl("", acl.AclSets, nil)
		if isTopmostPath {
			err = app.getAllBindingsInfo(d)
		}
	}
	return err
}

/***********    These are Translation Helper Function   ***********/
func (app *AclApp) convertDBAclRulesToInternal(dbCl *db.DB, aclName string, seqId int64, ruleKey db.Key) error {
	var err error
	if seqId != -1 {
		ruleKey.Comp = []string{aclName, "RULE_" + strconv.FormatInt(int64(seqId), 10)}
	}
	if ruleKey.Len() > 1 {
		ruleName := ruleKey.Get(1)
		if ruleName != "DEFAULT_RULE" {
			ruleData, err := dbCl.GetEntry(app.ruleTs, ruleKey)
			if err != nil {
				return err
			}
			if app.ruleTableMap[aclName] == nil {
				app.ruleTableMap[aclName] = make(map[string]db.Value)
			}
			app.ruleTableMap[aclName][ruleName] = ruleData

			// A rule matching on a port set holds the port the set resolved to,
			// so the set it came from has to be read from its own table.
			if refs := rulePortSetRefs(dbCl, aclName, ruleName); refs.IsPopulated() {
				if app.rulePortSetRef[aclName] == nil {
					app.rulePortSetRef[aclName] = make(map[string]db.Value)
				}
				app.rulePortSetRef[aclName][ruleName] = refs
			}
		}
	} else {
		ruleKeys, err := dbCl.GetKeys(app.ruleTs)
		if err != nil {
			return err
		}
		for i, _ := range ruleKeys {
			if aclName == ruleKeys[i].Get(0) {
				app.convertDBAclRulesToInternal(dbCl, aclName, -1, ruleKeys[i])
			}
		}
	}
	return err
}

func (app *AclApp) convertDBAclToInternal(dbCl *db.DB, aclkey db.Key) error {
	var err error
	if aclkey.Len() > 0 {
		// Get one particular ACL
		entry, err := dbCl.GetEntry(app.aclTs, aclkey)
		if err != nil {
			return err
		}
		if entry.IsPopulated() {
			app.aclTableMap[aclkey.Get(0)] = entry
			app.ruleTableMap[aclkey.Get(0)] = make(map[string]db.Value)
			err = app.convertDBAclRulesToInternal(dbCl, aclkey.Get(0), -1, db.Key{})
			if err != nil {
				return err
			}
		} else {
			return tlerr.NotFound("Acl %s is not configured", aclkey.Get(0))
		}
	} else {
		// Get all ACLs
		tbl, err := dbCl.GetTable(app.aclTs)
		if err != nil {
			return err
		}
		keys, _ := tbl.GetKeys()
		for i, _ := range keys {
			app.convertDBAclToInternal(dbCl, keys[i])
		}
	}
	return err
}

func (app *AclApp) convertInternalToOCAcl(aclName string, aclSets *ocbinds.OpenconfigAcl_Acl_AclSets, aclSet *ocbinds.OpenconfigAcl_Acl_AclSets_AclSet) {
	if len(aclName) > 0 {
		aclData := app.aclTableMap[aclName]
		if aclSet != nil {
			aclSet.Config.Name = aclSet.Name
			aclSet.Config.Type = aclSet.Type
			aclSet.State.Name = aclSet.Name
			aclSet.State.Type = aclSet.Type

			for k := range aclData.Field {
				if ACL_DESCRIPTION == k {
					descr := aclData.Get(k)
					aclSet.Config.Description = &descr
					aclSet.State.Description = &descr
				} else if "ports@" == k {
					continue
				}
			}

			// openconfig-acl-ext: reflect control-plane / management ACL marking
			// (SONiC CTRLPLANE table + services) back into the OC acl-set.
			if aclData.Get(ACL_TYPE) == SONIC_ACL_TYPE_CTRLPLANE {
				cp := true
				aclSet.Config.ControlPlane = &cp
				aclSet.State.ControlPlane = &cp
				if services := aclData.GetList(ACL_SERVICES); len(services) > 0 {
					aclSet.Config.Services = append([]string{}, services...)
					aclSet.State.Services = append([]string{}, services...)
				}
			}

			app.convertInternalToOCAclRule(aclName, aclSet.Type, -1, aclSet, nil)
		}
	} else {
		for acln := range app.aclTableMap {
			acldata := app.aclTableMap[acln]
			var aclNameStr string
			var aclType ocbinds.E_OpenconfigAcl_ACL_TYPE
			if acldata.Get(ACL_TYPE) == SONIC_ACL_TYPE_IPV4 {
				aclNameStr = ocAclSetNameFromTable(acldata, acln, SONIC_ACL_TYPE_IPV4)
				aclType = ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV4
			} else if acldata.Get(ACL_TYPE) == SONIC_ACL_TYPE_IPV6 {
				aclNameStr = ocAclSetNameFromTable(acldata, acln, SONIC_ACL_TYPE_IPV6)
				aclType = ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV6
			} else if acldata.Get(ACL_TYPE) == SONIC_ACL_TYPE_L2 {
				aclNameStr = ocAclSetNameFromTable(acldata, acln, SONIC_ACL_TYPE_L2)
				aclType = ocbinds.OpenconfigAcl_ACL_TYPE_ACL_L2
			} else if acldata.Get(ACL_TYPE) == SONIC_ACL_TYPE_CTRLPLANE {
				// A control-plane (CTRLPLANE) table does not carry the OC type in
				// its ACL_TYPE field; recover it from the table-name suffix that
				// was preserved when the acl-set was created.
				aclNameStr, aclType = ctrlPlaneOCNameType(acln)
			}
			aclSetPtr, aclErr := aclSets.NewAclSet(aclNameStr, aclType)
			if aclErr != nil {
				fmt.Println("Error handling: ", aclErr)
			}
			ygot.BuildEmptyTree(aclSetPtr)
			app.convertInternalToOCAcl(acln, nil, aclSetPtr)
		}
	}
}

func (app *AclApp) convertInternalToOCAclRule(aclName string, aclType ocbinds.E_OpenconfigAcl_ACL_TYPE, seqId int64, aclSet *ocbinds.OpenconfigAcl_Acl_AclSets_AclSet, entrySet *ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry) {
	if seqId != -1 {
		ruleName := "RULE_" + strconv.FormatInt(int64(seqId), 10)
		app.convertInternalToOCAclRuleProperties(aclName, ruleName, app.ruleTableMap[aclName][ruleName], aclType, nil, entrySet)
	} else {
		for ruleName := range app.ruleTableMap[aclName] {
			app.convertInternalToOCAclRuleProperties(aclName, ruleName, app.ruleTableMap[aclName][ruleName], aclType, aclSet, nil)
		}
	}
}

// aclRuleCounters returns the packet and byte counts of a rule from COUNTERS_DB.
// Counters are published by the flex counter service only while ACL polling is
// enabled ("counterpoll acl enable"); they read as zero otherwise, which is what
// the CLI reports as "N/A". The reason for a zero is logged for diagnosis.
func (app *AclApp) aclRuleCounters(aclKey, ruleKey string) (pkts uint64, octets uint64) {
	if app.countersDb == nil {
		log.V(2).Infof("aclRuleCounters: no counters db handle for %s:%s", aclKey, ruleKey)
		return 0, 0
	}

	// The whole map is fetched once per request; a rule tree can hold thousands
	// of rules, and every one of them would otherwise re-read it.
	if !app.oidsRead {
		oids, err := app.countersDb.GetMapAll(&db.TableSpec{Name: ACL_COUNTER_RULE_MAP})
		if err != nil {
			log.V(2).Infof("aclRuleCounters: %s unavailable; err=%v", ACL_COUNTER_RULE_MAP, err)
		}
		app.counterOids = oids
		app.oidsRead = true
	}

	oid := app.counterOids.Get(aclKey + ":" + ruleKey)
	if len(oid) == 0 {
		log.V(2).Infof("aclRuleCounters: no counter oid for %s:%s", aclKey, ruleKey)
		return 0, 0
	}

	entry, err := app.countersDb.GetEntry(&db.TableSpec{Name: COUNTERS_TABLE}, asKey(oid))
	if err != nil {
		log.V(2).Infof("aclRuleCounters: no counters for oid %s; err=%v", oid, err)
		return 0, 0
	}

	pkts, pErr := strconv.ParseUint(entry.Get(ACL_COUNTER_ATTR_PACKETS), 10, 64)
	octets, oErr := strconv.ParseUint(entry.Get(ACL_COUNTER_ATTR_BYTES), 10, 64)
	if pErr != nil || oErr != nil {
		log.V(2).Infof("aclRuleCounters: oid %s counters unreadable; pkts=%v octets=%v", oid, pErr, oErr)
	}

	return pkts, octets
}

func (app *AclApp) convertInternalToOCAclRuleProperties(aclName string, ruleName string, ruleData db.Value, aclType ocbinds.E_OpenconfigAcl_ACL_TYPE, aclSet *ocbinds.OpenconfigAcl_Acl_AclSets_AclSet, entrySet *ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry) {
	priority, _ := strconv.ParseInt(ruleData.Get("PRIORITY"), 10, 32)
	seqId := uint32(MAX_PRIORITY - priority)
	//ruleDescr := ruleData.Get("RULE_DESCRIPTION")

	if entrySet == nil {
		if aclSet != nil {
			entrySet_, _ := aclSet.AclEntries.NewAclEntry(seqId)
			entrySet = entrySet_
			ygot.BuildEmptyTree(entrySet)
		}
	}

	entrySet.Config.SequenceId = &seqId
	//entrySet.Config.Description = &ruleDescr
	entrySet.State.SequenceId = &seqId
	//entrySet.State.Description = &ruleDescr

	pkts, octets := app.aclRuleCounters(aclName, ruleName)
	entrySet.State.MatchedPackets = &pkts
	entrySet.State.MatchedOctets = &octets

	ygot.BuildEmptyTree(entrySet.Transport)
	ygot.BuildEmptyTree(entrySet.Actions)

	for ruleKey := range ruleData.Field {
		if "L4_SRC_PORT" == ruleKey || "L4_SRC_PORT_RANGE" == ruleKey {
			port := ruleData.Get(ruleKey)
			srcPort := getTransportSrcDestPorts(port, "src")
			entrySet.Transport.Config.SourcePort, _ = entrySet.Transport.Config.To_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_SourcePort_Union(srcPort)
			entrySet.Transport.State.SourcePort, _ = entrySet.Transport.State.To_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_State_SourcePort_Union(srcPort)
		} else if "L4_DST_PORT" == ruleKey || "L4_DST_PORT_RANGE" == ruleKey {
			port := ruleData.Get(ruleKey)
			destPort := getTransportSrcDestPorts(port, "dest")
			entrySet.Transport.Config.DestinationPort, _ = entrySet.Transport.Config.To_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_DestinationPort_Union(destPort)
			entrySet.Transport.State.DestinationPort, _ = entrySet.Transport.State.To_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_State_DestinationPort_Union(destPort)
		} else if "TCP_FLAGS" == ruleKey {
			tcpFlags := ruleData.Get(ruleKey)
			entrySet.Transport.Config.TcpFlags = getTransportConfigTcpFlags(tcpFlags)
			entrySet.Transport.State.TcpFlags = getTransportConfigTcpFlags(tcpFlags)
		} else if "PACKET_ACTION" == ruleKey {
			// "FORWARD" is the data-plane (SAI) accept value; "ACCEPT" is the
			// control-plane (caclmgrd/iptables) accept value. Both map to the
			// OpenConfig ACCEPT forwarding-action.
			if "FORWARD" == ruleData.Get(ruleKey) || "ACCEPT" == ruleData.Get(ruleKey) {
				entrySet.Actions.Config.ForwardingAction = ocbinds.OpenconfigAcl_FORWARDING_ACTION_ACCEPT
				entrySet.Actions.State.ForwardingAction = ocbinds.OpenconfigAcl_FORWARDING_ACTION_ACCEPT
			} else {
				entrySet.Actions.Config.ForwardingAction = ocbinds.OpenconfigAcl_FORWARDING_ACTION_DROP
				entrySet.Actions.State.ForwardingAction = ocbinds.OpenconfigAcl_FORWARDING_ACTION_DROP
			}
		}
	}

	// The L4 port match above is the port a set resolved to, not something the
	// client configured. Report the set it came from, which is held in
	// ACL_RULE_PORT_SET, in place of the port.
	if refs, ok := app.rulePortSetRef[aclName][ruleName]; ok {
		if srcSet := (&refs).Get(RULE_SRC_PORT_SET); len(srcSet) > 0 {
			entrySet.Transport.Config.SourcePortSet = &srcSet
			entrySet.Transport.State.SourcePortSet = &srcSet
			entrySet.Transport.Config.SourcePort = nil
			entrySet.Transport.State.SourcePort = nil
		}
		if dstSet := (&refs).Get(RULE_DST_PORT_SET); len(dstSet) > 0 {
			entrySet.Transport.Config.DestinationPortSet = &dstSet
			entrySet.Transport.State.DestinationPortSet = &dstSet
			entrySet.Transport.Config.DestinationPort = nil
			entrySet.Transport.State.DestinationPort = nil
		}
	}

	if aclType == ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV4 {
		if entrySet.Ipv4 == nil {
			entrySet.Ipv4 = &ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv4{}
		}
		ygot.BuildEmptyTree(entrySet.Ipv4)
		for ruleKey := range ruleData.Field {
			if "IP_PROTOCOL" == ruleKey {
				ipProto, _ := strconv.ParseInt(ruleData.Get(ruleKey), 10, 64)
				protocolVal := getIpProtocol(ipProto)
				entrySet.Ipv4.Config.Protocol, _ = entrySet.Ipv4.Config.To_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv4_Config_Protocol_Union(protocolVal)
				entrySet.Ipv4.State.Protocol, _ = entrySet.Ipv4.State.To_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv4_State_Protocol_Union(protocolVal)
			} else if "DSCP" == ruleKey {
				var dscp uint8
				dscpData, _ := strconv.ParseInt(ruleData.Get(ruleKey), 10, 64)
				dscp = uint8(dscpData)
				entrySet.Ipv4.Config.Dscp = &dscp
				entrySet.Ipv4.State.Dscp = &dscp
			} else if "SRC_IP" == ruleKey {
				addr := ruleData.Get(ruleKey)
				entrySet.Ipv4.Config.SourceAddress = &addr
				entrySet.Ipv4.State.SourceAddress = &addr
			} else if "DST_IP" == ruleKey {
				addr := ruleData.Get(ruleKey)
				entrySet.Ipv4.Config.DestinationAddress = &addr
				entrySet.Ipv4.State.DestinationAddress = &addr
			} else if "ICMP_TYPE" == ruleKey {
				num, err := strconv.ParseUint(ruleData.Get(ruleKey), 10, 8)
				if err != nil {
					log.Warningf("acl %s rule %s: bad ICMP_TYPE %q: %v", aclName, ruleName, ruleData.Get(ruleKey), err)
					continue
				}
				aclIcmpv4EmptyTree(entrySet.Ipv4)
				t := uint8(num)
				entrySet.Ipv4.Icmpv4.Config.Type = &t
				entrySet.Ipv4.Icmpv4.State.Type = &t
			} else if "ICMP_CODE" == ruleKey {
				num, err := strconv.ParseUint(ruleData.Get(ruleKey), 10, 8)
				if err != nil {
					log.Warningf("acl %s rule %s: bad ICMP_CODE %q: %v", aclName, ruleName, ruleData.Get(ruleKey), err)
					continue
				}
				aclIcmpv4EmptyTree(entrySet.Ipv4)
				c := uint8(num)
				entrySet.Ipv4.Icmpv4.Config.Code = &c
				entrySet.Ipv4.Icmpv4.State.Code = &c
			}
		}
	} else if aclType == ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV6 {
		if entrySet.Ipv6 == nil {
			entrySet.Ipv6 = &ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv6{}
		}
		ygot.BuildEmptyTree(entrySet.Ipv6)
		for ruleKey := range ruleData.Field {
			if "IP_PROTOCOL" == ruleKey {
				ipProto, _ := strconv.ParseInt(ruleData.Get(ruleKey), 10, 64)
				protocolVal := getIpProtocol(ipProto)
				entrySet.Ipv6.Config.Protocol, _ = entrySet.Ipv6.Config.To_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv6_Config_Protocol_Union(protocolVal)
				entrySet.Ipv6.State.Protocol, _ = entrySet.Ipv6.State.To_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv6_State_Protocol_Union(protocolVal)
			} else if "DSCP" == ruleKey {
				var dscp uint8
				dscpData, _ := strconv.ParseInt(ruleData.Get(ruleKey), 10, 64)
				dscp = uint8(dscpData)
				entrySet.Ipv6.Config.Dscp = &dscp
				entrySet.Ipv6.State.Dscp = &dscp
			} else if "SRC_IPV6" == ruleKey {
				addr := ruleData.Get(ruleKey)
				entrySet.Ipv6.Config.SourceAddress = &addr
				entrySet.Ipv6.State.SourceAddress = &addr
			} else if "DST_IPV6" == ruleKey {
				addr := ruleData.Get(ruleKey)
				entrySet.Ipv6.Config.DestinationAddress = &addr
				entrySet.Ipv6.State.DestinationAddress = &addr
			} else if "ICMPV6_TYPE" == ruleKey {
				// Reverse-map the numeric ICMPv6 type back to the standard
				// openconfig-icmpv6-types identity.
				num, err := strconv.ParseUint(ruleData.Get(ruleKey), 10, 8)
				if err != nil {
					log.Warningf("acl %s rule %s: bad ICMPV6_TYPE %q: %v", aclName, ruleName, ruleData.Get(ruleKey), err)
					continue
				}
				// A numeric ICMPv6 type outside the standard openconfig-icmpv6-types
				// identity set has no identityref value to represent it, so it is
				// enforced in the data plane but cannot be surfaced here.
				if id := getIcmpv6TypeId(uint8(num)); id != ocbinds.OpenconfigIcmpv6Types_TYPE_UNSET {
					aclIcmpv6EmptyTree(entrySet.Ipv6)
					entrySet.Ipv6.Icmpv6.Config.Type = id
					entrySet.Ipv6.Icmpv6.State.Type = id
				} else {
					log.Warningf("acl %s rule %s: ICMPV6_TYPE %d has no openconfig-icmpv6-types identity; omitted from GET", aclName, ruleName, num)
				}
			} else if "ICMPV6_CODE" == ruleKey {
				// A code number only names an identity alongside its type, so
				// read ICMPV6_TYPE straight from the rule: the iteration order
				// over the fields is undefined, and the type may not have been
				// seen yet.
				num, err := strconv.ParseUint(ruleData.Get(ruleKey), 10, 8)
				if err != nil {
					log.Warningf("acl %s rule %s: bad ICMPV6_CODE %q: %v", aclName, ruleName, ruleData.Get(ruleKey), err)
					continue
				}
				typeNum, err := strconv.ParseUint(ruleData.Get("ICMPV6_TYPE"), 10, 8)
				if err != nil {
					log.Warningf("acl %s rule %s: ICMPV6_CODE %d has no usable ICMPV6_TYPE to resolve it against; omitted from GET", aclName, ruleName, num)
					continue
				}
				if id := getIcmpv6CodeId(getIcmpv6TypeId(uint8(typeNum)), uint8(num)); id != ocbinds.OpenconfigIcmpv6Types_CODE_UNSET {
					aclIcmpv6EmptyTree(entrySet.Ipv6)
					entrySet.Ipv6.Icmpv6.Config.Code = id
					entrySet.Ipv6.Icmpv6.State.Code = id
				} else {
					log.Warningf("acl %s rule %s: ICMPV6_TYPE %d code %d has no openconfig-icmpv6-types identity; omitted from GET", aclName, ruleName, typeNum, num)
				}
			}
		}
	} else if aclType == ocbinds.OpenconfigAcl_ACL_TYPE_ACL_L2 {
		ygot.BuildEmptyTree(entrySet.L2)
		for ruleKey := range ruleData.Field {
			if "ETHER_TYPE" == ruleKey {
				ethType, _ := strconv.ParseUint(strings.Replace(ruleData.Get(ruleKey), "0x", "", -1), 16, 32)
				ethertype := getL2EtherType(ethType)
				entrySet.L2.Config.Ethertype, _ = entrySet.L2.Config.To_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_L2_Config_Ethertype_Union(ethertype)
				entrySet.L2.State.Ethertype, _ = entrySet.L2.State.To_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_L2_State_Ethertype_Union(ethertype)
			}
		}
	}
}

func (app *AclApp) convertInternalToOCAclRuleBinding(aclKey string, ruleKey string, priority uint32, seqId int64, direction string, aclSet ygot.GoStruct, entrySet ygot.GoStruct) {
	if seqId == -1 {
		seqId = int64(MAX_PRIORITY - priority)
	}

	pkts, octets := app.aclRuleCounters(aclKey, ruleKey)
	var ruleId uint32 = uint32(seqId)

	if direction == "INGRESS" {
		var ingressEntrySet *ocbinds.OpenconfigAcl_Acl_Interfaces_Interface_IngressAclSets_IngressAclSet_AclEntries_AclEntry
		var ok bool
		if entrySet == nil {
			ingressAclSet := aclSet.(*ocbinds.OpenconfigAcl_Acl_Interfaces_Interface_IngressAclSets_IngressAclSet)
			if ingressEntrySet, ok = ingressAclSet.AclEntries.AclEntry[ruleId]; !ok {
				ingressEntrySet, _ = ingressAclSet.AclEntries.NewAclEntry(ruleId)
			}
		} else {
			ingressEntrySet = entrySet.(*ocbinds.OpenconfigAcl_Acl_Interfaces_Interface_IngressAclSets_IngressAclSet_AclEntries_AclEntry)
		}
		if ingressEntrySet != nil {
			ygot.BuildEmptyTree(ingressEntrySet)
			ingressEntrySet.State.SequenceId = &ruleId
			ingressEntrySet.State.MatchedPackets = &pkts
			ingressEntrySet.State.MatchedOctets = &octets
		}
	} else if direction == "EGRESS" {
		var egressEntrySet *ocbinds.OpenconfigAcl_Acl_Interfaces_Interface_EgressAclSets_EgressAclSet_AclEntries_AclEntry
		var ok bool
		if entrySet == nil {
			egressAclSet := aclSet.(*ocbinds.OpenconfigAcl_Acl_Interfaces_Interface_EgressAclSets_EgressAclSet)
			if egressEntrySet, ok = egressAclSet.AclEntries.AclEntry[ruleId]; !ok {
				egressEntrySet, _ = egressAclSet.AclEntries.NewAclEntry(ruleId)
			}
		} else {
			egressEntrySet = entrySet.(*ocbinds.OpenconfigAcl_Acl_Interfaces_Interface_EgressAclSets_EgressAclSet_AclEntries_AclEntry)
		}
		if egressEntrySet != nil {
			ygot.BuildEmptyTree(egressEntrySet)
			egressEntrySet.State.SequenceId = &ruleId
			egressEntrySet.State.MatchedPackets = &pkts
			egressEntrySet.State.MatchedOctets = &octets
		}
	}
}

func (app *AclApp) convertInternalToOCAclBinding(d *db.DB, aclName string, intfId string, direction string, intfAclSet ygot.GoStruct) error {
	var err error
	if _, ok := app.aclTableMap[aclName]; !ok {
		aclEntry, err1 := d.GetEntry(app.aclTs, db.Key{Comp: []string{aclName}})
		if err1 != nil {
			return err1
		}
		if !contains(aclEntry.GetList("ports"), intfId) {
			return tlerr.InvalidArgs("Acl %s not binded with %s", aclName, intfId)
		}
	}

	if _, ok := app.ruleTableMap[aclName]; !ok {
		ruleKeys, _ := d.GetKeys(app.ruleTs)
		for i, _ := range ruleKeys {
			rulekey := ruleKeys[i]
			// Rulekey has two keys, first aclkey and second rulename
			if rulekey.Get(0) == aclName && rulekey.Get(1) != "DEFAULT_RULE" {
				seqId, _ := strconv.Atoi(strings.Replace(rulekey.Get(1), "RULE_", "", 1))
				app.convertInternalToOCAclRuleBinding(aclName, rulekey.Get(1), 0, int64(seqId), direction, intfAclSet, nil)
			}
		}
	} else {
		for ruleName := range app.ruleTableMap[aclName] {
			if ruleName != "DEFAULT_RULE" {
				seqId, _ := strconv.Atoi(strings.Replace(ruleName, "RULE_", "", 1))
				app.convertInternalToOCAclRuleBinding(aclName, ruleName, 0, int64(seqId), direction, intfAclSet, nil)
			}
		}
	}

	return err
}

func (app *AclApp) getAllBindingsInfo(d *db.DB) error {
	var err error
	acl := app.getAppRootObject()
	if len(app.aclTableMap) == 0 {
		aclKeys, _ := d.GetKeys(app.aclTs)
		for i, _ := range aclKeys {
			aclEntry, _ := d.GetEntry(app.aclTs, aclKeys[i])
			app.aclTableMap[(aclKeys[i]).Get(0)] = aclEntry
		}
	}
	var interfaces []string
	for aclName := range app.aclTableMap {
		aclData := app.aclTableMap[aclName]
		if len(aclData.Get("ports@")) > 0 {
			aclIntfs := aclData.GetList("ports")
			for i, _ := range aclIntfs {
				if !contains(interfaces, aclIntfs[i]) && aclIntfs[i] != "" {
					interfaces = append(interfaces, aclIntfs[i])
				}
			}
		}
	}

	for _, intfId := range interfaces {
		var intfData *ocbinds.OpenconfigAcl_Acl_Interfaces_Interface
		intfData, ok := acl.Interfaces.Interface[intfId]
		if !ok {
			intfData, _ = acl.Interfaces.NewInterface(intfId)
		}
		ygot.BuildEmptyTree(intfData)
		err = app.getAclBindingInfoForInterfaceData(d, intfData, intfId, "INGRESS")
		err = app.getAclBindingInfoForInterfaceData(d, intfData, intfId, "EGRESS")
	}
	return err
}

func (app *AclApp) getAclBindingInfoForInterfaceData(d *db.DB, intfData *ocbinds.OpenconfigAcl_Acl_Interfaces_Interface, intfId string, direction string) error {
	var err error
	if intfData != nil {
		intfData.Config.Id = intfData.Id
		intfData.State.Id = intfData.Id
	}
	if direction == "INGRESS" {
		if intfData.IngressAclSets != nil && len(intfData.IngressAclSets.IngressAclSet) > 0 {
			for ingressAclSetKey, _ := range intfData.IngressAclSets.IngressAclSet {
				aclKey := getAclKeyStrFromOCKey(ingressAclSetKey.SetName, ingressAclSetKey.Type)
				ocSetName := ingressAclSetKey.SetName

				ingressAclSet := intfData.IngressAclSets.IngressAclSet[ingressAclSetKey]
				if ingressAclSet != nil && ingressAclSet.AclEntries != nil && len(ingressAclSet.AclEntries.AclEntry) > 0 {
					for seqId, _ := range ingressAclSet.AclEntries.AclEntry {
						rulekey := "RULE_" + strconv.Itoa(int(seqId))
						entrySet := ingressAclSet.AclEntries.AclEntry[seqId]
						_, err := d.GetEntry(app.ruleTs, db.Key{Comp: []string{aclKey, rulekey}})
						if err != nil {
							return err
						}
						app.convertInternalToOCAclRuleBinding(aclKey, rulekey, 0, int64(seqId), direction, nil, entrySet)
					}
				} else {
					ygot.BuildEmptyTree(ingressAclSet)
					ingressAclSet.Config = &ocbinds.OpenconfigAcl_Acl_Interfaces_Interface_IngressAclSets_IngressAclSet_Config{SetName: &ocSetName, Type: ingressAclSetKey.Type}
					ingressAclSet.State = &ocbinds.OpenconfigAcl_Acl_Interfaces_Interface_IngressAclSets_IngressAclSet_State{SetName: &ocSetName, Type: ingressAclSetKey.Type}
					err = app.convertInternalToOCAclBinding(d, aclKey, intfId, direction, ingressAclSet)
				}
			}
		} else {
			err = app.findAndGetAclBindingInfoForInterfaceData(d, intfId, direction, intfData)
		}
	} else if direction == "EGRESS" {
		if intfData.EgressAclSets != nil && len(intfData.EgressAclSets.EgressAclSet) > 0 {
			for egressAclSetKey, _ := range intfData.EgressAclSets.EgressAclSet {
				aclKey := getAclKeyStrFromOCKey(egressAclSetKey.SetName, egressAclSetKey.Type)
				ocSetName := egressAclSetKey.SetName

				egressAclSet := intfData.EgressAclSets.EgressAclSet[egressAclSetKey]
				if egressAclSet != nil && egressAclSet.AclEntries != nil && len(egressAclSet.AclEntries.AclEntry) > 0 {
					for seqId, _ := range egressAclSet.AclEntries.AclEntry {
						rulekey := "RULE_" + strconv.Itoa(int(seqId))
						entrySet := egressAclSet.AclEntries.AclEntry[seqId]
						_, err := d.GetEntry(app.ruleTs, db.Key{Comp: []string{aclKey, rulekey}})
						if err != nil {
							return err
						}
						app.convertInternalToOCAclRuleBinding(aclKey, rulekey, 0, int64(seqId), direction, nil, entrySet)
					}
				} else {
					ygot.BuildEmptyTree(egressAclSet)
					egressAclSet.Config = &ocbinds.OpenconfigAcl_Acl_Interfaces_Interface_EgressAclSets_EgressAclSet_Config{SetName: &ocSetName, Type: egressAclSetKey.Type}
					egressAclSet.State = &ocbinds.OpenconfigAcl_Acl_Interfaces_Interface_EgressAclSets_EgressAclSet_State{SetName: &ocSetName, Type: egressAclSetKey.Type}
					err = app.convertInternalToOCAclBinding(d, aclKey, intfId, direction, egressAclSet)
				}
			}
		} else {
			err = app.findAndGetAclBindingInfoForInterfaceData(d, intfId, direction, intfData)
		}
	} else {
		log.Error("Unknown direction")
	}
	return err
}

func (app *AclApp) findAndGetAclBindingInfoForInterfaceData(d *db.DB, intfId string, direction string, intfData *ocbinds.OpenconfigAcl_Acl_Interfaces_Interface) error {
	var err error
	if len(app.aclTableMap) == 0 {
		aclKeys, _ := d.GetKeys(app.aclTs)
		for i, _ := range aclKeys {
			aclEntry, _ := d.GetEntry(app.aclTs, aclKeys[i])
			app.aclTableMap[aclKeys[i].Get(0)] = aclEntry
		}
	}

	for aclName, _ := range app.aclTableMap {
		aclData := app.aclTableMap[aclName]
		aclIntfs := aclData.GetList("ports")
		aclType := aclData.Get(ACL_TYPE)
		var aclOrigType ocbinds.E_OpenconfigAcl_ACL_TYPE
		if SONIC_ACL_TYPE_IPV4 == aclType {
			aclOrigType = ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV4
		} else if SONIC_ACL_TYPE_IPV6 == aclType {
			aclOrigType = ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV6
		} else if SONIC_ACL_TYPE_L2 == aclType {
			aclOrigType = ocbinds.OpenconfigAcl_ACL_TYPE_ACL_L2
		}

		if contains(aclIntfs, intfId) && direction == aclData.Get("stage") {
			ocSetName := ocAclSetNameFromTable(aclData, aclName, aclType)
			if direction == "INGRESS" {
				if intfData.IngressAclSets != nil {
					aclSetKey := ocbinds.OpenconfigAcl_Acl_Interfaces_Interface_IngressAclSets_IngressAclSet_Key{SetName: ocSetName, Type: aclOrigType}
					ingressAclSet, ok := intfData.IngressAclSets.IngressAclSet[aclSetKey]
					if !ok {
						ingressAclSet, _ = intfData.IngressAclSets.NewIngressAclSet(ocSetName, aclOrigType)
						ygot.BuildEmptyTree(ingressAclSet)
						ingressAclSet.Config = &ocbinds.OpenconfigAcl_Acl_Interfaces_Interface_IngressAclSets_IngressAclSet_Config{SetName: &ocSetName, Type: aclOrigType}
						ingressAclSet.State = &ocbinds.OpenconfigAcl_Acl_Interfaces_Interface_IngressAclSets_IngressAclSet_State{SetName: &ocSetName, Type: aclOrigType}
					}
					err = app.convertInternalToOCAclBinding(d, aclName, intfId, direction, ingressAclSet)
					if err != nil {
						return err
					}
				}
			} else if direction == "EGRESS" {
				if intfData.EgressAclSets != nil {
					aclSetKey := ocbinds.OpenconfigAcl_Acl_Interfaces_Interface_EgressAclSets_EgressAclSet_Key{SetName: ocSetName, Type: aclOrigType}
					egressAclSet, ok := intfData.EgressAclSets.EgressAclSet[aclSetKey]
					if !ok {
						egressAclSet, _ = intfData.EgressAclSets.NewEgressAclSet(ocSetName, aclOrigType)
						ygot.BuildEmptyTree(egressAclSet)
						egressAclSet.Config = &ocbinds.OpenconfigAcl_Acl_Interfaces_Interface_EgressAclSets_EgressAclSet_Config{SetName: &ocSetName, Type: aclOrigType}
						egressAclSet.State = &ocbinds.OpenconfigAcl_Acl_Interfaces_Interface_EgressAclSets_EgressAclSet_State{SetName: &ocSetName, Type: aclOrigType}
					}
					err = app.convertInternalToOCAclBinding(d, aclName, intfId, direction, egressAclSet)
					if err != nil {
						return err
					}
				}
			}
		}
	}
	return err
}

/*func (app *AclApp) isInterfaceBindWithACL(d *db.DB, intfId string) bool {
	var isFound bool = false

	if len(app.aclTableMap) == 0 {
		aclKeys, _ := d.GetKeys(app.aclTs)
		for i, _ := range aclKeys {
			aclEntry, _ := d.GetEntry(app.aclTs, aclKeys[i])
			app.aclTableMap[(aclKeys[i]).Get(0)] = aclEntry
		}
	}

	var interfaces []string
	for aclName := range app.aclTableMap {
		aclData := app.aclTableMap[aclName]
		if len(aclData.Get("ports@")) > 0 {
			aclIntfs := aclData.GetList("ports")
			for i, _ := range aclIntfs {
				if !contains(interfaces, aclIntfs[i]) && aclIntfs[i] != "" {
					interfaces = append(interfaces, aclIntfs[i])
				}
			}
		}
	}

	isFound = contains(interfaces, intfId)
	return isFound
}*/

func (app *AclApp) handleBindingsDeletion(d *db.DB) error {
	var err error

	acl := app.getAppRootObject()
	aclKeys, _ := d.GetKeys(app.aclTs)
	for i, _ := range aclKeys {
		aclEntry, _ := d.GetEntry(app.aclTs, aclKeys[i])
		var isRequestedAclFound = false
		if len(aclEntry.GetList("ports")) > 0 {
			if isSubtreeRequest(app.pathInfo.Template, "/openconfig-acl:acl/interfaces/interface{}") {
				direction := aclEntry.Get("stage")
				if isSubtreeRequest(app.pathInfo.Template, "/openconfig-acl:acl/interfaces/interface{}/ingress-acl-sets") && direction != "INGRESS" {
					return tlerr.InvalidArgs("Acl %s is not Ingress", aclKeys[i].Get(0))
				}
				if isSubtreeRequest(app.pathInfo.Template, "/openconfig-acl:acl/interfaces/interface{}/egress-acl-sets") && direction != "EGRESS" {
					return tlerr.InvalidArgs("Acl %s is not Egress", aclKeys[i].Get(0))
				}
				for intfId := range acl.Interfaces.Interface {
					_, acltype := getAclKeysFromStrKey(aclKeys[i].Get(0), aclEntry.Get("type"))
					intfData := acl.Interfaces.Interface[intfId]
					if isSubtreeRequest(app.pathInfo.Template, "/openconfig-acl:acl/interfaces/interface{}/ingress-acl-sets/ingress-acl-set{}{}") {
						for k := range intfData.IngressAclSets.IngressAclSet {
							if getAclKeyStrFromOCKey(k.SetName, k.Type) == aclKeys[i].Get(0) {
								if acltype == k.Type {
									isRequestedAclFound = true
								} else {
									return tlerr.InvalidArgs("Acl Type is not matching")
								}
							} else {
								goto SkipDBProcessing
							}
						}
					} else if isSubtreeRequest(app.pathInfo.Template, "/openconfig-acl:acl/interfaces/interface{}/egress-acl-sets/egress-acl-set{}{}") {
						for k := range intfData.EgressAclSets.EgressAclSet {
							if getAclKeyStrFromOCKey(k.SetName, k.Type) == aclKeys[i].Get(0) {
								if acltype == k.Type {
									isRequestedAclFound = true
								} else {
									return tlerr.InvalidArgs("Acl Type is not matching")
								}
							} else {
								goto SkipDBProcessing
							}
						}
					}
					intfs := aclEntry.GetList("ports")
					intfs = removeElement(intfs, intfId)
					aclEntry.SetList("ports", intfs)
					err = d.SetEntry(app.aclTs, aclKeys[i], aclEntry)
					if err != nil {
						return err
					}
					// If last interface removed, then remove stage field also
					if len(intfs) == 0 {
						aclEntry.Remove("stage")
					}
				}
			SkipDBProcessing:
			} else {
				aclEntry.Remove("stage")
				aclEntry.SetList("ports", []string{})
				err = d.SetEntry(app.aclTs, aclKeys[i], aclEntry)
				if err != nil {
					return err
				}
			}
		}
		if isRequestedAclFound {
			break
		}
	}

	return err
}

/********************   CREATE related    *******************************/
func (app *AclApp) convertOCAclsToInternal() {
	acl := app.getAppRootObject()
	if acl != nil {
		app.aclTableMap = make(map[string]db.Value)
		if acl.AclSets != nil && len(acl.AclSets.AclSet) > 0 {
			for aclSetKey, _ := range acl.AclSets.AclSet {
				aclSet := acl.AclSets.AclSet[aclSetKey]
				aclKey := getAclKeyStrFromOCKey(aclSetKey.Name, aclSetKey.Type)
				app.aclTableMap[aclKey] = db.Value{Field: map[string]string{}}

				if aclSet.Config != nil {
					if aclSet.Config.Type == ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV4 {
						app.aclTableMap[aclKey].Field[ACL_TYPE] = SONIC_ACL_TYPE_IPV4
					} else if aclSet.Config.Type == ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV6 {
						app.aclTableMap[aclKey].Field[ACL_TYPE] = SONIC_ACL_TYPE_IPV6
					} else if aclSet.Config.Type == ocbinds.OpenconfigAcl_ACL_TYPE_ACL_L2 {
						app.aclTableMap[aclKey].Field[ACL_TYPE] = SONIC_ACL_TYPE_L2
					}

					// openconfig-acl-ext: a control-plane / management ACL
					// (restrict-access) becomes a SONiC CTRLPLANE table bound to
					// management services. The acl-set key still carries the
					// ACL_IPV4/ACL_IPV6 OC type (used to interpret rule fields and
					// to reconstruct the OC type on read-back from the table name
					// suffix); only the persisted ACL_TABLE:type is overridden.
					if aclSet.Config.ControlPlane != nil && *aclSet.Config.ControlPlane {
						app.aclTableMap[aclKey].Field[ACL_TYPE] = SONIC_ACL_TYPE_CTRLPLANE
						if len(aclSet.Config.Services) > 0 {
							val := app.aclTableMap[aclKey]
							(&val).SetList(ACL_SERVICES, aclSet.Config.Services)
							app.aclTableMap[aclKey] = val
						}
					}

					if aclSet.Config.Description != nil && len(*aclSet.Config.Description) > 0 {
						app.aclTableMap[aclKey].Field[ACL_DESCRIPTION] = *aclSet.Config.Description
					}
					app.aclTableMap[aclKey].Field[ACL_OPENCONFIG_SET_NAME] = aclSetKey.Name
				}
			}
		}
	}
}

func (app *AclApp) convertOCAclRulesToInternal(d *db.DB, opcode int) error {
	acl := app.getAppRootObject()
	if acl != nil {
		app.ruleTableMap = make(map[string]map[string]db.Value)
		if acl.AclSets != nil && len(acl.AclSets.AclSet) > 0 {
			for aclSetKey, _ := range acl.AclSets.AclSet {
				aclSet := acl.AclSets.AclSet[aclSetKey]
				aclKey := getAclKeyStrFromOCKey(aclSetKey.Name, aclSetKey.Type)
				app.ruleTableMap[aclKey] = make(map[string]db.Value)

				// Control-plane (CTRLPLANE) ACLs are enforced by caclmgrd, which
				// applies its own default behavior per management service; do not
				// inject the data-plane default-deny rule for them.
				isControlPlane := aclSet.Config != nil && aclSet.Config.ControlPlane != nil && *aclSet.Config.ControlPlane

				if aclSet.AclEntries != nil {
					for seqId, _ := range aclSet.AclEntries.AclEntry {
						entrySet := aclSet.AclEntries.AclEntry[seqId]
						ruleName := "RULE_" + strconv.Itoa(int(seqId))
						app.ruleTableMap[aclKey][ruleName] = db.Value{Field: map[string]string{}}
						portSets, err := convertOCAclRuleToInternalAclRule(d, app.ruleTableMap[aclKey][ruleName], seqId, aclKey, aclSet.Type, entrySet, opcode)
						if err != nil {
							return err
						}
						if portSets != nil {
							if app.rulePortSetMap[aclKey] == nil {
								app.rulePortSetMap[aclKey] = make(map[string]*rulePortSetInfo)
							}
							app.rulePortSetMap[aclKey][ruleName] = portSets
						}
						if aclSet.Type == ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV6 && ruleHasIcmpv6CodeGroup(entrySet) {
							if app.ruleIcmpv6CodeGroup[aclKey] == nil {
								app.ruleIcmpv6CodeGroup[aclKey] = make(map[string]bool)
							}
							app.ruleIcmpv6CodeGroup[aclKey][ruleName] = true
						}
						// caclmgrd uses PACKET_ACTION directly as the iptables
						// jump target (-j ACCEPT/DROP), so a control-plane "accept"
						// must be ACCEPT rather than the data-plane SAI "FORWARD".
						if isControlPlane && app.ruleTableMap[aclKey][ruleName].Field["PACKET_ACTION"] == "FORWARD" {
							app.ruleTableMap[aclKey][ruleName].Field["PACKET_ACTION"] = "ACCEPT"
						}
					}
				}

				yangPathStr, _ := getYangPathFromUri(app.pathInfo.Path)
				if !isControlPlane && yangPathStr != "/openconfig-acl:acl/acl-sets/acl-set/acl-entries" && yangPathStr != "/openconfig-acl:acl/acl-sets/acl-set/acl-entries/acl-entry" {
					app.createDefaultDenyAclRule(d, aclKey, app.ruleTableMap[aclKey])
				}
			}
		}
	}

	return nil
}

func (app *AclApp) convertOCAclBindingsToInternal() {
	aclObj := app.getAppRootObject()

	if aclObj.Interfaces != nil && len(aclObj.Interfaces.Interface) > 0 {
		aclInterfacesMap := make(map[string][]string)
		// Below code assumes that an ACL can be either INGRESS or EGRESS but not both.
		for intfId, _ := range aclObj.Interfaces.Interface {
			intf := aclObj.Interfaces.Interface[intfId]
			if intf != nil {
				if intf.IngressAclSets != nil && len(intf.IngressAclSets.IngressAclSet) > 0 {
					for inAclKey, _ := range intf.IngressAclSets.IngressAclSet {
						aclName := getAclKeyStrFromOCKey(inAclKey.SetName, inAclKey.Type)
						// TODO: Need to handle Subinterface also
						if intf.InterfaceRef != nil && intf.InterfaceRef.Config.Interface != nil {
							aclInterfacesMap[aclName] = append(aclInterfacesMap[aclName], *intf.InterfaceRef.Config.Interface)
						} else {
							aclInterfacesMap[aclName] = append(aclInterfacesMap[aclName], *intf.Id)
						}
						if _, ok := app.aclTableMap[aclName]; !ok {
							app.aclTableMap[aclName] = db.Value{Field: map[string]string{}}
						}
						app.aclTableMap[aclName].Field[ACL_OPENCONFIG_SET_NAME] = inAclKey.SetName
						app.aclTableMap[aclName].Field["stage"] = "INGRESS"
					}
				}

				if intf.EgressAclSets != nil && len(intf.EgressAclSets.EgressAclSet) > 0 {
					for outAclKey, _ := range intf.EgressAclSets.EgressAclSet {
						aclName := getAclKeyStrFromOCKey(outAclKey.SetName, outAclKey.Type)
						if intf.InterfaceRef != nil && intf.InterfaceRef.Config.Interface != nil {
							aclInterfacesMap[aclName] = append(aclInterfacesMap[aclName], *intf.InterfaceRef.Config.Interface)
						} else {
							aclInterfacesMap[aclName] = append(aclInterfacesMap[aclName], *intf.Id)
						}
						if _, ok := app.aclTableMap[aclName]; !ok {
							app.aclTableMap[aclName] = db.Value{Field: map[string]string{}}
						}
						app.aclTableMap[aclName].Field[ACL_OPENCONFIG_SET_NAME] = outAclKey.SetName
						app.aclTableMap[aclName].Field["stage"] = "EGRESS"
					}
				}
			}
		}
		for k, _ := range aclInterfacesMap {
			val := app.aclTableMap[k]
			(&val).SetList("ports", aclInterfacesMap[k])
		}
	}
}

func (app *AclApp) createDefaultDenyAclRule(d *db.DB, aclName string, rulesInfo map[string]db.Value) {
	existingRuleEntry, err := d.GetEntry(app.ruleTs, db.Key{Comp: []string{aclName, "DEFAULT_RULE"}})
	// If Default Rule already exists, Do not add new Default Rule
	if existingRuleEntry.IsPopulated() && err == nil {
		return
	}
	m := make(map[string]string)
	rulesInfo["DEFAULT_RULE"] = db.Value{Field: m}
	rulesInfo["DEFAULT_RULE"].Field["PRIORITY"] = strconv.FormatInt(int64(MIN_PRIORITY), 10)
	rulesInfo["DEFAULT_RULE"].Field["PACKET_ACTION"] = "DROP"
	rulesInfo["DEFAULT_RULE"].Field["IP_TYPE"] = "ANY"
}

func convertOCAclRuleToInternalAclRule(d *db.DB, ruleData db.Value, seqId uint32, aclName string, aclType ocbinds.E_OpenconfigAcl_ACL_TYPE, rule *ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry, opcode int) (*rulePortSetInfo, error) {
	ruleIndex := seqId
	ruleData.Field["PRIORITY"] = strconv.FormatInt(int64(MAX_PRIORITY-ruleIndex), 10)
	// Rule Description is not supported in Sonic. So commenting this out.
	/*
	   if rule.Config != nil && rule.Config.Description != nil {
	       ruleData.Field["RULE_DESCRIPTION"] = *rule.Config.Description
	   }
	*/

	if ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV4 == aclType {
		convertOCToInternalIPv4(ruleData, aclName, ruleIndex, rule)
	} else if ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV6 == aclType {
		if err := convertOCToInternalIPv6(d, ruleData, aclName, ruleIndex, rule, opcode); err != nil {
			return nil, err
		}
	} else if ocbinds.OpenconfigAcl_ACL_TYPE_ACL_L2 == aclType {
		convertOCToInternalL2(ruleData, aclName, ruleIndex, rule)
	} /*else if ocbinds.OpenconfigAcl_ACL_TYPE_ACL_MIXED == aclType {
	  } */

	portSets, err := convertOCToInternalTransport(d, ruleData, aclName, ruleIndex, rule)
	if err != nil {
		return nil, err
	}
	convertOCToInternalInputInterface(ruleData, aclName, ruleIndex, rule)
	if err := convertOCToInternalInputAction(ruleData, aclName, ruleIndex, rule); err != nil {
		return nil, err
	}

	return portSets, nil
}

func convertOCToInternalL2(ruleData db.Value, aclName string, ruleIndex uint32, rule *ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry) {
	if rule.L2 == nil {
		return
	}
	if rule.L2.Config.Ethertype != nil && util.IsTypeStructPtr(reflect.TypeOf(rule.L2.Config.Ethertype)) {
		ethertypeType := reflect.TypeOf(rule.L2.Config.Ethertype).Elem()
		var b bytes.Buffer
		switch ethertypeType {
		case reflect.TypeOf(ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_L2_Config_Ethertype_Union_E_OpenconfigPacketMatchTypes_ETHERTYPE{}):
			v := (rule.L2.Config.Ethertype).(*ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_L2_Config_Ethertype_Union_E_OpenconfigPacketMatchTypes_ETHERTYPE)
			//ruleData["ETHER_TYPE"] = v.E_OpenconfigPacketMatchTypes_ETHERTYPE.ΛMap()["E_OpenconfigPacketMatchTypes_ETHERTYPE"][int64(v.E_OpenconfigPacketMatchTypes_ETHERTYPE)].Name
			fmt.Fprintf(&b, "0x%0.4x", ETHERTYPE_MAP[v.E_OpenconfigPacketMatchTypes_ETHERTYPE])
			ruleData.Field["ETHER_TYPE"] = b.String()
		case reflect.TypeOf(ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_L2_Config_Ethertype_Union_Uint16{}):
			v := (rule.L2.Config.Ethertype).(*ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_L2_Config_Ethertype_Union_Uint16)
			fmt.Fprintf(&b, "0x%0.4x", v.Uint16)
			ruleData.Field["ETHER_TYPE"] = b.String()
		}
	}
}

// aclIcmpv4EmptyTree ensures the icmpv4 match container and its Config/State
// children exist on an IPv4 ACL entry before its type/code leaves are populated.
// It is called lazily from the ICMP_TYPE/ICMP_CODE branches only, so rules that
// carry no ICMP fields never get an icmpv4 container in their GET output.
//
// ygot.BuildEmptyTree on the parent Ipv4 does not build icmpv4's Config/State in
// the targeted-subtree GET flow (a GET of .../ipv4/icmpv4): the translib
// framework pre-allocates the icmpv4 target container but leaves Config/State
// nil, and BuildEmptyTree only descends into nil children. BuildEmptyTree itself
// only fills nil fields, so calling it here unconditionally is safe whether
// Icmpv4 was just allocated or was pre-allocated empty by the framework; guarding
// it behind the nil check would leave Config/State nil and crash on assignment.
func aclIcmpv4EmptyTree(ipv4 *ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv4) {
	if ipv4.Icmpv4 == nil {
		ipv4.Icmpv4 = &ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv4_Icmpv4{}
	}
	ygot.BuildEmptyTree(ipv4.Icmpv4)
}

// aclIcmpv6EmptyTree is the ICMPv6 counterpart of aclIcmpv4EmptyTree.
func aclIcmpv6EmptyTree(ipv6 *ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv6) {
	if ipv6.Icmpv6 == nil {
		ipv6.Icmpv6 = &ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv6_Icmpv6{}
	}
	// See aclIcmpv4EmptyTree: BuildEmptyTree must run even when the framework
	// pre-allocated an empty Icmpv6 target container (targeted /ipv6/icmpv6 GET).
	ygot.BuildEmptyTree(ipv6.Icmpv6)
}

func convertOCToInternalIPv4(ruleData db.Value, aclName string, ruleIndex uint32, rule *ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry) {
	if rule.Ipv4 == nil {
		return
	}
	// ipv4/config and ipv4/icmpv4 are sibling containers, so a subtree write
	// that only touches icmpv4 leaves Config nil; guard the config-field
	// derefs accordingly.
	if rule.Ipv4.Config != nil {
		if rule.Ipv4.Config.Protocol != nil && util.IsTypeStructPtr(reflect.TypeOf(rule.Ipv4.Config.Protocol)) {
			protocolType := reflect.TypeOf(rule.Ipv4.Config.Protocol).Elem()
			switch protocolType {
			case reflect.TypeOf(ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv4_Config_Protocol_Union_E_OpenconfigPacketMatchTypes_IP_PROTOCOL{}):
				v := (rule.Ipv4.Config.Protocol).(*ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv4_Config_Protocol_Union_E_OpenconfigPacketMatchTypes_IP_PROTOCOL)
				//ruleData["IP_PROTOCOL"] = v.E_OpenconfigPacketMatchTypes_IP_PROTOCOL.ΛMap()["E_OpenconfigPacketMatchTypes_IP_PROTOCOL"][int64(v.E_OpenconfigPacketMatchTypes_IP_PROTOCOL)].Name
				ruleData.Field["IP_PROTOCOL"] = strconv.FormatInt(int64(IP_PROTOCOL_MAP[v.E_OpenconfigPacketMatchTypes_IP_PROTOCOL]), 10)
			case reflect.TypeOf(ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv4_Config_Protocol_Union_Uint8{}):
				v := (rule.Ipv4.Config.Protocol).(*ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv4_Config_Protocol_Union_Uint8)
				ruleData.Field["IP_PROTOCOL"] = strconv.FormatInt(int64(v.Uint8), 10)
			}
		}

		if rule.Ipv4.Config.Dscp != nil {
			ruleData.Field["DSCP"] = strconv.FormatInt(int64(*rule.Ipv4.Config.Dscp), 10)
		}
		if rule.Ipv4.Config.SourceAddress != nil {
			ruleData.Field["SRC_IP"] = *rule.Ipv4.Config.SourceAddress
		}
		if rule.Ipv4.Config.DestinationAddress != nil {
			ruleData.Field["DST_IP"] = *rule.Ipv4.Config.DestinationAddress
		}
	}
	// openconfig-acl-ext ICMPv4 type/code match (SONiC ICMP_TYPE and
	// ICMP_CODE). Both are the numeric values, so no identity map is needed.
	// Restricting ICMP means pairing these with IP_PROTOCOL 1 and a drop
	// action, for example to drop ICMP redirects (type 5).
	if rule.Ipv4.Icmpv4 != nil && rule.Ipv4.Icmpv4.Config != nil {
		if rule.Ipv4.Icmpv4.Config.Type != nil {
			ruleData.Field["ICMP_TYPE"] = strconv.FormatInt(int64(*rule.Ipv4.Icmpv4.Config.Type), 10)
		}
		if rule.Ipv4.Icmpv4.Config.Code != nil {
			ruleData.Field["ICMP_CODE"] = strconv.FormatInt(int64(*rule.Ipv4.Icmpv4.Config.Code), 10)
		}
	}
	ruleData.Field["IP_TYPE"] = "IPV4ANY"
}

func convertOCToInternalIPv6(d *db.DB, ruleData db.Value, aclName string, ruleIndex uint32, rule *ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry, opcode int) error {
	if rule.Ipv6 == nil {
		return nil
	}
	// ipv6/config and ipv6/icmpv6 are sibling containers, so a subtree write
	// that only touches icmpv6 leaves Config nil; guard the config-field
	// derefs accordingly.
	if rule.Ipv6.Config != nil {
		if rule.Ipv6.Config.Protocol != nil && util.IsTypeStructPtr(reflect.TypeOf(rule.Ipv6.Config.Protocol)) {
			protocolType := reflect.TypeOf(rule.Ipv6.Config.Protocol).Elem()
			switch protocolType {
			case reflect.TypeOf(ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv6_Config_Protocol_Union_E_OpenconfigPacketMatchTypes_IP_PROTOCOL{}):
				v := (rule.Ipv6.Config.Protocol).(*ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv6_Config_Protocol_Union_E_OpenconfigPacketMatchTypes_IP_PROTOCOL)
				//ruleData["IP_PROTOCOL"] = v.E_OpenconfigPacketMatchTypes_IP_PROTOCOL.ΛMap()["E_OpenconfigPacketMatchTypes_IP_PROTOCOL"][int64(v.E_OpenconfigPacketMatchTypes_IP_PROTOCOL)].Name
				ruleData.Field["IP_PROTOCOL"] = strconv.FormatInt(int64(IP_PROTOCOL_MAP[v.E_OpenconfigPacketMatchTypes_IP_PROTOCOL]), 10)
			case reflect.TypeOf(ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv6_Config_Protocol_Union_Uint8{}):
				v := (rule.Ipv6.Config.Protocol).(*ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv6_Config_Protocol_Union_Uint8)
				ruleData.Field["IP_PROTOCOL"] = strconv.FormatInt(int64(v.Uint8), 10)
			}
		}

		if rule.Ipv6.Config.Dscp != nil {
			ruleData.Field["DSCP"] = strconv.FormatInt(int64(*rule.Ipv6.Config.Dscp), 10)
		}
		if rule.Ipv6.Config.SourceAddress != nil {
			ruleData.Field["SRC_IPV6"] = *rule.Ipv6.Config.SourceAddress
		}
		if rule.Ipv6.Config.DestinationAddress != nil {
			ruleData.Field["DST_IPV6"] = *rule.Ipv6.Config.DestinationAddress
		}
		if rule.Ipv6.Config.SourceFlowLabel != nil {
			ruleData.Field["SRC_FLOWLABEL"] = strconv.FormatInt(int64(*rule.Ipv6.Config.SourceFlowLabel), 10)
		}
		if rule.Ipv6.Config.DestinationFlowLabel != nil {
			ruleData.Field["DST_FLOWLABEL"] = strconv.FormatInt(int64(*rule.Ipv6.Config.DestinationFlowLabel), 10)
		}
	}
	// openconfig-acl-ext: ICMPv6 type/code match (SONiC ICMPV6_TYPE and
	// ICMPV6_CODE). Both leaves keep the standard openconfig-icmpv6-types
	// identityrefs; map them to the numeric type and code.
	if rule.Ipv6.Icmpv6 != nil && rule.Ipv6.Icmpv6.Config != nil {
		if err := setRuleIcmpv6TypeCode(d, ruleData, aclName, ruleIndex, rule.Ipv6.Icmpv6.Config, opcode); err != nil {
			return err
		}
	}
	ruleData.Field["IP_TYPE"] = "IPV6ANY"
	return nil
}

// setRuleIcmpv6TypeCode maps the icmpv6 type/code match onto the SONiC ACL_RULE
// ICMPV6_TYPE and ICMPV6_CODE fields.
//
// An ICMPv6 code is only meaningful inside a message type, and neither the
// identity tree nor CVL can express that pairing, so it is checked here.
// Without the check a rule could be installed matching a type/code combination
// no packet carries, which matches nothing while reading back as configured.
//
// The check is against the rule the request leaves behind rather than the
// request alone, because CREATE and UPDATE merge into the ACL_RULE row that is
// already there: a request may set the code and leave the type in place, or set
// the type over a code that is already stored. Only REPLACE writes the row
// whole, so only there is the payload the entire rule.
func setRuleIcmpv6TypeCode(d *db.DB, ruleData db.Value, aclName string, ruleIndex uint32,
	config *ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv6_Icmpv6_Config, opcode int) error {

	reqType, reqCode := config.Type, config.Code
	if reqType == ocbinds.OpenconfigIcmpv6Types_TYPE_UNSET && reqCode == ocbinds.OpenconfigIcmpv6Types_CODE_UNSET {
		return nil
	}

	// A per-type group identity (DST_UNREACHABLE_CODE and friends) is checked
	// against the type like a code is, but installs no ICMPV6_CODE: it stands
	// for every code of the type.
	reqCodeInfo, reqCodeOk := ICMPV6_CODE_MAP[reqCode]
	reqGroupType, reqGroupOk := ICMPV6_CODE_GROUP_MAP[reqCode]
	codeType := ocbinds.OpenconfigIcmpv6Types_TYPE_UNSET
	switch {
	case reqCodeOk:
		codeType = reqCodeInfo.typ
	case reqGroupOk:
		codeType = reqGroupType
	case reqCode != ocbinds.OpenconfigIcmpv6Types_CODE_UNSET:
		return tlerr.InvalidArgs(
			"icmpv6 code %s is not a known ICMPv6 code on rule %d of %s",
			icmpv6CodeName(reqCode), ruleIndex, aclName)
	}

	// The type the rule ends up with. A CREATE or UPDATE that names only the
	// code keeps the type already stored, so that is the one to check against.
	effType := reqType
	storedCode, haveStoredCode := uint8(0), false
	if opcode != REPLACE {
		stored, err := d.GetEntry(&db.TableSpec{Name: RULE_TABLE},
			db.Key{Comp: []string{aclName, "RULE_" + strconv.Itoa(int(ruleIndex))}})
		if err == nil && stored.IsPopulated() {
			if effType == ocbinds.OpenconfigIcmpv6Types_TYPE_UNSET {
				if n, perr := strconv.ParseUint(stored.Get("ICMPV6_TYPE"), 10, 8); perr == nil {
					effType = getIcmpv6TypeId(uint8(n))
				}
			}
			if n, perr := strconv.ParseUint(stored.Get("ICMPV6_CODE"), 10, 8); perr == nil {
				storedCode, haveStoredCode = uint8(n), true
			}
		}
	}

	if codeType != ocbinds.OpenconfigIcmpv6Types_TYPE_UNSET {
		// Compare the identity the client named, not its number: the same
		// number is a different code under each type, so a code of the wrong
		// type whose number happens to be valid under the configured one would
		// otherwise be stored as a code the client never asked for.
		if effType == ocbinds.OpenconfigIcmpv6Types_TYPE_UNSET {
			return tlerr.InvalidArgs(
				"icmpv6 code %s needs the icmpv6 type %s to be set on rule %d of %s",
				icmpv6CodeName(reqCode), icmpv6TypeName(codeType), ruleIndex, aclName)
		}
		if codeType != effType {
			return tlerr.InvalidArgs(
				"icmpv6 code %s is a code of type %s, not of the configured type %s, on rule %d of %s",
				icmpv6CodeName(reqCode), icmpv6TypeName(codeType), icmpv6TypeName(effType),
				ruleIndex, aclName)
		}
	} else if haveStoredCode && reqType != ocbinds.OpenconfigIcmpv6Types_TYPE_UNSET {
		// Changing only the type would leave the stored code beside a type it
		// does not belong to, matching a combination no packet carries.
		if getIcmpv6CodeId(effType, storedCode) == ocbinds.OpenconfigIcmpv6Types_CODE_UNSET {
			return tlerr.InvalidArgs(
				"icmpv6 code %d already on rule %d of %s is not a code of type %s; set or delete the code with the type",
				storedCode, ruleIndex, aclName, icmpv6TypeName(effType))
		}
	}

	if reqType != ocbinds.OpenconfigIcmpv6Types_TYPE_UNSET {
		if num, ok := ICMPV6_TYPE_MAP[reqType]; ok {
			ruleData.Field["ICMPV6_TYPE"] = strconv.FormatInt(int64(num), 10)
		}
	}
	if reqCodeOk {
		ruleData.Field["ICMPV6_CODE"] = strconv.FormatInt(int64(reqCodeInfo.num), 10)
	}
	return nil
}

// icmpv6TypeName and icmpv6CodeName render an identity as the name a client
// used, so an error names the value that was rejected rather than its ordinal.
func icmpv6TypeName(t ocbinds.E_OpenconfigIcmpv6Types_TYPE) string {
	if def, ok := t.ΛMap()["E_OpenconfigIcmpv6Types_TYPE"][int64(t)]; ok {
		return def.Name
	}
	return strconv.FormatInt(int64(t), 10)
}

func icmpv6CodeName(c ocbinds.E_OpenconfigIcmpv6Types_CODE) string {
	if def, ok := c.ΛMap()["E_OpenconfigIcmpv6Types_CODE"][int64(c)]; ok {
		return def.Name
	}
	return strconv.FormatInt(int64(c), 10)
}

func ruleHasIcmpv6CodeGroup(rule *ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry) bool {
	if rule.Ipv6 == nil || rule.Ipv6.Icmpv6 == nil || rule.Ipv6.Icmpv6.Config == nil {
		return false
	}
	_, ok := ICMPV6_CODE_GROUP_MAP[rule.Ipv6.Icmpv6.Config.Code]
	return ok
}

// clearStaleRuleIcmpv6Code removes the ICMPv6 code stored on a rule whose
// payload named a per-type code group. The group installs no ICMPV6_CODE, so a
// merge would otherwise leave the stored code filtering.
func (app *AclApp) clearStaleRuleIcmpv6Code(d *db.DB, aclKey, ruleKey string) error {
	if !app.ruleIcmpv6CodeGroup[aclKey][ruleKey] {
		return nil
	}
	existing, err := d.GetEntry(app.ruleTs, asKey(aclKey, ruleKey))
	if err != nil || !(&existing).Has("ICMPV6_CODE") {
		return nil
	}
	return d.DeleteEntryFields(app.ruleTs, asKey(aclKey, ruleKey), createEmptyDbValue("ICMPV6_CODE"))
}

func convertOCToInternalTransport(d *db.DB, ruleData db.Value, aclName string, ruleIndex uint32, rule *ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry) (*rulePortSetInfo, error) {
	if rule.Transport == nil || rule.Transport.Config == nil {
		return nil, nil
	}
	portSets := &rulePortSetInfo{
		srcLiteral: rule.Transport.Config.SourcePort != nil,
		dstLiteral: rule.Transport.Config.DestinationPort != nil,
	}
	if rule.Transport.Config.SourcePort != nil && util.IsTypeStructPtr(reflect.TypeOf(rule.Transport.Config.SourcePort)) {
		sourceportType := reflect.TypeOf(rule.Transport.Config.SourcePort).Elem()
		switch sourceportType {
		case reflect.TypeOf(ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_SourcePort_Union_E_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_SourcePort{}):
			v := (rule.Transport.Config.SourcePort).(*ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_SourcePort_Union_E_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_SourcePort)
			ruleData.Field["L4_SRC_PORT"] = v.E_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_SourcePort.ΛMap()["E_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_SourcePort"][int64(v.E_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_SourcePort)].Name
		case reflect.TypeOf(ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_SourcePort_Union_String{}):
			v := (rule.Transport.Config.SourcePort).(*ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_SourcePort_Union_String)
			ruleData.Field["L4_SRC_PORT_RANGE"] = strings.Replace(v.String, "..", "-", 1)
		case reflect.TypeOf(ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_SourcePort_Union_Uint16{}):
			v := (rule.Transport.Config.SourcePort).(*ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_SourcePort_Union_Uint16)
			ruleData.Field["L4_SRC_PORT"] = strconv.FormatInt(int64(v.Uint16), 10)
		}
	}

	if rule.Transport.Config.DestinationPort != nil && util.IsTypeStructPtr(reflect.TypeOf(rule.Transport.Config.DestinationPort)) {
		destportType := reflect.TypeOf(rule.Transport.Config.DestinationPort).Elem()
		switch destportType {
		case reflect.TypeOf(ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_DestinationPort_Union_E_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_DestinationPort{}):
			v := (rule.Transport.Config.DestinationPort).(*ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_DestinationPort_Union_E_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_DestinationPort)
			ruleData.Field["L4_DST_PORT"] = v.E_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_DestinationPort.ΛMap()["E_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_DestinationPort"][int64(v.E_OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_DestinationPort)].Name
		case reflect.TypeOf(ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_DestinationPort_Union_String{}):
			v := (rule.Transport.Config.DestinationPort).(*ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_DestinationPort_Union_String)
			ruleData.Field["L4_DST_PORT_RANGE"] = strings.Replace(v.String, "..", "-", 1)
		case reflect.TypeOf(ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_DestinationPort_Union_Uint16{}):
			v := (rule.Transport.Config.DestinationPort).(*ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_DestinationPort_Union_Uint16)
			ruleData.Field["L4_DST_PORT"] = strconv.FormatInt(int64(v.Uint16), 10)
		}
	}

	// A port set is resolved here and the rule keeps the port it resolved to:
	// orchagent drops a rule that carries a field it does not know, so the set
	// name cannot travel with it. The reference is recorded separately, in
	// ACL_RULE_PORT_SET.
	if rule.Transport.Config.SourcePortSet != nil {
		if portSets.srcLiteral {
			return nil, tlerr.InvalidArgs(
				"source-port and source-port-set cannot both be set on rule %d of %s", ruleIndex, aclName)
		}
		portVal, err := portSetMatch(d, *rule.Transport.Config.SourcePortSet)
		if err != nil {
			return nil, err
		}
		setRulePortMatch(ruleData, true, portVal)
		portSets.srcSet = *rule.Transport.Config.SourcePortSet
	}

	if rule.Transport.Config.DestinationPortSet != nil {
		if portSets.dstLiteral {
			return nil, tlerr.InvalidArgs(
				"destination-port and destination-port-set cannot both be set on rule %d of %s", ruleIndex, aclName)
		}
		portVal, err := portSetMatch(d, *rule.Transport.Config.DestinationPortSet)
		if err != nil {
			return nil, err
		}
		setRulePortMatch(ruleData, false, portVal)
		portSets.dstSet = *rule.Transport.Config.DestinationPortSet
	}

	var tcpFlags uint32 = 0x00
	if len(rule.Transport.Config.TcpFlags) > 0 {
		for _, flag := range rule.Transport.Config.TcpFlags {
			switch flag {
			case ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_FIN:
				tcpFlags |= 0x01
			case ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_SYN:
				tcpFlags |= 0x02
			case ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_RST:
				tcpFlags |= 0x04
			case ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_PSH:
				tcpFlags |= 0x08
			case ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_ACK:
				tcpFlags |= 0x10
			case ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_URG:
				tcpFlags |= 0x20
			case ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_ECE:
				tcpFlags |= 0x40
			case ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_CWR:
				tcpFlags |= 0x80
			}
		}
		var b bytes.Buffer
		fmt.Fprintf(&b, "0x%0.2x/0x%0.2x", tcpFlags, tcpFlags)
		ruleData.Field["TCP_FLAGS"] = b.String()
	}

	return portSets, nil
}

func convertOCToInternalInputInterface(ruleData db.Value, aclName string, ruleIndex uint32, rule *ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry) {
	if rule.InputInterface != nil && rule.InputInterface.InterfaceRef != nil {
		ruleData.Field["IN_PORTS"] = *rule.InputInterface.InterfaceRef.Config.Interface
	}
}

func convertOCToInternalInputAction(ruleData db.Value, aclName string, ruleIndex uint32, rule *ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry) error {
	if rule.Actions != nil && rule.Actions.Config != nil {
		// ACL_RULE has no logging action, so LOG_SYSLOG cannot be applied.
		// Refuse it rather than accept a rule that silently does not log.
		if rule.Actions.Config.LogAction == ocbinds.OpenconfigAcl_LOG_ACTION_LOG_SYSLOG {
			return tlerr.NotSupported(
				"log-action LOG_SYSLOG is not supported on rule %d of %s; only LOG_NONE is", ruleIndex, aclName)
		}

		switch rule.Actions.Config.ForwardingAction {
		case ocbinds.OpenconfigAcl_FORWARDING_ACTION_ACCEPT:
			ruleData.Field["PACKET_ACTION"] = "FORWARD"
		case ocbinds.OpenconfigAcl_FORWARDING_ACTION_DROP, ocbinds.OpenconfigAcl_FORWARDING_ACTION_REJECT:
			ruleData.Field["PACKET_ACTION"] = "DROP"
		default:
		}
	}
	return nil
}

func (app *AclApp) handleRuleFieldsDeletion(d *db.DB, aclKey string, ruleKey string) error {
	var err error

	ruleEntry, err := d.GetEntry(app.ruleTs, asKey(aclKey, ruleKey))
	if err != nil {
		return err
	}
	nodeInfo, err := getTargetNodeYangSchema(app.pathInfo.Path, (*app.ygotRoot).(*ocbinds.Device))
	if err != nil {
		return err
	}
	// Port set references live in their own table, so they are cleared after the
	// rule fields have been written.
	var clearRefs []string
	if nodeInfo.IsLeaf() {
		switch nodeInfo.Name {
		case "description":
			(&ruleEntry).Remove("RULE_DESCRIPTION")
		// L2
		case "ethertype":
			(&ruleEntry).Remove("ETHER_TYPE")
			// IPv4/IPv6
		case "source-address":
			if strings.Contains(app.pathInfo.Path, "ipv4/config") {
				(&ruleEntry).Remove("SRC_IP")
			} else if strings.Contains(app.pathInfo.Path, "ipv6/config") {
				(&ruleEntry).Remove("SRC_IPV6")
			}
		case "destination-address":
			if strings.Contains(app.pathInfo.Path, "ipv4/config") {
				(&ruleEntry).Remove("DST_IP")
			} else if strings.Contains(app.pathInfo.Path, "ipv6/config") {
				(&ruleEntry).Remove("DST_IPV6")
			}
		case "dscp":
			(&ruleEntry).Remove("DSCP")
		case "protocol":
			(&ruleEntry).Remove("IP_PROTOCOL")
			// ICMP type/code. The leaf is named the same under icmpv4 and
			// icmpv6, so the path decides which SONiC field it maps to.
		case "type":
			if strings.Contains(app.pathInfo.Path, "icmpv4/config") {
				(&ruleEntry).Remove("ICMP_TYPE")
			} else if strings.Contains(app.pathInfo.Path, "icmpv6/config") {
				(&ruleEntry).Remove("ICMPV6_TYPE")
			}
		case "code":
			if strings.Contains(app.pathInfo.Path, "icmpv4/config") {
				(&ruleEntry).Remove("ICMP_CODE")
			} else if strings.Contains(app.pathInfo.Path, "icmpv6/config") {
				(&ruleEntry).Remove("ICMPV6_CODE")
			}
			// transport. Deleting the port match takes the port set it was
			// resolved from with it; deleting the reference takes the match.
		case "source-port", "source-port-set":
			(&ruleEntry).Remove(L4_SRC_PORT)
			(&ruleEntry).Remove(L4_SRC_PORT_RANGE)
			clearRefs = append(clearRefs, RULE_SRC_PORT_SET)
		case "destination-port", "destination-port-set":
			(&ruleEntry).Remove(L4_DST_PORT)
			(&ruleEntry).Remove(L4_DST_PORT_RANGE)
			clearRefs = append(clearRefs, RULE_DST_PORT_SET)
			// actions
		case "forwarding-action":
			(&ruleEntry).Remove("PACKET_ACTION")
			//input-interface
		case "interface":
			(&ruleEntry).Remove("IN_PORTS")
			//case "subinterface":
		}
	} else if nodeInfo.IsContainer() {
		targetType := reflect.TypeOf(*app.ygotTarget)
		switch targetType.Elem().Name() {
		case "OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_L2", "OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_L2_Config":
			(&ruleEntry).Remove("ETHER_TYPE")
			// The icmpv4 match fields live under the sibling ipv4/icmpv4
			// container, so they must only be cleared when the whole ipv4
			// container is deleted, not when only ipv4/config is deleted. On a
			// DELETE the config container is empty, so ygot collapses the
			// target to the parent Ipv4 struct for both paths; the request
			// path is the reliable discriminator.
		case "OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv4", "OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv4_Config":
			(&ruleEntry).Remove("IP_PROTOCOL")
			(&ruleEntry).Remove("SRC_IP")
			(&ruleEntry).Remove("DST_IP")
			(&ruleEntry).Remove("DSCP")
			if !strings.Contains(app.pathInfo.Path, "ipv4/config") {
				(&ruleEntry).Remove("ICMP_TYPE")
				(&ruleEntry).Remove("ICMP_CODE")
			}
		case "OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv4_Icmpv4", "OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv4_Icmpv4_Config":
			(&ruleEntry).Remove("ICMP_TYPE")
			(&ruleEntry).Remove("ICMP_CODE")
			// Same sibling-container reasoning as ipv4 above for icmpv6.
		case "OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv6", "OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv6_Config":
			(&ruleEntry).Remove("IP_PROTOCOL")
			(&ruleEntry).Remove("SRC_IPV6")
			(&ruleEntry).Remove("DST_IPV6")
			(&ruleEntry).Remove("DSCP")
			if !strings.Contains(app.pathInfo.Path, "ipv6/config") {
				(&ruleEntry).Remove("ICMPV6_TYPE")
				(&ruleEntry).Remove("ICMPV6_CODE")
			}
		case "OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv6_Icmpv6", "OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv6_Icmpv6_Config":
			(&ruleEntry).Remove("ICMPV6_TYPE")
			(&ruleEntry).Remove("ICMPV6_CODE")
		case "OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport", "OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config":
			(&ruleEntry).Remove(L4_SRC_PORT)
			(&ruleEntry).Remove(L4_SRC_PORT_RANGE)
			(&ruleEntry).Remove(L4_DST_PORT)
			(&ruleEntry).Remove(L4_DST_PORT_RANGE)
			(&ruleEntry).Remove("TCP_FLAGS")
			clearRefs = append(clearRefs, RULE_SRC_PORT_SET, RULE_DST_PORT_SET)
		case "OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_InputInterface", "OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_InputInterface_InterfaceRef", "OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_InputInterface_InterfaceRef_Config":
			(&ruleEntry).Remove("IN_PORTS")
		case "OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Actions", "OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Actions_Config":
			(&ruleEntry).Remove("PACKET_ACTION")
		}
	} else if nodeInfo.IsLeafList() {
		switch nodeInfo.Name {
		case "tcp-flags":
			(&ruleEntry).Remove("TCP_FLAGS")
		}
	} else {
		log.Error("This yang type is not handled currently")
	}
	err = d.SetEntry(app.ruleTs, asKey(aclKey, ruleKey), ruleEntry)
	if err != nil {
		return err
	}

	if len(clearRefs) > 0 {
		err = app.clearRulePortSetRefs(d, aclKey, ruleKey, clearRefs...)
	}

	return err
}

// clearStaleRulePortMatch removes the L4 port match a rule is moving away from,
// so a rule can go from a port to a range and back.
//
// The plain port and the range are a yang choice, and the one that is no longer
// used has to be removed in a request of its own: CVL validates a write against
// the merged view of the request and the current row, so clearing and setting
// the two halves at once looks to it like a rule matching on a port and on a
// range at the same time.
func (app *AclApp) clearStaleRulePortMatch(d *db.DB, aclKey, ruleKey string, ruleData db.Value) error {
	existing, err := d.GetEntry(app.ruleTs, asKey(aclKey, ruleKey))
	if err != nil || !existing.IsPopulated() {
		return nil
	}

	stale := db.Value{Field: map[string]string{}}
	for _, isSrc := range []bool{true, false} {
		port, portRange := rulePortFields(isSrc)
		switch {
		case (&ruleData).Has(port) && (&existing).Has(portRange):
			stale.Field[portRange] = ""
		case (&ruleData).Has(portRange) && (&existing).Has(port):
			stale.Field[port] = ""
		}
	}

	if !stale.IsPopulated() {
		return nil
	}

	return d.DeleteEntryFields(app.ruleTs, asKey(aclKey, ruleKey), stale)
}

// rulePortSetInfo records what a rule's payload said about its L4 port match, so
// the ACL_RULE_PORT_SET references can be brought in line once the rule itself
// has been written.
type rulePortSetInfo struct {
	srcSet     string // port set the source port match was resolved from
	dstSet     string // port set the destination port match was resolved from
	srcLiteral bool   // payload carried a literal source port
	dstLiteral bool   // payload carried a literal destination port
}

// syncRulePortSetRefs updates the port set references of the rules the request
// touched. A reference goes away when the rule stops matching on a set, either
// because it was rewritten without one or because it now matches a literal port.
func (app *AclApp) syncRulePortSetRefs(d *db.DB, opcode int) error {
	for aclKey, rules := range app.ruleTableMap {
		for ruleKey := range rules {
			if ruleKey == "DEFAULT_RULE" {
				continue
			}

			existing := rulePortSetRefs(d, aclKey, ruleKey)
			refs := db.Value{Field: map[string]string{}}
			// An update merges into the rule, so the references it says nothing
			// about have to survive it.
			if opcode == UPDATE && existing.IsPopulated() {
				refs = existing.Copy()
			}

			if info := app.rulePortSetMap[aclKey][ruleKey]; info != nil {
				if info.srcSet != "" {
					refs.Field[RULE_SRC_PORT_SET] = info.srcSet
				} else if info.srcLiteral {
					(&refs).Remove(RULE_SRC_PORT_SET)
				}
				if info.dstSet != "" {
					refs.Field[RULE_DST_PORT_SET] = info.dstSet
				} else if info.dstLiteral {
					(&refs).Remove(RULE_DST_PORT_SET)
				}
			}

			key := db.Key{Comp: []string{aclKey, ruleKey}}
			switch {
			case len(refs.Field) == 0:
				if existing.IsPopulated() {
					if err := d.DeleteEntry(app.refTs, key); err != nil {
						return err
					}
				}
			case !existing.Equals(&refs):
				if err := d.SetEntry(app.refTs, key, refs); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// deleteRulePortSetRefs removes the port set references of one rule.
func (app *AclApp) deleteRulePortSetRefs(d *db.DB, aclKey, ruleKey string) error {
	refs := rulePortSetRefs(d, aclKey, ruleKey)
	if !refs.IsPopulated() {
		return nil
	}
	return d.DeleteEntry(app.refTs, db.Key{Comp: []string{aclKey, ruleKey}})
}

// deleteAclPortSetRefs removes the port set references of every rule of an ACL.
func (app *AclApp) deleteAclPortSetRefs(d *db.DB, aclKey string) error {
	keys, err := d.GetKeys(app.refTs)
	if err != nil {
		return err
	}

	for _, key := range keys {
		if key.Len() > 0 && key.Get(0) == aclKey {
			if err := d.DeleteEntry(app.refTs, key); err != nil {
				return err
			}
		}
	}

	return nil
}

// clearRulePortSetRefs removes the named references of a rule, and the row along
// with them when nothing is left to point at.
func (app *AclApp) clearRulePortSetRefs(d *db.DB, aclKey, ruleKey string, fields ...string) error {
	refs := rulePortSetRefs(d, aclKey, ruleKey)
	if !refs.IsPopulated() {
		return nil
	}

	for _, field := range fields {
		(&refs).Remove(field)
	}

	key := db.Key{Comp: []string{aclKey, ruleKey}}
	if len(refs.Field) == 0 {
		return d.DeleteEntry(app.refTs, key)
	}
	return d.SetEntry(app.refTs, key, refs)
}

func (app *AclApp) setAclDataInConfigDb(d *db.DB, aclData map[string]db.Value, createFlag bool) error {
	var err error
	for key := range aclData {
		existingEntry, err := d.GetEntry(app.aclTs, db.Key{Comp: []string{key}})
		// If Create ACL request comes and ACL already exists, throw error
		if createFlag && existingEntry.IsPopulated() {
			return tlerr.AlreadyExists("Acl %s already exists", key)
		}
		if createFlag || (!createFlag && err != nil && !existingEntry.IsPopulated()) {
			err := d.CreateEntry(app.aclTs, db.Key{Comp: []string{key}}, aclData[key])
			if err != nil {
				return err
			}
		} else {
			if existingEntry.IsPopulated() {
				mod := db.Value{Field: map[string]string{}}
				payload := aclData[key]
				if existingEntry.Get(ACL_DESCRIPTION) != payload.Field[ACL_DESCRIPTION] {
					mod.Field[ACL_DESCRIPTION] = payload.Field[ACL_DESCRIPTION]
				}
				if ocName := payload.Get(ACL_OPENCONFIG_SET_NAME); len(ocName) > 0 &&
					existingEntry.Get(ACL_OPENCONFIG_SET_NAME) != ocName {
					mod.Field[ACL_OPENCONFIG_SET_NAME] = ocName
				}
				if mod.IsPopulated() {
					err := d.ModEntry(app.aclTs, db.Key{Comp: []string{key}}, mod)
					if err != nil {
						return err
					}
				}
			}
		}
	}
	return err
}

func (app *AclApp) setAclRuleDataInConfigDb(d *db.DB, ruleData map[string]map[string]db.Value, createFlag bool) error {
	var err error
	for aclName := range ruleData {
		for ruleName := range ruleData[aclName] {
			existingRuleEntry, err := d.GetEntry(app.ruleTs, db.Key{Comp: []string{aclName, ruleName}})
			// If Create Rule request comes and Rule already exists, throw error
			if createFlag && existingRuleEntry.IsPopulated() {
				return tlerr.AlreadyExists("Rule %s already exists", ruleName)
			}
			if createFlag || (!createFlag && err != nil && !existingRuleEntry.IsPopulated()) {
				err := d.CreateEntry(app.ruleTs, db.Key{Comp: []string{aclName, ruleName}}, ruleData[aclName][ruleName])
				if err != nil {
					return err
				}
			} else {
				if existingRuleEntry.IsPopulated() && ruleName != "DEFAULT_RULE" {
					if err := app.clearStaleRulePortMatch(d, aclName, ruleName, ruleData[aclName][ruleName]); err != nil {
						return err
					}
					if err := app.clearStaleRuleIcmpv6Code(d, aclName, ruleName); err != nil {
						return err
					}
					err := d.ModEntry(app.ruleTs, db.Key{Comp: []string{aclName, ruleName}}, ruleData[aclName][ruleName])
					if err != nil {
						return err
					}
				}
			}
		}
	}
	return err
}

// replaceInterfaceAclBindings removes stale ingress or egress bindings on an
// interface when a Replace targets the whole ingress-acl-sets or egress-acl-sets
// list. Without this, setAclBindDataInConfigDb only adds the new ACL ports and
// leaves the previous ACL table rows bound to the same interface.
func (app *AclApp) replaceInterfaceAclBindings(d *db.DB) error {
	ingressList := isSubtreeRequest(app.pathInfo.Template, "/openconfig-acl:acl/interfaces/interface{}/ingress-acl-sets") &&
		!isSubtreeRequest(app.pathInfo.Template, "/openconfig-acl:acl/interfaces/interface{}/ingress-acl-sets/ingress-acl-set{}{}")
	egressList := isSubtreeRequest(app.pathInfo.Template, "/openconfig-acl:acl/interfaces/interface{}/egress-acl-sets") &&
		!isSubtreeRequest(app.pathInfo.Template, "/openconfig-acl:acl/interfaces/interface{}/egress-acl-sets/egress-acl-set{}{}")
	if !ingressList && !egressList {
		return nil
	}

	acl := app.getAppRootObject()
	if acl.Interfaces == nil {
		return nil
	}

	keep := make(map[string]struct{})
	for k := range app.aclTableMap {
		keep[k] = struct{}{}
	}

	for intfId, intfData := range acl.Interfaces.Interface {
		if intfData == nil {
			continue
		}
		if ingressList {
			if err := app.stripInterfaceAclBindings(d, intfId, "INGRESS", keep); err != nil {
				return err
			}
		}
		if egressList {
			if err := app.stripInterfaceAclBindings(d, intfId, "EGRESS", keep); err != nil {
				return err
			}
		}
	}
	return nil
}

func (app *AclApp) stripInterfaceAclBindings(d *db.DB, intfId, stage string, keep map[string]struct{}) error {
	aclKeys, err := d.GetKeys(app.aclTs)
	if err != nil {
		return err
	}
	for _, key := range aclKeys {
		if _, ok := keep[key.Get(0)]; ok {
			continue
		}
		entry, err := d.GetEntry(app.aclTs, key)
		if err != nil || !entry.IsPopulated() {
			continue
		}
		if entry.Get("stage") != stage {
			continue
		}
		ports := entry.GetList("ports")
		if !contains(ports, intfId) {
			continue
		}
		ports = removeElement(ports, intfId)
		entry.SetList("ports", ports)
		if len(ports) == 0 {
			entry.Remove("stage")
		}
		if err := d.SetEntry(app.aclTs, key, entry); err != nil {
			return err
		}
	}
	return nil
}

func ocAclSetNameFromTable(aclData db.Value, sonicKey, sonicType string) string {
	if name := aclData.Get(ACL_OPENCONFIG_SET_NAME); len(name) > 0 {
		return name
	}
	name, _ := getAclKeysFromStrKey(sonicKey, sonicType)
	return name
}

func (app *AclApp) setAclBindDataInConfigDb(d *db.DB, aclData map[string]db.Value, opcode int) error {
	var err error
	for aclKey, aclInfo := range aclData {
		// Get ACL info from DB
		dbAcl, err := d.GetEntry(app.aclTs, db.Key{Comp: []string{aclKey}})
		if err != nil {
			return err
		}
		if ocName := aclInfo.Get(ACL_OPENCONFIG_SET_NAME); len(ocName) > 0 {
			dbAcl.Set(ACL_OPENCONFIG_SET_NAME, ocName)
		}
		if REPLACE == opcode {
			dbAcl.SetList("ports", aclInfo.GetList("ports"))
			dbAcl.Set("stage", aclInfo.Get("stage"))
		} else {
			dbAclIntfs := dbAcl.GetList("ports")
			if len(dbAclIntfs) > 0 {
				dbAclDirec := dbAcl.Get("stage")
				newDirec := aclInfo.Get("stage")
				if (UPDATE == opcode) && (len(dbAclDirec) > 0) && (len(newDirec) > 0) && (dbAclDirec != newDirec) {
					return tlerr.InvalidArgs("Acl direction of %s not allowed when it is already configured as %s", newDirec, dbAclDirec)
				}
				// Merge interfaces from DB to list in aclInfo and set back in DB
				intfs := aclInfo.GetList("ports")
				for _, ifId := range dbAclIntfs {
					if !contains(intfs, ifId) {
						intfs = append(intfs, ifId)
					}
				}
				dbAcl.SetList("ports", intfs)
			} else {
				dbAcl.SetList("ports", aclInfo.GetList("ports"))
			}

			if len(dbAcl.Get("stage")) == 0 {
				dbAcl.Set("stage", aclInfo.Get("stage"))
			}
		}
		err = d.SetEntry(app.aclTs, db.Key{Comp: []string{aclKey}}, dbAcl)
		//err = d.ModEntry(app.aclTs, db.Key{Comp: []string{aclKey}}, dbAcl)
		if err != nil {
			return err
		}
	}
	return err
}

func getIpProtocol(proto int64) interface{} {
	for k, v := range IP_PROTOCOL_MAP {
		if uint8(proto) == v {
			return k
		}
	}
	return uint8(proto)
}

// getIcmpv6TypeId reverse-maps a numeric ICMPv6 type to its standard
// openconfig-icmpv6-types identity, or UNSET if unknown.
func getIcmpv6TypeId(num uint8) ocbinds.E_OpenconfigIcmpv6Types_TYPE {
	for k, v := range ICMPV6_TYPE_MAP {
		if num == v {
			return k
		}
	}
	return ocbinds.OpenconfigIcmpv6Types_TYPE_UNSET
}

// getIcmpv6CodeId recovers the CODE identity for a numeric code. The same
// number stands for a different code under each type, so the type selects which
// identity it names.
func getIcmpv6CodeId(typ ocbinds.E_OpenconfigIcmpv6Types_TYPE, num uint8) ocbinds.E_OpenconfigIcmpv6Types_CODE {
	if typ == ocbinds.OpenconfigIcmpv6Types_TYPE_UNSET {
		return ocbinds.OpenconfigIcmpv6Types_CODE_UNSET
	}
	for k, v := range ICMPV6_CODE_MAP {
		if v.typ == typ && v.num == num {
			return k
		}
	}
	return ocbinds.OpenconfigIcmpv6Types_CODE_UNSET
}

func getTransportSrcDestPorts(portVal string, portType string) interface{} {
	var portRange string = ""

	portNum, err := strconv.Atoi(portVal)
	if err != nil && strings.Contains(portVal, "-") {
		// CONFIG_DB stores ranges as "<low>-<high>", but the OpenConfig
		// port-num-range type requires "<low>..<high>".
		portRange = strings.Replace(portVal, "-", "..", 1)
	}

	if len(portRange) > 0 {
		return portRange
	} else if portNum > 0 {
		return uint16(portNum)
	} else {
		if "src" == portType {
			return ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_SourcePort_ANY
		} else if "dest" == portType {
			return ocbinds.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config_DestinationPort_ANY
		}
	}
	return nil
}

func getTransportConfigTcpFlags(tcpFlags string) []ocbinds.E_OpenconfigPacketMatchTypes_TCP_FLAGS {
	var flags []ocbinds.E_OpenconfigPacketMatchTypes_TCP_FLAGS
	if len(tcpFlags) > 0 {
		flagStr := strings.Split(tcpFlags, "/")[0]
		flagNumber, _ := strconv.ParseUint(strings.Replace(flagStr, "0x", "", -1), 16, 32)
		for i := 0; i < 8; i++ {
			mask := 1 << uint(i)
			if (int(flagNumber) & mask) > 0 {
				switch int(flagNumber) & mask {
				case 0x01:
					flags = append(flags, ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_FIN)
				case 0x02:
					flags = append(flags, ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_SYN)
				case 0x04:
					flags = append(flags, ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_RST)
				case 0x08:
					flags = append(flags, ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_PSH)
				case 0x10:
					flags = append(flags, ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_ACK)
				case 0x20:
					flags = append(flags, ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_URG)
				case 0x40:
					flags = append(flags, ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_ECE)
				case 0x80:
					flags = append(flags, ocbinds.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_CWR)
				default:
				}
			}
		}
	}
	return flags
}

func getL2EtherType(etherType uint64) interface{} {
	for k, v := range ETHERTYPE_MAP {
		if uint32(etherType) == v {
			return k
		}
	}
	return uint16(etherType)
}

// ctrlPlaneOCNameType reconstructs the OpenConfig acl-set name and type for a
// control-plane (CTRLPLANE) table whose ACL_TABLE:type no longer encodes the OC
// type. The OC type is preserved in the composite table-name suffix
// (e.g. "MGMT_SSH_ACL_IPV4"); default to ACL_IPV4 if no known suffix is found.
func ctrlPlaneOCNameType(aclKey string) (string, ocbinds.E_OpenconfigAcl_ACL_TYPE) {
	switch {
	case strings.HasSuffix(aclKey, "_"+OPENCONFIG_ACL_TYPE_IPV6):
		return strings.Replace(aclKey, "_"+OPENCONFIG_ACL_TYPE_IPV6, "", 1), ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV6
	case strings.HasSuffix(aclKey, "_"+OPENCONFIG_ACL_TYPE_L2):
		return strings.Replace(aclKey, "_"+OPENCONFIG_ACL_TYPE_L2, "", 1), ocbinds.OpenconfigAcl_ACL_TYPE_ACL_L2
	default:
		return strings.Replace(aclKey, "_"+OPENCONFIG_ACL_TYPE_IPV4, "", 1), ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV4
	}
}

func getAclKeysFromStrKey(aclKey string, aclType string) (string, ocbinds.E_OpenconfigAcl_ACL_TYPE) {
	var aclOrigName string
	var aclOrigType ocbinds.E_OpenconfigAcl_ACL_TYPE

	if SONIC_ACL_TYPE_IPV4 == aclType {
		aclOrigName = strings.Replace(aclKey, "_"+OPENCONFIG_ACL_TYPE_IPV4, "", 1)
		aclOrigType = ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV4
	} else if SONIC_ACL_TYPE_IPV6 == aclType {
		aclOrigName = strings.Replace(aclKey, "_"+OPENCONFIG_ACL_TYPE_IPV6, "", 1)
		aclOrigType = ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV6
	} else if SONIC_ACL_TYPE_L2 == aclType {
		aclOrigName = strings.Replace(aclKey, "_"+OPENCONFIG_ACL_TYPE_L2, "", 1)
		aclOrigType = ocbinds.OpenconfigAcl_ACL_TYPE_ACL_L2
	}
	return aclOrigName, aclOrigType
}

// getAclTypeOCEnumFromName returns the ACL_TYPE enum from name
func getAclTypeOCEnumFromName(val string) (ocbinds.E_OpenconfigAcl_ACL_TYPE, error) {
	switch val {
	case "ACL_IPV4", "openconfig-acl:ACL_IPV4":
		return ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV4, nil
	case "ACL_IPV6", "openconfig-acl:ACL_IPV6":
		return ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV6, nil
	case "ACL_L2", "openconfig-acl:ACL_L2":
		return ocbinds.OpenconfigAcl_ACL_TYPE_ACL_L2, nil
	default:
		return ocbinds.OpenconfigAcl_ACL_TYPE_UNSET,
			tlerr.NotSupported("ACL Type '%s' not supported", val)
	}
}

func convertSonicAclTypeToOC(aclType string) (ocbinds.E_OpenconfigAcl_ACL_TYPE, string) {
	switch aclType {
	case SONIC_ACL_TYPE_IPV4:
		return ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV4, "ACL_IPV4"
	case SONIC_ACL_TYPE_IPV6:
		return ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV6, "ACL_IPV6"
	case SONIC_ACL_TYPE_L2:
		return ocbinds.OpenconfigAcl_ACL_TYPE_ACL_L2, "ACL_L2"
	default:
		return ocbinds.OpenconfigAcl_ACL_TYPE_UNSET, ""
	}
}

func getAclKeyStrFromOCKey(aclname string, acltype ocbinds.E_OpenconfigAcl_ACL_TYPE) string {
	aclT := acltype.ΛMap()["E_OpenconfigAcl_ACL_TYPE"][int64(acltype)].Name
	return getAclKeyStr(aclname, aclT)
}

// getAclKeyStr builds the ACL_TABLE key for an acl-set name and its OC type name.
// SONiC table names carry neither spaces nor dashes, so both are substituted the
// same way on every path that reaches the db, reads and subscriptions included.
func getAclKeyStr(aclName string, aclType string) string {
	aclN := strings.Replace(strings.Replace(aclName, " ", "_", -1), "-", "_", -1)
	return aclN + "_" + aclType
}

/*
Check if targetUriPath is child (subtree) of nodePath
The return value can be used to decide if subtrees needs
to visited to fill the data or not.
*/
func isSubtreeRequest(targetUriPath string, nodePath string) bool {
	return strings.HasPrefix(targetUriPath, nodePath)
}

func (app *AclApp) translateSubscribe(req translateSubRequest) (translateSubResponse, error) {
	ymap := yangMapTree{
		subtree: map[string]*yangMapTree{
			"acl-sets/acl-set": {
				mapFunc: app.translateSubscribeAclSet,
				subtree: map[string]*yangMapTree{
					"acl-entries/acl-entry": {
						mapFunc: app.translateSubscribeAclEntry,
					},
				},
			},
			"interfaces": {
				mapFunc: app.translateSubscribeAclBindings,
			},
		}}

	nb := notificationInfoBuilder{
		pathInfo: NewPathInfo(req.path),
		yangMap:  ymap,
	}
	return nb.Build()
}

func (app *AclApp) translateSubscribeAclSet(nb *notificationInfoBuilder) error {
	aclName := nb.pathInfo.StringVar("name", "*")
	aclType := nb.pathInfo.StringVar("type", "*")

	nb.New().PathKey("name", aclName).PathKey("type", aclType)
	nb.Table(db.ConfigDB, ACL_TABLE).Key(getAclKeyStr(aclName, aclType))
	if nb.SetFieldPrefix("config") {
		nb.Field("description", ACL_DESCRIPTION)
	}
	if nb.SetFieldPrefix("state") {
		nb.Field("description", ACL_DESCRIPTION)
	}

	return nil
}

func (app *AclApp) translateSubscribeAclEntry(nb *notificationInfoBuilder) error {
	aclName := nb.pathInfo.StringVar("name", "*")
	aclType := nb.pathInfo.StringVar("type", "*")
	ruleSeq := nb.pathInfo.StringVar("sequence-id", "*")

	nb.New().PathKey("sequence-id", ruleSeq)
	nb.Table(db.ConfigDB, RULE_TABLE).Key(getAclKeyStr(aclName, aclType), "RULE_"+ruleSeq)

	ipAclFieldSetter := func(prefix string) {
		if nb.SetFieldPrefix(prefix) {
			nb.Field("source-address", "SRC_IP")
			nb.Field("destination-address", "DST_IP")
			nb.Field("dscp", "DSCP")
			nb.Field("protocol", "IP_PROTOCOL")
		}
	}
	ipv6AclFieldSetter := func(prefix string) {
		if nb.SetFieldPrefix(prefix) {
			nb.Field("source-address", "SRC_IPV6")
			nb.Field("destination-address", "DST_IPV6")
			nb.Field("dscp", "DSCP")
			nb.Field("protocol", "IP_PROTOCOL")
		}
	}
	ipTransportFieldSetter := func(prefix string) {
		if nb.SetFieldPrefix(prefix) {
			nb.Field("source-port", "L4_SRC_PORT")
			nb.Field("source-port", "L4_SRC_PORT_RANGE")
			nb.Field("destination-port", "L4_DST_PORT")
			nb.Field("destination-port", "L4_DST_PORT_RANGE")
			nb.Field("tcp-flags", "TCP_FLAGS")
		}
	}
	// The icmpv4/icmpv6 match leaves live under their own container (not
	// ipv4/config), so they need their own prefix registration for ON_CHANGE
	// subscriptions to fire when ICMP_TYPE/ICMP_CODE change in CONFIG_DB.
	icmpv4FieldSetter := func(prefix string) {
		if nb.SetFieldPrefix(prefix) {
			nb.Field("type", "ICMP_TYPE")
			nb.Field("code", "ICMP_CODE")
		}
	}
	icmpv6FieldSetter := func(prefix string) {
		if nb.SetFieldPrefix(prefix) {
			nb.Field("type", "ICMPV6_TYPE")
			nb.Field("code", "ICMPV6_CODE")
		}
	}
	if wildcardMatch(aclType, "ACL_IPV4") {
		ipAclFieldSetter("ipv4/config")
		ipAclFieldSetter("ipv4/state")
		icmpv4FieldSetter("ipv4/icmpv4/config")
		icmpv4FieldSetter("ipv4/icmpv4/state")
		ipTransportFieldSetter("transport/config")
		ipTransportFieldSetter("transport/state")
	}
	if wildcardMatch(aclType, "ACL_IPV6") {
		ipv6AclFieldSetter("ipv6/config")
		ipv6AclFieldSetter("ipv6/state")
		icmpv6FieldSetter("ipv6/icmpv6/config")
		icmpv6FieldSetter("ipv6/icmpv6/state")
		ipTransportFieldSetter("transport/config")
		ipTransportFieldSetter("transport/state")
	}
	if nb.SetFieldPrefix("actions/config") {
		nb.Field("forwarding-action", "PACKET_ACTION")
	}
	if nb.SetFieldPrefix("actions/state") {
		nb.Field("forwarding-action", "PACKET_ACTION")
	}

	return nil
}

func (app *AclApp) translateSubscribeAclBindings(nb *notificationInfoBuilder) error {
	nb.New().OnChange(false).Preferred(Sample)
	return nil
}

func (app *AclApp) processSubscribe(req processSubRequest) (processSubResponse, error) {
	resp := processSubResponse{
		path: req.path,
	}

	switch req.table.Name {
	case ACL_TABLE:
		if path.GetElemAt(resp.path, 2) != "acl-set" {
			return resp, tlerr.New("Unknown path template: %v", path.String(req.path))
		}

		_, aclTypeStr := convertSonicAclTypeToOC(req.entry.Get(ACL_TYPE))
		aclName := strings.TrimSuffix(req.key.Get(0), "_"+aclTypeStr)
		resolveKeyAt(resp.path, 2, "name", aclName)
		resolveKeyAt(resp.path, 2, "type", aclTypeStr)

	case RULE_TABLE:
		ruleName := req.key.Get(1)
		if !strings.HasPrefix(ruleName, "RULE_") {
			// The default-deny rule translib maintains itself has no
			// sequence-id, and the read path leaves it out of acl-entries too.
			return resp, tlerr.New("Not an acl-entry: %s", ruleName)
		}

		aclName := req.key.Get(0)
		aclEntry, err := req.dbs[db.ConfigDB].GetEntry(&db.TableSpec{Name: ACL_TABLE}, asKey(aclName))
		if err != nil {
			return resp, err
		}

		_, aclTypeStr := convertSonicAclTypeToOC(aclEntry.Get(ACL_TYPE))
		aclName = strings.TrimSuffix(aclName, "_"+aclTypeStr)
		resolveKeyAt(resp.path, 2, "name", aclName)
		resolveKeyAt(resp.path, 2, "type", aclTypeStr)
		resolveKeyAt(resp.path, 4, "sequence-id", strings.TrimPrefix(ruleName, "RULE_"))

	default:
		return resp, tlerr.New("Unknown table: %s", req.table.Name)
	}

	return resp, nil
}

// resolveKeyAt fills a key the subscribe path template left as a wildcard, and
// leaves a key the client spelled out alone.
//
// The value derived from the DB key is only an approximation of what was asked
// for: a dash is not allowed in a SONiC table name, so an acl-set named
// "ACL-12-IPV4" is stored as "ACL_12_IPV4" and reads back with underscores.
// Overwriting the requested name with that form makes translib's check that the
// resolved path still matches the requested template fail, and the notification
// is dropped - which is how a subscription carrying a wildcard rule key ended up
// reporting nothing at all for such an acl-set.
func resolveKeyAt(p *gnmi.Path, index int, name, value string) {
	if index < path.Len(p) {
		if cur, ok := p.Elem[index].Key[name]; ok && cur != "*" {
			return
		}
	}
	path.SetKeyAt(p, index, name, value)
}
