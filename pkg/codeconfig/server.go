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

package codeconfig

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/nitrictech/nitric/pkg/api/nitric/v1"
)

type Server struct {
	name     string
	function *FunctionDependencies
	pb.UnimplementedFaasServiceServer
	pb.UnimplementedResourceServiceServer
}

// TriggerStream - Starts a new FaaS server stream
//
// The deployment server collects information from stream InitRequests, then immediately terminates the stream
// This behavior captures enough information to identify function handlers, without executing the handler code
// during the build process.
func (s *Server) TriggerStream(stream pb.FaasService_TriggerStreamServer) error {
	cm, err := stream.Recv()
	if err != nil {
		return status.Errorf(codes.Internal, "error reading message from stream: %v", err)
	}

	ir := cm.GetInitRequest()
	if ir == nil {
		// SHUT IT DOWN!!!!
		// The first message must be an init request from the prospective FaaS worker
		return status.Error(codes.FailedPrecondition, "first message must be InitRequest")
	}

	switch w := ir.Worker.(type) {
	case *pb.InitRequest_Api:
		return s.function.AddApiHandler(w.Api)
	case *pb.InitRequest_Schedule:
		return s.function.AddScheduleHandler(w.Schedule)
	case *pb.InitRequest_Subscription:
		return s.function.AddSubscriptionHandler(w.Subscription)
	}

	// treat as normal function worker
	// XXX: No-op for now. This can be handled exclusively at runtime
	// Close the stream, once we've received the InitRequest
	return nil
}

// Declare - Accepts resource declarations, adding them as dependencies to the Function
func (s *Server) Declare(ctx context.Context, req *pb.ResourceDeclareRequest) (*pb.ResourceDeclareResponse, error) {
	switch req.Resource.Type {
	case pb.ResourceType_Bucket:
		s.function.AddBucket(req.Resource.Name, req.GetBucket())
	case pb.ResourceType_Collection:
		s.function.AddCollection(req.Resource.Name, req.GetCollection())
	case pb.ResourceType_Queue:
		s.function.AddQueue(req.Resource.Name, req.GetQueue())
	case pb.ResourceType_Topic:
		s.function.AddTopic(req.Resource.Name, req.GetTopic())
	case pb.ResourceType_Policy:
		s.function.AddPolicy(req.GetPolicy())
	case pb.ResourceType_Secret:
		s.function.AddSecret(req.Resource.Name, req.GetSecret())
	case pb.ResourceType_Api:
		s.function.AddApiSecurityDefinitions(req.Resource.Name, req.GetApi().SecurityDefinitions)
		s.function.AddApiSecurity(req.Resource.Name, req.GetApi().Security)
	}

	return &pb.ResourceDeclareResponse{}, nil
}

func (s *Server) Details(ctx context.Context, req *pb.ResourceDetailsRequest) (*pb.ResourceDetailsResponse, error) {
	switch req.Resource.Type {
	case pb.ResourceType_Api:
		return &pb.ResourceDetailsResponse{
			Provider: "dev",
			Service:  "Api",
			Id:       req.Resource.Name,
			Details: &pb.ResourceDetailsResponse_Api{
				Api: &pb.ApiResourceDetails{
					Url: "http://localhost:50051/apis/" + req.Resource.Name,
				},
			},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported resource type %s", req.Resource.Type)
	}
}

// NewServer - Creates a new deployment server
func NewServer(name string, function *FunctionDependencies) *Server {
	return &Server{
		name:     name,
		function: function,
	}
}
