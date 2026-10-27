package transformer

import (
	"reflect"
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

func cpAclDevice(setName string, bindInterfaces ...string) *ocbinds.Device {
	d := &ocbinds.Device{}
	ygot.BuildEmptyTree(d)
	ygot.BuildEmptyTree(d.System)
	ygot.BuildEmptyTree(d.System.ControlPlaneTraffic)
	ygot.BuildEmptyTree(d.System.ControlPlaneTraffic.Ingress)
	aclSet, _ := d.System.ControlPlaneTraffic.Ingress.Acl.NewAclSet(setName, ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV4)
	ygot.BuildEmptyTree(aclSet)
	aclSet.Config.SetName = ygot.String(setName)
	aclSet.Config.Type = ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV4
	aclSet.Config.BindInterfaces = bindInterfaces
	return d
}

func TestYangToDb_CpAcl_BindInterfaces(t *testing.T) {
	const uri = "/openconfig-system:system/openconfig-system-controlplane:control-plane-traffic/ingress/acl/acl-set[set-name=CP-ACL][type=ACL_IPV4]"
	const aclKey = "CP_ACL_ACL_IPV4"

	tests := []struct {
		name    string
		op      Operation
		device  *ocbinds.Device
		wantErr bool
		wantMap map[string]map[string]db.Value
	}{
		{name: "UPDATE without bind-interfaces is rejected", op: UPDATE, device: cpAclDevice("CP-ACL"), wantErr: true},
		{name: "REPLACE without bind-interfaces is rejected", op: REPLACE, device: cpAclDevice("CP-ACL"), wantErr: true},
		{
			name:   "UPDATE with bind-interfaces binds the ports",
			op:     UPDATE,
			device: cpAclDevice("CP-ACL", "Ethernet8"),
			wantMap: map[string]map[string]db.Value{
				cpAclBindTable: {aclKey: {Field: map[string]string{"ports@": "Ethernet8"}}},
				cpAclTable: {aclKey: {Field: map[string]string{
					cpAclTypeField: "L3", cpAclStageField: cpAclStage, "ports@": "Ethernet8"}}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ygRoot := ygot.GoStruct(tt.device)
			inParams := XfmrParams{
				ygRoot:       &ygRoot,
				oper:         tt.op,
				uri:          uri,
				requestUri:   uri,
				subOpDataMap: make(map[Operation]*map[db.DBNum]map[string]map[string]db.Value),
			}

			gotMap, err := YangToDb_cp_acl_xfmr(inParams)
			if tt.wantErr {
				if _, ok := err.(tlerr.InvalidArgsError); !ok {
					t.Fatalf("YangToDb_cp_acl_xfmr() error = %v (%T), want InvalidArgsError", err, err)
				}
				if len(gotMap) != 0 {
					t.Fatalf("YangToDb_cp_acl_xfmr() wrote %v on a rejected request", gotMap)
				}
				return
			}
			if err != nil {
				t.Fatalf("YangToDb_cp_acl_xfmr() error = %v", err)
			}
			if !reflect.DeepEqual(gotMap, tt.wantMap) {
				t.Fatalf("YangToDb_cp_acl_xfmr() = %v, want %v", gotMap, tt.wantMap)
			}
		})
	}
}
