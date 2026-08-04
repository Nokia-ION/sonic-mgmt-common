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

package cvl_test

import (
	"testing"

	"github.com/Azure/sonic-mgmt-common/cvl"
	cmn "github.com/Azure/sonic-mgmt-common/cvl/common"
)

func TestValidateEditConfig_STP_Mst_SkipRootguardDefault(t *testing.T) {
	cfgData := []cmn.CVLEditConfigData{
		{
			VType: cmn.VALIDATE_ALL,
			VOp:   cmn.OP_CREATE,
			Key:   "STP|GLOBAL",
			Data: map[string]string{
				"mode": "mst",
			},
		},
	}

	verifyValidateEditConfig(t, cfgData, Success)
}

func TestValidateEditConfig_STP_Pvst_RootguardDefault(t *testing.T) {
	cfgData := []cmn.CVLEditConfigData{
		{
			VType: cmn.VALIDATE_ALL,
			VOp:   cmn.OP_CREATE,
			Key:   "STP|GLOBAL",
			Data: map[string]string{
				"mode": "pvst",
			},
		},
	}

	verifyValidateEditConfig(t, cfgData, Success)
}

func TestValidateEditConfig_STP_Mst_RootguardTimeout_Negative(t *testing.T) {
	cfgData := []cmn.CVLEditConfigData{
		{
			VType: cmn.VALIDATE_ALL,
			VOp:   cmn.OP_CREATE,
			Key:   "STP|GLOBAL",
			Data: map[string]string{
				"mode":              "mst",
				"rootguard_timeout": "30",
			},
		},
	}

	verifyValidateEditConfig(t, cfgData, CVLErrorInfo{
		ErrCode:          cvl.CVL_SEMANTIC_ERROR,
		TableName:        "STP",
		Field:            "rootguard_timeout",
		ConstraintErrMsg: "Root guard timeout configuration is allowed only in PVST mode",
	})
}
