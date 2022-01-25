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

package main

import (
	"github.com/tigrisdata/tigrisdb/store/kv"
	"github.com/tigrisdata/tigrisdb/util/log"
)

type apiConfig struct {
	Host     string
	HTTPPort int16 `mapstructure:"http_port" yaml:"http_port"`
	GRPCPort int16 `mapstructure:"grpc_port" yaml:"grpc_port"`
}

type Config struct {
	API      apiConfig `yaml:"api" json:"api"`
	Log      log.LogConfig
	DynamoDB kv.DynamodbConfig
}

var config = Config{
	Log: log.LogConfig{
		Level: "trace",
	},
	API: apiConfig{
		Host:     "0.0.0.0",
		HTTPPort: 8081,
		GRPCPort: 8082,
	},
}
