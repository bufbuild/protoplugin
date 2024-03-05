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

// Package main implements a very simple plugin that scaffolds using the protogen
// package with the protoplugin package.
//
// For plugins that generate Golang code, protogen is generally all you need, and protoplugin
// is somewhat superfluous, however the additional request/response checking may be useful.
// Regardless, this is mostly a demonstration of concepts.
package main

import (
	"context"
	"errors"

	"github.com/bufbuild/protoplugin"
	"google.golang.org/protobuf/compiler/protogen"
)

func main() {
	protoplugin.Main(protoplugin.HandlerFunc(handle))
}

func handle(
	_ context.Context,
	responseWriter *protoplugin.ResponseWriter,
	request *protoplugin.Request,
) error {
	plugin, err := protogen.Options{}.New(request.CodeGeneratorRequest())
	if err != nil {
		return err
	}
	if err := handleProtogenPlugin(plugin); err != nil {
		plugin.Error(err)
	}
	response := plugin.Response()
	responseWriter.AddCodeGeneratorResponseFiles(response.GetFile()...)
	responseWriter.AddError(response.GetError())
	responseWriter.AddFeatureProto3Optional()
	return nil
}

func handleProtogenPlugin(plugin *protogen.Plugin) error {
	_ = plugin
	return errors.New("TODO")
}
