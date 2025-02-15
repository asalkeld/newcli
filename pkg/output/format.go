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

package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/jedib0t/go-pretty/table"
	"gopkg.in/yaml.v2"

	"github.com/nitrictech/cli/pkg/pflagext"
)

var (
	allowedFormats = []string{"json", "yaml", "table"}
	defaultFormat  = "table"
	outputFormat   string
	OutputTypeFlag = pflagext.NewStringEnumVar(&outputFormat, allowedFormats, defaultFormat)
)

func Print(object interface{}) {
	switch outputFormat {
	case "json":
		printJson(object)
	case "yaml":
		printYaml(object)
	default:
		printTable(object)
	}
}

func printJson(object interface{}) {
	b, err := json.MarshalIndent(object, "", "  ")
	if err != nil {
		panic(err)
	}
	fmt.Print(string(b))
}

func printYaml(object interface{}) {
	b, err := yaml.Marshal(object)
	if err != nil {
		panic(err)
	}
	fmt.Print(string(b))
}

func printTable(object interface{}) {
	ro := reflect.TypeOf(object)

	switch ro.Kind() {
	case reflect.Map:
		printMap(object, os.Stdout)
	case reflect.Array, reflect.Slice:
		printList(object, os.Stdout)
	case reflect.Struct:
		printStruct(object, os.Stdout)
	default:
		spew.Dump(object)
	}
}

func tags(f reflect.StructField) []string {
	if f.Tag != "" {
		for _, tName := range []string{"yaml", "json"} {
			if f.Tag.Get(tName) != "" {
				return strings.Split(f.Tag.Get(tName), ",")
			}
		}
	}

	return nil
}

func nameFromField(f reflect.StructField) string {
	t := tags(f)
	if len(t) > 0 {
		return t[0]
	}
	return ""
}

func namesFrom(t reflect.Type) table.Row {
	names := []interface{}{}

	switch t.Kind() {
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			name := nameFromField(t.Field(i))
			if name != "" {
				names = append(names, name)
			}
		}
	case reflect.Slice, reflect.Array, reflect.Func, reflect.Chan, reflect.Interface, reflect.Map:
		// not yet supported
	default:
		names = append(names, "value")
	}

	return names
}

// printList will print something like the following:
// +--------------+-----------------+--------+--------------------------------+
// | ID           | REPOSITORY      | TAG    | CREATEDAT                      |
// +--------------+-----------------+--------+--------------------------------+
// | 6e83378b322a | go-create-local | latest | 2022-01-07 15:19:01 +1000 AEST |
// | 49e64c2fd5c1 | go-read-local   | latest | 2022-01-07 15:19:18 +1000 AEST |
// | ea9f8d14df25 | go-list-local   | latest | 2022-01-07 15:18:44 +1000 AEST |
// +--------------+-----------------+--------+--------------------------------+
func printList(object interface{}, out io.Writer) {
	tab := table.NewWriter()
	tab.SetOutputMirror(out)

	t := reflect.TypeOf(object)
	names := namesFrom(t.Elem())
	if len(names) > 0 {
		tab.AppendHeader(names)
	}
	rows := []table.Row{}
	v := reflect.ValueOf(object)

	for i := 0; i < v.Len(); i++ {
		switch v.Index(i).Kind() {
		case reflect.Struct:
			row := table.Row{}
			for fi := 0; fi < v.Index(i).NumField(); fi++ {
				if len(row) < len(names) {
					if v.Index(i).Field(fi).Kind() == reflect.Ptr {
						row = append(row, v.Index(i).Field(fi).Elem())
					} else {
						row = append(row, v.Index(i).Field(fi))
					}
				}
			}
			rows = append(rows, row)
		case reflect.Slice, reflect.Array, reflect.Func, reflect.Chan, reflect.Interface, reflect.Map:
			// not yet supported
		default:
			// simple types
			rows = append(rows, table.Row{v.Index(i)})
		}
	}
	tab.AppendRows(rows)
	tab.Render()
}

// printMap will print something like the following:
// +----------+-------------+----------+--------+
// | KEY      | NAME        | PROVIDER | REGION |
// +----------+-------------+----------+--------+
// | local    | default     | local    |        |
// | test-app | super-duper | aws      | eastus |
// +----------+-------------+----------+--------+
func printMap(object interface{}, out io.Writer) {
	tab := table.NewWriter()
	tab.SetOutputMirror(out)

	names := namesFrom(reflect.TypeOf(object).Elem())
	tab.AppendHeader(append(table.Row{"key"}, names...))

	value := reflect.ValueOf(object)
	iter := value.MapRange()

	rows := []table.Row{}

	keyList := []reflect.Value{}
	for iter.Next() {
		keyList = append(keyList, iter.Key())
	}
	sort.SliceStable(keyList, func(i, j int) bool {
		return keyList[i].String() < keyList[j].String()
	})
	for _, k := range keyList {
		v := value.MapIndex(k)

		switch v.Kind() {
		case reflect.Struct:
			row := table.Row{k}
			for fi := 0; fi < v.NumField(); fi++ {
				if len(row) <= len(names) {
					row = append(row, v.Field(fi))
				}
			}
			rows = append(rows, row)
		case reflect.Slice, reflect.Array, reflect.Func, reflect.Chan, reflect.Interface, reflect.Map:
			// not yet supported
		default:
			// simple types
			rows = append(rows, table.Row{k, v})
		}
	}
	tab.AppendRows(rows)
	tab.Render()
}

// printStruct will print something like the following:
//+------------+--------------------------------+
//| ID         | 6e83378b322a                   |
//| REPOSITORY | go-create-local                |
//| TAG        | latest                         |
//| CREATEDAT  | 2022-01-07 15:19:01 +1000 AEST |
//+------------+--------------------------------+
func printStruct(object interface{}, out io.Writer) {
	tab := table.NewWriter()
	tab.SetOutputMirror(out)

	rows := []table.Row{}
	v := reflect.ValueOf(object)
	t := reflect.TypeOf(object)
	for fi := 0; fi < v.NumField(); fi++ {
		name := nameFromField(t.Field(fi))
		if name != "" {
			rows = append(rows, table.Row{strings.ToUpper(name), v.Field(fi)})
		}
	}

	tab.AppendRows(rows)
	tab.Render()
}
