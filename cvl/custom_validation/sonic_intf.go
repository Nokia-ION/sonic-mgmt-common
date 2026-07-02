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

package custom_validation

import (
	"strconv"
	"strings"

	util "github.com/Azure/sonic-mgmt-common/cvl/internal/util"
)

// How will the total number of PortChannels change after this update?
func calcPortChannelDelta(vc *CustValidationCtxt) int {
	delta := 0
	for _, curCfg := range vc.ReqData {
		keys := strings.Split(curCfg.Key, "|")
		if len(keys) > 0 && keys[0] == "PORTCHANNEL" {
			if curCfg.VOp == OP_CREATE {
				delta += 1
			} else if curCfg.VOp == OP_DELETE {
				delta -= 1
			}
		}
	}
	return delta
}

// ValidatePortChannelCreationDeletion Custom validation for PortChannel creation or deletion
func (t *CustomValidation) ValidatePortChannelCreationDeletion(vc *CustValidationCtxt) CVLErrorInfo {
	if vc.CurCfg.VOp == OP_CREATE {

		keys := strings.Split(vc.CurCfg.Key, "|")
		if len(keys) > 0 {
			if keys[0] == "PORTCHANNEL" {
				poKeys, err := vc.RClient.Keys("PORTCHANNEL" + "|*").Result()
				if err != nil {
					return CVLErrorInfo{ErrCode: CVL_SEMANTIC_KEY_NOT_EXIST}
				}

				if len(poKeys) >= 128 {
					util.TRACE_LEVEL_LOG(util.TRACE_SEMANTIC, "Maximum number of portchannels already created.")
					return CVLErrorInfo{
						ErrCode:          CVL_SEMANTIC_ERROR,
						TableName:        "PORTCHANNEL",
						Keys:             strings.Split(vc.CurCfg.Key, "|"),
						ConstraintErrMsg: "Maximum number(128) of portchannels already created in the system. Cannot create new portchannel.",
						ErrAppTag:        "max-reached",
					}
				}

				delta := calcPortChannelDelta(vc)
				total := len(poKeys) + delta
				if total > 128 {
					util.TRACE_LEVEL_LOG(util.TRACE_SEMANTIC, "Cannot create more than supported number of portchannels in the system.")
					errStr := "Number of portchannels already created in the system are " + strconv.Itoa(len(poKeys)) + "can't add " + strconv.Itoa(delta) + ". Maximum number of portchannel that can be supported are 128."
					return CVLErrorInfo{
						ErrCode:          CVL_SEMANTIC_ERROR,
						TableName:        "PORTCHANNEL",
						Keys:             strings.Split(vc.CurCfg.Key, "|"),
						ConstraintErrMsg: errStr,
						ErrAppTag:        "max-reached",
					}
				}
			}
		}
	}

	return CVLErrorInfo{ErrCode: CVL_SUCCESS}
}
