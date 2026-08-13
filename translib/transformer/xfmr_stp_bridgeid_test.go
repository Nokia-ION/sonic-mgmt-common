////////////////////////////////////////////////////////////////////////////////
//                                                                            //
//  Copyright 2026 Nokia.                                                     //
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

import "testing"

func TestBridgeIdToMacPriorityDottedFormat(t *testing.T) {
	priority, mac := bridgeIdToMacPriority("8000.781f.7cfc.b617")
	if priority != 32768 {
		t.Fatalf("expected priority 32768, got %d", priority)
	}
	if mac != "78:1f:7c:fc:b6:17" {
		t.Fatalf("expected mac 78:1f:7c:fc:b6:17, got %q", mac)
	}
}

func TestBridgeIdMacToOcFormatDottedBridgeId(t *testing.T) {
	mac := bridgeIdMacToOcFormat("8000.781f.7cfc.cf17")
	if mac != "78:1f:7c:fc:cf:17" {
		t.Fatalf("expected mac 78:1f:7c:fc:cf:17, got %q", mac)
	}
}

func TestBridgeIdMacToOcFormatPlainHexBridgeId(t *testing.T) {
	mac := bridgeIdMacToOcFormat("8064b86a97e24e9c")
	if mac != "b8:6a:97:e2:4e:9c" {
		t.Fatalf("expected mac b8:6a:97:e2:4e:9c, got %q", mac)
	}
}

func TestBridgeIdToMacPriorityPlainHexFormat(t *testing.T) {
	priority, mac := bridgeIdToMacPriority("8064b86a97e24e9c")
	if priority != 32768 {
		t.Fatalf("expected priority 32768, got %d", priority)
	}
	if mac != "b8:6a:97:e2:4e:9c" {
		t.Fatalf("expected mac b8:6a:97:e2:4e:9c, got %q", mac)
	}
}

func TestBridgeIdToMacPriorityZeroRootPriority(t *testing.T) {
	priority, mac := bridgeIdToMacPriority("0000.781f.7cfc.b617")
	if priority != 0 {
		t.Fatalf("expected priority 0, got %d", priority)
	}
	if mac != "78:1f:7c:fc:b6:17" {
		t.Fatalf("expected mac 78:1f:7c:fc:b6:17, got %q", mac)
	}
}

func TestBridgeIdToMacPriorityRemoteRoot(t *testing.T) {
	priority, mac := bridgeIdToMacPriority("0064b86a97e24e9c")
	if priority != 0 {
		t.Fatalf("expected priority 0, got %d", priority)
	}
	if mac != "b8:6a:97:e2:4e:9c" {
		t.Fatalf("expected mac b8:6a:97:e2:4e:9c, got %q", mac)
	}
}
