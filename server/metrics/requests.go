// Copyright 2022 Tigris Data, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package metrics

import (
	"context"
	"fmt"
	"strings"

	"github.com/tigrisdata/tigris/server/request"

	"github.com/tigrisdata/tigris/server/config"
	"github.com/uber-go/tally"
	"google.golang.org/grpc"
)

var (
	OkRequests       tally.Scope
	ErrorRequests    tally.Scope
	RequestsRespTime tally.Scope
)

func getRequestOkTagKeys() []string {
	return []string{
		"grpc_method",
		"grpc_service",
		"tigris_tenant",
		"grpc_service_type",
		"env",
		"db",
		"collection",
	}
}

func getRequestTimerTagKeys() []string {
	return []string{
		"grpc_method",
		"grpc_service",
		"tigris_tenant",
		"grpc_service_type",
		"env",
		"db",
		"collection",
	}
}

func getRequestErrorTagKeys() []string {
	return []string{
		"grpc_method",
		"grpc_service",
		"tigris_tenant",
		"grpc_service_type",
		"env",
		"db",
		"collection",
		"error_code",
		"error_value",
	}
}

type RequestEndpointMetadata struct {
	serviceName   string
	methodInfo    grpc.MethodInfo
	namespaceName string
}

func newRequestEndpointMetadata(ctx context.Context, serviceName string, methodInfo grpc.MethodInfo) RequestEndpointMetadata {
	return RequestEndpointMetadata{serviceName: serviceName, methodInfo: methodInfo, namespaceName: request.GetNameSpaceFromHeader(ctx)}
}

func (r *RequestEndpointMetadata) GetMethodName() string {
	return strings.Split(r.methodInfo.Name, "/")[2]
}

func (r *RequestEndpointMetadata) GetServiceType() string {
	if r.methodInfo.IsServerStream {
		return "stream"
	} else {
		return "unary"
	}
}

func (r *RequestEndpointMetadata) GetInitialTags() map[string]string {
	return map[string]string{
		"grpc_method":       r.methodInfo.Name,
		"grpc_service":      r.serviceName,
		"tigris_tenant":     r.namespaceName,
		"grpc_service_type": r.GetServiceType(),
		"env":               config.GetEnvironment(),
		"db":                request.UnknownValue,
		"collection":        request.UnknownValue,
	}
}

func (r *RequestEndpointMetadata) getFullMethod() string {
	return fmt.Sprintf("/%s/%s", r.serviceName, r.methodInfo.Name)
}

func GetGrpcEndPointMetadataFromFullMethod(ctx context.Context, fullMethod string, methodType string) RequestEndpointMetadata {
	var methodInfo grpc.MethodInfo
	methodList := strings.Split(fullMethod, "/")
	svcName := methodList[1]
	methodName := methodList[2]
	if methodType == "unary" {
		methodInfo = grpc.MethodInfo{
			Name:           methodName,
			IsClientStream: false,
			IsServerStream: false,
		}
	} else if methodType == "stream" {
		methodInfo = grpc.MethodInfo{
			Name:           methodName,
			IsClientStream: false,
			IsServerStream: true,
		}
	}
	return newRequestEndpointMetadata(ctx, svcName, methodInfo)
}

func initializeRequestScopes() {
	OkRequests = Requests.SubScope("count")
	ErrorRequests = Requests.SubScope("count")
	RequestsRespTime = Requests.SubScope("response")
}
