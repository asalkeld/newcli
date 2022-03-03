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

package stack

import (
	"log"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"

	"github.com/nitrictech/cli/pkg/build"
	"github.com/nitrictech/cli/pkg/codeconfig"
	"github.com/nitrictech/cli/pkg/output"
	"github.com/nitrictech/cli/pkg/provider"
	"github.com/nitrictech/cli/pkg/provider/types"
	"github.com/nitrictech/cli/pkg/stack"
	"github.com/nitrictech/cli/pkg/target"
	"github.com/nitrictech/cli/pkg/tasklet"
)

var stackName string

var stackCmd = &cobra.Command{
	Use:   "stack",
	Short: "Manage stacks",
	Long: `Manage stacks.

A stack is a named update target, and a single project may have many of them.

The stack commands generally need 3 things:
1. a target (either explicitly with "-t <targetname> or defined in the config)
2. a name (either explicitly with -n <stack name> or use the default name of "s")
3. a project definition, this automatically collected from the code in functions.
   A glob to the functions can be a supplied by:
  - Configuration - there are default globs for each supported language in the .nitiric-config.yaml
  - Arguments to the stack actions.
	`,
	Example: `nitric stack up
nitric stack down
nitric stack list
`,
}

var stackUpdateCmd = &cobra.Command{
	Use:   "update [handlerGlob]",
	Short: "Deploy code to a cloud and/or update resource changes",
	Long:  `Deploy a Nitric stack.`,
	Example: `# Configured default handlerGlob (project in the current directory).
nitric stack up -t aws

# use an explicit handlerGlob (project in the current directory)
nitric stack up -t aws "functions/*/*.go"

# use an explicit handlerGlob and explicit project directory
nitric stack up -s ../projectX -t aws "functions/*/*.go"

# use a custom stack name
nitric stack up -n prod -t aws`,
	Run: func(cmd *cobra.Command, args []string) {
		t, err := target.FromOptions()
		cobra.CheckErr(err)
		s, err := stack.FromOptions(args)
		cobra.CheckErr(err)

		log.SetOutput(output.NewPtermWriter(pterm.Debug))

		codeAsConfig := tasklet.Runner{
			StartMsg: "Gathering configuration from code..",
			Runner: func(_ output.Progress) error {
				s, err = codeconfig.Populate(s)
				return err
			},
			StopMsg: "Configuration gathered",
		}
		tasklet.MustRun(codeAsConfig, tasklet.Opts{})

		p, err := provider.NewProvider(s, t)
		cobra.CheckErr(err)

		buildImages := tasklet.Runner{
			StartMsg: "Building Images",
			Runner: func(_ output.Progress) error {
				return build.Create(s, t)
			},
			StopMsg: "Images built",
		}
		tasklet.MustRun(buildImages, tasklet.Opts{})

		d := &types.Deployment{}
		deploy := tasklet.Runner{
			StartMsg: "Deploying..",
			Runner: func(progress output.Progress) error {
				d, err = p.Apply(progress, stackName)
				return err
			},
			StopMsg: "Stack",
		}
		tasklet.MustRun(deploy, tasklet.Opts{SuccessPrefix: "Deployed"})

		rows := [][]string{{"API", "Endpoint"}}
		for k, v := range d.ApiEndpoints {
			rows = append(rows, []string{k, v})
		}
		_ = pterm.DefaultTable.WithBoxed().WithData(rows).Render()
	},
	Args:    cobra.MinimumNArgs(0),
	Aliases: []string{"up"},
}

var stackDeleteCmd = &cobra.Command{
	Use:   "down",
	Short: "Destroy an existing stack and its resources from the cloud",
	Long: `Destroy an existing stack and its resources

This command deletes an entire existing stack by name.  After running to completion,
all of this stack's resources and associated state will be gone.

Warning: this command is generally irreversible and should be used with great care.`,
	Example: `nitric project down
nitric project down -s ../project/ -t prod
nitric project down -n prod-aws -s ../project/ -t prod
`,
	Run: func(cmd *cobra.Command, args []string) {
		t, err := target.FromOptions()
		cobra.CheckErr(err)

		s, err := stack.FromOptionsMinimal()
		cobra.CheckErr(err)

		p, err := provider.NewProvider(s, t)
		cobra.CheckErr(err)

		deploy := tasklet.Runner{
			StartMsg: "Deleting..",
			Runner: func(progress output.Progress) error {
				return p.Delete(progress, stackName)
			},
			StopMsg: "Stack",
		}
		tasklet.MustRun(deploy, tasklet.Opts{
			SuccessPrefix: "Deleted",
		})
	},
	Args: cobra.ExactArgs(0),
}

var stackListCmd = &cobra.Command{
	Use:   "list",
	Short: "list stacks for a project",
	Long:  `Lists Nitric application stacks for a project.`,
	Example: `nitric list
nitric stack list -s ../project/ -t prod
`,
	Run: func(cmd *cobra.Command, args []string) {
		t, err := target.FromOptions()
		cobra.CheckErr(err)

		s, err := stack.FromOptionsMinimal()
		cobra.CheckErr(err)

		p, err := provider.NewProvider(s, t)
		cobra.CheckErr(err)

		deps, err := p.List()
		cobra.CheckErr(err)

		output.Print(deps)
	},
	Args:    cobra.ExactArgs(0),
	Aliases: []string{"ls"},
}

func RootCommand() *cobra.Command {
	stackCmd.AddCommand(stackUpdateCmd)
	stackUpdateCmd.Flags().StringVarP(&stackName, "name", "n", "dep", "the name of the stack")
	cobra.CheckErr(target.AddOptions(stackUpdateCmd, false))
	stack.AddOptions(stackUpdateCmd)

	stackCmd.AddCommand(stackDeleteCmd)
	stackDeleteCmd.Flags().StringVarP(&stackName, "name", "n", "dep", "the name of the stack")
	cobra.CheckErr(target.AddOptions(stackDeleteCmd, false))
	stack.AddOptions(stackDeleteCmd)

	stackCmd.AddCommand(stackListCmd)
	stack.AddOptions(stackListCmd)
	cobra.CheckErr(target.AddOptions(stackListCmd, false))
	return stackCmd
}
