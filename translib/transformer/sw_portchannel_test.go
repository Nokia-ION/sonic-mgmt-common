package transformer

import (
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/db"
)

func TestGetLagState(t *testing.T) {
	ifName := "PortChannel54321"
	lagInfoMap := map[string]db.Value{"PortChannel1": db.Value{}}
	if r := getLagState(XfmrParams{}, nil, &ifName, lagInfoMap, nil); r == nil {
		t.Fatalf("Operation did not return an error as expected")
	}
}

func TestDeleteLagIntfAndMembers(t *testing.T) {
	if err := deleteLagIntfAndMembers(nil, nil); err == nil {
		t.Fatalf("Call to deleteLagIntfAndMembers did not return an error as expected")
	}
}

func TestUint16Conv(t *testing.T) {
	if val, err := uint16Conv("12"); err != nil || val != 12 {
		t.Fatalf("Call to uint16Conv did not return expected result (val=%v, err=%v)", val, err)
	}
	if _, err := uint16Conv("string"); err == nil {
		t.Fatalf("Call to uint16Conv did not return an error as expected")
	}
}
