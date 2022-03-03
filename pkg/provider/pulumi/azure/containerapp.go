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
	"github.com/pkg/errors"
	web "github.com/pulumi/pulumi-azure-native/sdk/go/azure/web/v20210301"
	"github.com/pulumi/pulumi-azure/sdk/v4/go/azure/authorization"
	"github.com/pulumi/pulumi-azure/sdk/v4/go/azure/containerservice"
	"github.com/pulumi/pulumi-azure/sdk/v4/go/azure/eventgrid"
	"github.com/pulumi/pulumi-azure/sdk/v4/go/azure/operationalinsights"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/nitrictech/cli/pkg/project"
	"github.com/nitrictech/cli/pkg/provider/pulumi/common"
)

type ContainerAppsArgs struct {
	ResourceGroupName pulumi.StringInput
	Location          pulumi.StringInput
	SubscriptionID    pulumi.StringInput

	Topics map[string]*eventgrid.Topic

	KVaultName                    pulumi.StringInput
	StorageAccountBlobEndpoint    pulumi.StringInput
	StorageAccountQueueEndpoint   pulumi.StringInput
	MongoDatabaseName             pulumi.StringInput
	MongoDatabaseConnectionString pulumi.StringInput
}

type ContainerApps struct {
	pulumi.ResourceState

	Name     string
	Registry *containerservice.Registry
	Apps     map[string]*ContainerApp
}

func (a *azureProvider) newContainerApps(ctx *pulumi.Context, name string, args *ContainerAppsArgs, opts ...pulumi.ResourceOption) (*ContainerApps, error) {
	res := &ContainerApps{
		Name: name,
		Apps: map[string]*ContainerApp{},
	}
	err := ctx.RegisterComponentResource("nitric:func:ContainerApps", name, res, opts...)
	if err != nil {
		return nil, err
	}

	env := web.EnvironmentVarArray{}

	if args.StorageAccountBlobEndpoint != nil {
		env = append(env, web.EnvironmentVarArgs{
			Name:  pulumi.String("AZURE_STORAGE_ACCOUNT_BLOB_ENDPOINT"),
			Value: args.StorageAccountBlobEndpoint,
		})
	}

	if args.StorageAccountQueueEndpoint != nil {
		env = append(env, web.EnvironmentVarArgs{
			Name:  pulumi.String("AZURE_STORAGE_ACCOUNT_QUEUE_ENDPOINT"),
			Value: args.StorageAccountQueueEndpoint,
		})
	}

	if args.MongoDatabaseConnectionString != nil {
		env = append(env, web.EnvironmentVarArgs{
			Name:  pulumi.String("MONGODB_CONNECTION_STRING"),
			Value: args.MongoDatabaseConnectionString,
		})
	}

	if args.MongoDatabaseName != nil {
		env = append(env, web.EnvironmentVarArgs{
			Name:  pulumi.String("MONGODB_DATABASE"),
			Value: args.MongoDatabaseName,
		})
	}

	if args.KVaultName != nil {
		env = append(env, web.EnvironmentVarArgs{
			Name:  pulumi.String("KVAULT_NAME"),
			Value: args.KVaultName,
		})
	}

	res.Registry, err = containerservice.NewRegistry(ctx, resourceName(ctx, name, RegistryRT), &containerservice.RegistryArgs{
		ResourceGroupName: args.ResourceGroupName,
		Location:          args.Location,
		AdminEnabled:      pulumi.BoolPtr(true),
		Sku:               pulumi.String("Basic"),
	}, pulumi.Parent(res))
	if err != nil {
		return nil, err
	}

	aw, err := operationalinsights.NewAnalyticsWorkspace(ctx, resourceName(ctx, name, AnalyticsWorkspaceRT), &operationalinsights.AnalyticsWorkspaceArgs{
		Location:          args.Location,
		ResourceGroupName: args.ResourceGroupName,
		Sku:               pulumi.String("PerGB2018"),
		RetentionInDays:   pulumi.Int(30),
	}, pulumi.Parent(res))
	if err != nil {
		return nil, err
	}

	kube, err := web.NewKubeEnvironment(ctx, resourceName(ctx, name, KubeRT), &web.KubeEnvironmentArgs{
		Location:          args.Location,
		ResourceGroupName: args.ResourceGroupName,
		EnvironmentType:   pulumi.String("Managed"),
		AppLogsConfiguration: web.AppLogsConfigurationArgs{
			Destination: pulumi.String("log-analytics"),
			LogAnalyticsConfiguration: web.LogAnalyticsConfigurationArgs{
				SharedKey:  aw.PrimarySharedKey,
				CustomerId: aw.WorkspaceId,
			},
		},
		Tags: common.Tags(ctx, ctx.Stack()+"Kube"),
	}, pulumi.Parent(res))
	if err != nil {
		return nil, err
	}

	for _, c := range a.s.Computes() {
		localImageName := c.ImageTagName(a.s, "")
		repositoryUrl := res.Registry.LoginServer.ApplyT(func(server string) string {
			return server + "/" + c.ImageTagName(a.s, a.t.Provider)
		}).(pulumi.StringOutput)

		image, err := common.NewImage(ctx, c.Unit().Name+"Image", &common.ImageArgs{
			LocalImageName:  localImageName,
			SourceImageName: c.ImageTagName(a.s, a.t.Provider),
			RepositoryUrl:   repositoryUrl,
			Username:        res.Registry.AdminUsername,
			Password:        res.Registry.AdminPassword,
			Server:          res.Registry.LoginServer,
			TempDir:         a.tmpDir}, pulumi.Parent(res))
		if err != nil {
			return nil, errors.WithMessage(err, "function image tag "+c.Unit().Name)
		}

		res.Apps[c.Unit().Name], err = a.newContainerApp(ctx, c.Unit().Name, &ContainerAppArgs{
			ResourceGroupName: args.ResourceGroupName,
			Location:          args.Location,
			SubscriptionID:    args.SubscriptionID,
			Registry:          res.Registry,
			KubeEnv:           kube,
			ImageUri:          image.DockerImage.ImageName,
			Env:               env,
			Topics:            args.Topics,
			Compute:           c,
		}, pulumi.Parent(res))
		if err != nil {
			return nil, err
		}
	}

	return res, nil
}

type ContainerAppArgs struct {
	ResourceGroupName pulumi.StringInput
	Location          pulumi.StringInput
	SubscriptionID    pulumi.StringInput
	Registry          *containerservice.Registry
	KubeEnv           *web.KubeEnvironment
	ImageUri          pulumi.StringInput
	Env               web.EnvironmentVarArray
	Compute           project.Compute
	Topics            map[string]*eventgrid.Topic
}

type ContainerApp struct {
	pulumi.ResourceState

	Name          string
	Sp            *SevicePrinciple
	App           *web.ContainerApp
	Subscriptions map[string]*eventgrid.Topic
}

// Built in role definitions for Azure
// See below URL for mapping
// https://docs.microsoft.com/en-us/azure/role-based-access-control/built-in-roles
var RoleDefinitions = map[string]string{
	"KVSecretsOfficer":    "b86a8fe4-44ce-4948-aee5-eccb2c155cd7",
	"BlobDataContrib":     "ba92f5b4-2d11-453d-a403-e96b0029c9fe",
	"QueueDataContrib":    "974c5e8b-45b9-4653-ba55-5f855dd0fb88",
	"EventGridDataSender": "d5a91429-5739-47e2-a06b-3470a27159e7",
}

func (a *azureProvider) newContainerApp(ctx *pulumi.Context, name string, args *ContainerAppArgs, opts ...pulumi.ResourceOption) (*ContainerApp, error) {
	res := &ContainerApp{
		Name:          name,
		Subscriptions: map[string]*eventgrid.Topic{},
	}
	err := ctx.RegisterComponentResource("nitric:func:ContainerApp", name, res, opts...)
	if err != nil {
		return nil, err
	}

	res.Sp, err = newSevicePrinciple(ctx, name, &SevicePrincipleArgs{}, pulumi.Parent(res))
	if err != nil {
		return nil, err
	}

	scope := pulumi.Sprintf("%s/resourceGroups/%s", args.SubscriptionID, args.ResourceGroupName)

	// Assign roles to the new SP
	for defName, id := range RoleDefinitions {
		_ = ctx.Log.Info("Assignment "+resourceName(ctx, name+defName, AssignmentRT)+" roleDef "+id, &pulumi.LogArgs{Ephemeral: true})

		_, err = authorization.NewAssignment(ctx, resourceName(ctx, name+defName, AssignmentRT), &authorization.AssignmentArgs{
			PrincipalId:      res.Sp.ServicePrincipalId,
			RoleDefinitionId: pulumi.Sprintf("%s/providers/Microsoft.Authorization/roleDefinitions/%s", args.SubscriptionID, id),
			Scope:            scope,
		}, pulumi.Parent(res))
		if err != nil {
			return nil, err
		}
	}

	env := web.EnvironmentVarArray{
		web.EnvironmentVarArgs{
			Name:  pulumi.String("AZURE_SUBSCRIPTION_ID"),
			Value: args.SubscriptionID,
		},
		web.EnvironmentVarArgs{
			Name:      pulumi.String("AZURE_CLIENT_ID"),
			SecretRef: pulumi.String("client-id"),
		},
		web.EnvironmentVarArgs{
			Name:      pulumi.String("AZURE_TENANT_ID"),
			SecretRef: pulumi.String("tenant-id"),
		},
		web.EnvironmentVarArgs{
			Name:      pulumi.String("AZURE_CLIENT_SECRET"),
			SecretRef: pulumi.String("client-secret"),
		},
		web.EnvironmentVarArgs{
			Name:  pulumi.String("TOLERATE_MISSING_SERVICES"),
			Value: pulumi.String("true"),
		},
	}

	//memory := common.IntValueOrDefault(args.Compute.Unit().Memory, 128)
	// we can't define memory without defining the cpu..
	res.App, err = web.NewContainerApp(ctx, resourceName(ctx, name, ContainerAppRT), &web.ContainerAppArgs{
		ResourceGroupName: args.ResourceGroupName,
		Location:          args.Location,
		KubeEnvironmentId: args.KubeEnv.ID(),
		Configuration: web.ConfigurationArgs{
			Ingress: web.IngressArgs{
				External:   pulumi.BoolPtr(true),
				TargetPort: pulumi.Int(9001),
			},
			Registries: web.RegistryCredentialsArray{
				web.RegistryCredentialsArgs{
					Server:            args.Registry.LoginServer,
					Username:          args.Registry.AdminUsername,
					PasswordSecretRef: pulumi.String("pwd"),
				},
			},
			Secrets: web.SecretArray{
				web.SecretArgs{
					Name:  pulumi.String("pwd"),
					Value: args.Registry.AdminPassword,
				},
				web.SecretArgs{
					Name:  pulumi.String("client-id"),
					Value: res.Sp.ClientID,
				},
				web.SecretArgs{
					Name:  pulumi.String("tenant-id"),
					Value: res.Sp.TenantID,
				},
				web.SecretArgs{
					Name:  pulumi.String("client-secret"),
					Value: res.Sp.ClientSecret,
				},
			},
		},
		Tags: common.Tags(ctx, name),
		Template: web.TemplateArgs{
			Containers: web.ContainerArray{
				web.ContainerArgs{
					Name:  pulumi.String("myapp"),
					Image: args.ImageUri,
					Env:   append(env, args.Env...),
				},
			},
		},
	}, pulumi.Parent(res))
	if err != nil {
		return nil, err
	}

	// Determine required subscriptions so they can be setup once the container starts
	for _, t := range args.Compute.Unit().Triggers.Topics {
		topic, ok := args.Topics[t]
		if ok {
			res.Subscriptions[t] = topic
		}
	}

	return res, ctx.RegisterResourceOutputs(res, pulumi.Map{
		"name":         pulumi.StringPtr(res.Name),
		"containerApp": res.App,
		//"subscriptions": res.Subscriptions,
	})
}
