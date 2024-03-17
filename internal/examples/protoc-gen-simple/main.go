// Copyright 2024 Buf Technologies, Inc.
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

// Package main implements a very simple plugin that just outputs text files
// with the names of the top-level messages in each file.
//
// Example: if a/b.proto had top-level messages C, D, the file "a/b.proto.txt" would be
// outputted, containing "C\nD\n".
package main

import (
	"context"
	"strings"

	"github.com/bufbuild/protoplugin"
)

const version = "0.0.1"

func main() {
	protoplugin.Main(protoplugin.HandlerFunc(handle), protoplugin.WithVersion(version))
}

func handle(
	_ context.Context,
	_ protoplugin.PluginEnv,
	responseWriter protoplugin.ResponseWriter,
	request protoplugin.Request,
) error {
	// Set the flag indicating that we support proto3 optionals. We don't even use them in this
	// plugin, but protoc will error if it encounters a proto3 file with an optional but the
	// plugin has not indicated it will support it.
	responseWriter.SetFeatureProto3Optional()

	for _, fileDescriptorProto := range request.FileDescriptorProtosToGenerate() {
		topLevelMessageNames := make([]string, len(fileDescriptorProto.GetMessageType()))
		for i, descriptorProto := range fileDescriptorProto.GetMessageType() {
			topLevelMessageNames[i] = descriptorProto.GetName()
		}
		// Add the response file to the response.
		responseWriter.AddFile(
			fileDescriptorProto.GetName()+".txt",
			strings.Join(topLevelMessageNames, "\n")+"\n",
		)
	}

	return nil
}
