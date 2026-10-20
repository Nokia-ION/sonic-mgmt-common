////////////////////////////////////////////////////////////////////////////////
//                                                                            //
//  Copyright 2026 Nokia, Inc.                                                //
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

package transformer

import (
	"reflect"
	"sort"
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/openconfig/ygot/ygot"
)

func TestDbToYangPathGrpcServerPathXfmr(t *testing.T) {
	nameKey := GRPC_SERVER + "/name"

	tests := []struct {
		name       string
		tblName    string
		tblKeyComp []string
		want       map[string]string
	}{{
		// The key a subscription wildcarding the server name has to resolve.
		name:       "certificate key names its server",
		tblName:    CREDENTIALS_TBL,
		tblKeyComp: []string{CRED_CERT_PREFIX, "gnxi"},
		want:       map[string]string{nameKey: "gnxi"},
	}, {
		// A Certz SSL profile other than the default is not a gRPC server.
		name:       "another ssl profile names no server",
		tblName:    CREDENTIALS_TBL,
		tblKeyComp: []string{CRED_CERT_PREFIX, "octa-ssl-profile"},
		want:       map[string]string{},
	}, {
		// Stored once under a key naming no server, and reported against the
		// default server, as the read path does.
		name:       "pathz policy falls to the default server",
		tblName:    CREDENTIALS_TBL,
		tblKeyComp: []string{CRED_PATHZ_PREFIX, "ACTIVE"},
		want:       map[string]string{nameKey: GNXI_ID},
	}, {
		// Leaving the key unset is what tells the caller it cannot name an
		// instance, and is reported there rather than guessed at here.
		name:       "an unrelated credential names no server",
		tblName:    CREDENTIALS_TBL,
		tblKeyComp: []string{"AUTHZ_POLICY", "gnxi"},
		want:       map[string]string{},
	}, {
		name:       "a certificate key with no name names no server",
		tblName:    CREDENTIALS_TBL,
		tblKeyComp: []string{CRED_CERT_PREFIX},
		want:       map[string]string{},
	}, {
		name:       "another table names no server",
		tblName:    MEMORY_TBL,
		tblKeyComp: []string{CRED_CERT_PREFIX, "gnxi"},
		want:       map[string]string{},
	}, {
		name:       "an empty key names no server",
		tblName:    CREDENTIALS_TBL,
		tblKeyComp: nil,
		want:       map[string]string{},
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := XfmrDbToYgPathParams{
				tblName:    tt.tblName,
				tblKeyComp: tt.tblKeyComp,
				ygPathKeys: make(map[string]string),
			}

			if err := DbToYangPath_grpc_server_path_xfmr(params); err != nil {
				t.Fatalf("DbToYangPath_grpc_server_path_xfmr() error = %v", err)
			}
			if !reflect.DeepEqual(params.ygPathKeys, tt.want) {
				t.Errorf("ygPathKeys = %v, want %v", params.ygPathKeys, tt.want)
			}
		})
	}
}

// Both CREDENTIALS rows resolve to the same grpc-server instance, so a request
// registered against a row it does not read is answered with that row's
// notification too, repeating the leaf the caller asked for.
func TestSubscribeGrpcServerXfmrRegistersOnlyRowsItReads(t *testing.T) {
	certKey := CRED_CERT_PREFIX + "|" + GNXI_ID
	pathzKey := CRED_PATHZ_PREFIX + "|" + CRED_PATHZ_ACTIVE_KEY

	// The subscribe callback is handed the module-prefixed form of the path,
	// which is what GRPC_SERVER and PATHZ_POLICY_COUNTERS are built from.
	tests := []struct {
		name       string
		uri        string
		want       []string
		wantSample bool
	}{{
		name: "a wildcarded name reads the certificate row alone",
		uri:  GRPC_SERVER + "/state/name",
		want: []string{certKey},
	}, {
		name: "a named server reads the certificate row alone",
		uri:  GRPC_OC_SERVERS + "/grpc-server[name=gnxi]/state/name",
		want: []string{certKey},
	}, {
		name: "the certificate-id leaf reads the certificate row alone",
		uri:  GRPC_OC_SERVERS + "/grpc-server[name=gnxi]/config/certificate-id",
		want: []string{certKey},
	}, {
		// Counters are sampled rather than reported on change.
		name:       "the counters read the pathz row alone",
		uri:        PATHZ_POLICY_COUNTERS + "/paths/path",
		want:       []string{pathzKey},
		wantSample: true,
	}, {
		// Spanning both, so it is driven by the certificate row rather than
		// repeating every leaf once per row.
		name: "the whole server subtree reads the certificate row alone",
		uri:  GRPC_SERVER,
		want: []string{certKey},
	}, {
		name: "the servers container reads the certificate row alone",
		uri:  GRPC_OC_SERVERS,
		want: []string{certKey},
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := Subscribe_grpc_server_xfmr(XfmrSubscInParams{uri: tt.uri})
			if err != nil {
				t.Fatalf("Subscribe_grpc_server_xfmr() error = %v", err)
			}
			got := out.dbDataMap[db.StateDB][CREDENTIALS_TBL]
			if len(got) != len(tt.want) {
				t.Fatalf("registered rows = %v, want %v", keysOf(got), tt.want)
			}
			for _, key := range tt.want {
				if _, ok := got[key]; !ok {
					t.Errorf("row %q not registered; got %v", key, keysOf(got))
				}
			}
			if out.nOpts == nil {
				t.Fatalf("no notification options returned")
			}
			if gotSample := out.nOpts.pType == Sample; gotSample != tt.wantSample {
				t.Errorf("sampled = %v, want %v", gotSample, tt.wantSample)
			}
		})
	}
}

func TestGrpcServerNameFromUri(t *testing.T) {
	tests := []struct {
		name    string
		uri     string
		want    string
		wantErr bool
	}{{
		name: "an unset key addresses the one server",
		uri:  GRPC_SERVER + "/config/port",
		want: GNXI_ID,
	}, {
		name: "a wildcard addresses the one server",
		uri:  GRPC_OC_SERVERS + "/grpc-server[name=*]/config/port",
		want: GNXI_ID,
	}, {
		name: "the server by name",
		uri:  GRPC_OC_SERVERS + "/grpc-server[name=gnxi]/config/port",
		want: GNXI_ID,
	}, {
		name:    "an ssl profile is not a server",
		uri:     GRPC_OC_SERVERS + "/grpc-server[name=octa-ssl-profile]/config/port",
		wantErr: true,
	}, {
		name:    "the OC default name is not this server",
		uri:     GRPC_OC_SERVERS + "/grpc-server[name=DEFAULT]/config/port",
		wantErr: true,
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := grpcServerNameFromUri(NewPathInfo(tt.uri))
			if (err != nil) != tt.wantErr {
				t.Fatalf("grpcServerNameFromUri() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("grpcServerNameFromUri() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestYangToDbGrpcServerEnable(t *testing.T) {
	port := uint16(50052)
	tests := []struct {
		name    string
		enable  bool
		want    map[string]map[string]db.Value
		wantErr bool
	}{{
		name:   "enabling the running server is accepted",
		enable: true,
		want: map[string]map[string]db.Value{
			GNMI_CFG_TBL: {GNMI_CFG_KEY: {Field: map[string]string{"port": "50052"}}},
		},
	}, {
		name:    "disabling the server is rejected",
		enable:  false,
		wantErr: true,
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enable := tt.enable
			d := &ocbinds.Device{System: &ocbinds.OpenconfigSystem_System{
				GrpcServers: &ocbinds.OpenconfigSystem_System_GrpcServers{},
			}}
			srv, err := d.System.GrpcServers.NewGrpcServer(GNXI_ID)
			if err != nil {
				t.Fatalf("NewGrpcServer() error = %v", err)
			}
			srv.Config = &ocbinds.OpenconfigSystem_System_GrpcServers_GrpcServer_Config{Enable: &enable, Port: &port}
			ygRoot := ygot.GoStruct(d)
			got, err := YangToDb_grpc_server_xfmr(XfmrParams{
				ygRoot: &ygRoot,
				oper:   UPDATE,
				uri:    GRPC_OC_SERVERS + "/grpc-server[name=gnxi]/config",
			})
			if (err != nil) != tt.wantErr {
				t.Fatalf("YangToDb_grpc_server_xfmr() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("YangToDb_grpc_server_xfmr() = %v, want %v", got, tt.want)
			}
		})
	}
}

func keysOf(m map[string]map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
