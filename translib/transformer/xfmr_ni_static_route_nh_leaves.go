package transformer

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
)

// A next-hop's interface-ref names the interface its index already carries, so
// it is checked against the index rather than stored apart; enable-bfd is the
// next-hop's element of the bfd list.
const (
	niStaticRouteBfdFld          = "bfd"
	niStaticRouteTargetNhSub     = "static-route-next-hop-sub"
	niStaticRouteInterfaceRefCnt = "interface-ref"
	niStaticRouteEnableBfdCnt    = "enable-bfd"
)

var niStaticRouteNhSubReqUriRe = regexp.MustCompile(niStaticRouteUriPfx +
	`/static-routes/static\[prefix=([^\]]+)\]/next-hops/next-hop\[index=([^\]]+)\]/(interface-ref|enable-bfd)(?:/(config|state)(?:/(interface|subinterface|enabled))?)?$`)

func matchNiStaticRouteNhSubReq(uri string) *niStaticRouteReqInfo {
	m := niStaticRouteNhSubReqUriRe.FindStringSubmatch(uri)
	if m == nil {
		return nil
	}
	return &niStaticRouteReqInfo{
		niName:         m[1],
		protoName:      m[2],
		prefix:         niStaticRouteUnescapeKey(m[3]),
		index:          niStaticRouteUnescapeKey(m[4]),
		subNode:        m[5],
		subSide:        m[6],
		targetLeaf:     m[7],
		targetPathKind: niStaticRouteTargetNhSub,
	}
}

// niStaticRouteSplitIfname splits a stored interface into the interface and
// subinterface OpenConfig names apart: Ethernet0.10 is subinterface 10 of
// Ethernet0.
func niStaticRouteSplitIfname(ifname string) (string, *uint32) {
	if i := strings.LastIndex(ifname, "."); i > 0 {
		if n, err := strconv.ParseUint(ifname[i+1:], 10, 32); err == nil {
			sub := uint32(n)
			return ifname[:i], &sub
		}
	}
	return ifname, nil
}

// niStaticRouteIsIpIndex reports a next-hop index naming an IP gateway, the
// only kind FRR monitors with BFD.
func niStaticRouteIsIpIndex(index string) bool {
	return index != niStaticRouteDropToken && !strings.HasPrefix(index, niStaticRouteLocalLinkToken)
}

// niStaticRouteNhSubFields applies the interface-ref and enable-bfd of a
// next-hop payload, nhObj, to nh, the next-hop its index stands for.
func niStaticRouteNhSubFields(nhObj *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_NextHops_NextHop, index string, nh map[string]string) error {
	if nhObj == nil {
		return nil
	}
	if ir := nhObj.InterfaceRef; ir != nil && ir.Config != nil && (ir.Config.Interface != nil || ir.Config.Subinterface != nil) {
		want := ""
		if ir.Config.Interface != nil {
			want = strings.TrimSpace(*ir.Config.Interface)
		}
		if ir.Config.Subinterface != nil {
			want = fmt.Sprintf("%s.%d", want, *ir.Config.Subinterface)
		}
		if have := strings.TrimSpace(nh[niStaticRouteIfnameFld]); want != have {
			return tlerr.InvalidArgsError{Format: fmt.Sprintf("static route next-hop interface-ref names %q; its index %q names %q", want, index, have)}
		}
	}
	if eb := nhObj.EnableBfd; eb != nil && eb.Config != nil && eb.Config.Enabled != nil {
		if *eb.Config.Enabled && !niStaticRouteIsIpIndex(index) {
			return tlerr.NotSupportedError{Format: "static route next-hop enable-bfd is supported on IP next-hops only"}
		}
		nh[niStaticRouteBfdFld] = strconv.FormatBool(*eb.Config.Enabled)
	}
	return nil
}

// niStaticRouteNhSubToYang fills the interface-ref and enable-bfd of nhObj on
// side, "config" or "state", from e, a stored or an applied next-hop. FRR does
// not report BFD monitoring, so enable-bfd state mirrors the configuration and
// is filled with it.
func niStaticRouteNhSubToYang(nhObj *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_NextHops_NextHop, e map[string]string, side string) {
	ifname := strings.TrimSpace(e[niStaticRouteIfnameFld])
	if ifname != "" && ifname != niStaticRouteNullIfname {
		iface, sub := niStaticRouteSplitIfname(ifname)
		if nhObj.InterfaceRef == nil {
			nhObj.InterfaceRef = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_NextHops_NextHop_InterfaceRef{}
		}
		if side == "state" {
			nhObj.InterfaceRef.State = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_NextHops_NextHop_InterfaceRef_State{Interface: &iface, Subinterface: sub}
		} else {
			nhObj.InterfaceRef.Config = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_NextHops_NextHop_InterfaceRef_Config{Interface: &iface, Subinterface: sub}
		}
	}
	if side == "state" {
		return
	}
	if raw := strings.TrimSpace(e[niStaticRouteBfdFld]); raw != "" && niStaticRouteIsIpIndex(niStaticRouteNhIndex(e)) {
		enabled := raw == "true"
		nhObj.EnableBfd = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_NextHops_NextHop_EnableBfd{
			Config: &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_NextHops_NextHop_EnableBfd_Config{Enabled: &enabled},
			State:  &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_NextHops_NextHop_EnableBfd_State{Enabled: &enabled},
		}
	}
}

// niStaticRouteNhSubDelete serves a DELETE below a next-hop: enable-bfd turns
// monitoring off; the interface-ref is part of the next-hop's identity, so the
// next-hop itself is what has to go.
func niStaticRouteNhSubDelete(inParams XfmrParams, reqInfo *niStaticRouteReqInfo) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	if reqInfo.subNode == niStaticRouteInterfaceRefCnt {
		return res, tlerr.NotSupportedError{Format: "static route next-hop interface-ref is part of the next-hop index; delete the next-hop instead", Path: inParams.requestUri}
	}
	existing := niStaticRouteEntryOrEmpty(inParams.dbs[db.ConfigDB], reqInfo.niName, reqInfo.prefix)
	entries := niStaticRouteDecodeNextHops(existing)
	found := false
	for _, e := range entries {
		if niStaticRouteNhIndex(e) == reqInfo.index {
			found = true
		}
	}
	if !found {
		return res, tlerr.NotFoundError{Format: fmt.Sprintf("static route %s has no next-hop %s", reqInfo.prefix, reqInfo.index)}
	}
	entries = niStaticRouteUpsertNextHop(entries, reqInfo.index, map[string]string{niStaticRouteBfdFld: "false"})
	// The bfd list is shared with the other next-hops, so it is rewritten
	// through an update rather than deleted.
	if inParams.subOpDataMap == nil {
		return res, tlerr.InvalidArgsError{Format: "unable to rewrite static route next-hops for DELETE"}
	}
	updMap := make(RedisDbMap)
	updMap[db.ConfigDB] = makeNiStaticRouteResMap(reqInfo.niName, reqInfo.prefix, niStaticRouteEncodeNextHops(entries, existing, reqInfo.prefix))
	inParams.subOpDataMap[UPDATE] = &updMap
	return res, nil
}

// niStaticRouteNhSubDbToYang serves a GET below a next-hop.
func niStaticRouteNhSubDbToYang(inParams XfmrParams, reqInfo *niStaticRouteReqInfo) error {
	wrappedRoot, err := wrapNiStaticRouteRootAsDevice(inParams.ygRoot, reqInfo)
	if err != nil {
		return err
	}
	// enable-bfd state is the configuration, which FRR does not report.
	fromApplied := reqInfo.subSide == "state" && reqInfo.subNode == niStaticRouteInterfaceRefCnt
	var entry db.Value
	if fromApplied {
		applied, held := niStaticRouteAppliedEntry(inParams, reqInfo.niName, reqInfo.prefix)
		if !held {
			return nil
		}
		entry = applied
	} else if entry, err = niStaticRouteEntryFromDb(inParams.dbs[db.ConfigDB], reqInfo.niName, reqInfo.prefix); err != nil {
		return err
	}
	for _, e := range niStaticRouteDecodeNextHops(entry) {
		if niStaticRouteNhIndex(e) != reqInfo.index {
			continue
		}
		nhObj := getNiStaticRouteNextHopObj(wrappedRoot, reqInfo.niName, reqInfo.protoName, reqInfo.prefix, reqInfo.index, true)
		if nhObj == nil {
			return nil
		}
		side := "config"
		if fromApplied {
			side = "state"
		}
		niStaticRouteNhSubToYang(nhObj, e, side)
		niStaticRouteNhSubTrim(nhObj, reqInfo)
		return nil
	}
	if fromApplied {
		return nil
	}
	return tlerr.NotFoundError{Format: fmt.Sprintf("static route %s has no next-hop %s", reqInfo.prefix, reqInfo.index)}
}

// niStaticRouteNhSubTrim keeps only what a GET below a next-hop asked for.
func niStaticRouteNhSubTrim(nhObj *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_NextHops_NextHop, reqInfo *niStaticRouteReqInfo) {
	if reqInfo.subNode == niStaticRouteInterfaceRefCnt {
		nhObj.EnableBfd = nil
		if ir := nhObj.InterfaceRef; ir != nil {
			if reqInfo.subSide == "config" {
				ir.State = nil
			}
			if reqInfo.subSide == "state" {
				ir.Config = nil
			}
			switch reqInfo.targetLeaf {
			case "interface":
				if ir.Config != nil {
					ir.Config.Subinterface = nil
				}
				if ir.State != nil {
					ir.State.Subinterface = nil
				}
			case "subinterface":
				if ir.Config != nil {
					ir.Config.Interface = nil
				}
				if ir.State != nil {
					ir.State.Interface = nil
				}
			}
		}
		return
	}
	nhObj.InterfaceRef = nil
	if eb := nhObj.EnableBfd; eb != nil {
		if reqInfo.subSide == "config" {
			eb.State = nil
		}
		if reqInfo.subSide == "state" {
			eb.Config = nil
		}
	}
}

func init() {
	for _, p := range []string{"interface-ref/config/interface", "interface-ref/config/subinterface", "enable-bfd/config/enabled"} {
		niStaticRouteMappedPayloadLeaves["static/next-hops/next-hop/"+p] = true
	}
}
