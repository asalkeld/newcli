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

package common

import (
	"io/ioutil"

	"github.com/pulumi/pulumi-docker/sdk/v3/go/docker"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type ImageArgs struct {
	LocalImageName  string
	SourceImageName string
	RepositoryUrl   pulumi.StringInput
	TempDir         string
	Server          pulumi.StringInput
	Username        pulumi.StringInput
	Password        pulumi.StringInput
}

type Image struct {
	pulumi.ResourceState

	Name        string
	DockerImage *docker.Image
}

func NewImage(ctx *pulumi.Context, name string, args *ImageArgs, opts ...pulumi.ResourceOption) (*Image, error) {
	res := &Image{Name: name}
	err := ctx.RegisterComponentResource("nitric:Image", name, res, opts...)
	if err != nil {
		return nil, err
	}

	dummyDockerFilePath, err := ioutil.TempFile(args.TempDir, "*.dockerfile")
	if err != nil {
		return nil, err
	}
	_, err = dummyDockerFilePath.WriteString("FROM " + args.SourceImageName + "\n")
	if err != nil {
		return nil, err
	}

	imageArgs := &docker.ImageArgs{
		ImageName: args.RepositoryUrl,
		Build: docker.DockerBuildArgs{
			Env: pulumi.StringMap{
				"DOCKER_BUILDKIT": pulumi.String("1"),
			},
			Dockerfile: pulumi.String(dummyDockerFilePath.Name()),
		},
		Registry: docker.ImageRegistryArgs{
			Server:   args.Server,
			Username: args.Username,
			Password: args.Password,
		},
	}
	res.DockerImage, err = docker.NewImage(ctx, name+"-image", imageArgs, pulumi.Parent(res))
	if err != nil {
		return nil, err
	}

	return res, ctx.RegisterResourceOutputs(res, pulumi.Map{
		"name":          pulumi.String(res.Name),
		"imageUri":      res.DockerImage.ImageName,
		"baseImageName": res.DockerImage.BaseImageName,
	})
}
