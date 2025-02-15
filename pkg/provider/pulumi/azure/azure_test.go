// Copyright Nitric Pty Ltd.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at:
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package azure

import (
	"reflect"
	"testing"

	"github.com/nitrictech/cli/pkg/provider/pulumi/common"
)

func Test_azureProvider_Plugins(t *testing.T) {
	want := []common.Plugin{
		{Name: "azure-native", Version: "v1.60.0"},
		{Name: "azure", Version: "v4.39.0"},
		{Name: "azuread", Version: "v5.17.0"},
	}
	got := (&azureProvider{}).Plugins()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("azureProvider.Plugins() = %v, want %v", got, want)
	}
}
