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
	"fmt"
	"strings"

	"github.com/uber-go/tally"
	"google.golang.org/grpc"
)

const (
	ServerRequestsReceivedTotal      = "server_requests_received_total"
	ServerRequestsHandledTotal       = "server_requests_handled_total"
	ServerRequestsUnknownErrorTotal  = "server_requests_unknown_error_total"
	ServerRequestsSpecificErrorTotal = "server_requests_specific_error_total"
	ServerRequestsOkTotal            = "server_requests_ok_total"
	ServerRequestsHandlingTimeBucket = "server_requests_handling_time_bucket"
)

var (
	InitializedServerRequestsCounterNames = []string{
		ServerRequestsReceivedTotal,
		ServerRequestsHandledTotal,
		ServerRequestsUnknownErrorTotal,
		ServerRequestsOkTotal,
	}
	ServerRequestsHistogramNames = []string{
		ServerRequestsHandlingTimeBucket,
	}
)

type ServerEndpointMetadata struct {
	grpcServiceName string
	grpcMethodName  string
	grpcTypeName    string
}

type ServerRequestCounter struct {
	Name    string
	Tags    map[string]string
	Counter tally.Counter
}

type ServerRequestHistogram struct {
	Name      string
	Tags      map[string]string
	Histogram tally.Histogram
}

func newServerEndpointMetadata(grpcServiceName string, grpcMethodName string, grpcTypeName string) ServerEndpointMetadata {
	return ServerEndpointMetadata{grpcServiceName: grpcServiceName, grpcMethodName: grpcMethodName, grpcTypeName: grpcTypeName}
}

func (g *ServerEndpointMetadata) getTags() map[string]string {
	return map[string]string{
		"method":  g.grpcMethodName,
		"service": g.grpcServiceName,
		"type":    g.grpcTypeName,
	}
}

func (g *ServerEndpointMetadata) getSpecificErrorTags(source string, code string) map[string]string {
	return map[string]string{
		"method":  g.grpcMethodName,
		"service": g.grpcServiceName,
		"type":    g.grpcTypeName,
		"source":  source,
		"code":    code,
	}
}

func (g *ServerEndpointMetadata) getFullMethod() string {
	return fmt.Sprintf("/%s/%s", g.grpcServiceName, g.grpcMethodName)
}

func getGrpcEndPointMetadata(svcName string, methodInfo grpc.MethodInfo) ServerEndpointMetadata {
	var endpointMetadata ServerEndpointMetadata
	if methodInfo.IsServerStream {
		endpointMetadata = newServerEndpointMetadata(svcName, methodInfo.Name, "stream")
	} else {
		endpointMetadata = newServerEndpointMetadata(svcName, methodInfo.Name, "unary")
	}
	return endpointMetadata
}

func getGrpcEndPointMetadataFromFullMethod(fullMethod string, methodType string) ServerEndpointMetadata {
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
	}
	if methodType == "stream" {
		methodInfo = grpc.MethodInfo{
			Name:           methodName,
			IsClientStream: false,
			IsServerStream: true,
		}
	}
	return getGrpcEndPointMetadata(svcName, methodInfo)
}

func GetSpecificErrorCounter(fullMethod string, methodType string, errSource string, errCode string) *ServerRequestCounter {
	endpointMetadata := getGrpcEndPointMetadataFromFullMethod(fullMethod, methodType)
	tags := endpointMetadata.getSpecificErrorTags(errSource, errCode)
	counter := ServerRequestCounter{
		Name:    ServerRequestsSpecificErrorTotal,
		Tags:    tags,
		Counter: Root.Tagged(tags).Counter(ServerRequestsSpecificErrorTotal),
	}
	return &counter
}

func InitServerRequestCounters(svcName string, methodInfo grpc.MethodInfo) {
	endpointMetadata := getGrpcEndPointMetadata(svcName, methodInfo)
	fullMethodName := endpointMetadata.getFullMethod()
	tags := endpointMetadata.getTags()

	for _, counterName := range InitializedServerRequestsCounterNames {
		counter := ServerRequestCounter{
			Name:    counterName,
			Tags:    tags,
			Counter: Root.Tagged(tags).Counter(counterName),
		}
		if _, ok := ServerRequestCounters[fullMethodName]; !ok {
			child := make(map[string]*ServerRequestCounter)
			ServerRequestCounters[fullMethodName] = child
		}
		ServerRequestCounters[fullMethodName][counterName] = &counter
	}
}

func InitServerRequestHistograms(svcName string, methodInfo grpc.MethodInfo) {
	endpointMetadata := getGrpcEndPointMetadata(svcName, methodInfo)
	fullMethodName := endpointMetadata.getFullMethod()
	tags := endpointMetadata.getTags()

	for _, histogramName := range ServerRequestsHistogramNames {
		histogram := ServerRequestHistogram{
			Name:      histogramName,
			Tags:      tags,
			Histogram: Root.Tagged(tags).Histogram(histogramName, tally.DefaultBuckets),
		}
		if _, ok := ServerRequestHistograms[fullMethodName]; !ok {
			child := make(map[string]*ServerRequestHistogram)
			ServerRequestHistograms[fullMethodName] = child
		}
		ServerRequestHistograms[fullMethodName][histogramName] = &histogram
	}
}

func InitRequestMetricsForServer(s *grpc.Server) {
	for svcName, info := range s.GetServiceInfo() {
		for _, method := range info.Methods {
			InitServerRequestCounters(svcName, method)
			InitServerRequestHistograms(svcName, method)
		}
	}
}

func GetServerRequestCounter(fullMethod string, counterName string) *ServerRequestCounter {
	return ServerRequestCounters[fullMethod][counterName]
}

func GetServerRequestHistogram(fullMethod string, histogramName string) *ServerRequestHistogram {
	return ServerRequestHistograms[fullMethod][histogramName]
}
