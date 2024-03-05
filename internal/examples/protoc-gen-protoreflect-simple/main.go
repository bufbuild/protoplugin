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
// with the names of the top-level messages in each file, but uses the protoreflect
// package instead of directly using the FileDescriptorProtos.
//
// Example: if a/b.proto had top-level messages C, D, the file "a/b.proto.txt" would be
// outputted, containing "C\nD\n".
//
// This is functionally equivalent to the protoc-gen-simple example, but shows how
// you can use the protoplugin API to get richer types.
package main

import (
	"context"
	"strings"

	"github.com/bufbuild/protoplugin"
)

func main() {
	protoplugin.Main(protoplugin.HandlerFunc(handle))
}

func handle(
	_ context.Context,
	responseWriter *protoplugin.ResponseWriter,
	request *protoplugin.Request,
) error {
	// Set the flag indicating that we support proto3 optionals. We don't even use them in this
	// plugin, but protoc will error if it encounters a proto3 file with an optional but the
	// plugin has not indicated it will support it.
	responseWriter.AddFeatureProto3Optional()

	fileDescriptors, err := request.FileDescriptorsToGenerate()
	if err != nil {
		return err
	}
	for _, fileDescriptor := range fileDescriptors {
		messages := fileDescriptor.Messages()
		topLevelMessageNames := make([]string, messages.Len())
		for i := 0; i < messages.Len(); i++ {
			topLevelMessageNames[i] = string(messages.Get(i).Name())
		}
		// Add the response file to the response.
		responseWriter.AddFile(
			fileDescriptor.Path()+".txt",
			strings.Join(topLevelMessageNames, "\n")+"\n",
		)
	}

	return nil
}
