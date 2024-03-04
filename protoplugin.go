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

// Package protoplugin is a simple library to assist in writing protoc plugins.
package protoplugin

import (
	"context"
	"fmt"
	"io"
	"os"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/pluginpb"
)

// Main can be called by main functions to run a Handler.
//
// If an error is returned by the handler, Main will exit with exit code 1.
//
//	func main() {
//	  protoplugin.Main(context.Background(), newHandler())
//	}
func Main(ctx context.Context, handler Handler, options ...RunOption) {
	if err := Run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, handler, options...); err != nil {
		if errString := err.Error(); errString != "" {
			_, _ = fmt.Fprintln(os.Stderr, errString)
		}
		os.Exit(1)
	}
}

// Run runs the plugin using the Handler for the given stdio.
func Run(
	ctx context.Context,
	args []string,
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
	handler Handler,
	options ...RunOption,
) error {
	// We don't use args yet, but reserving it for future use in case we want to implement automatic handling of a version flag.
	_ = args

	runOptions := newRunOptions()
	for _, option := range options {
		option.applyRunOption(runOptions)
	}
	warningHandlerFunc := runOptions.warningHandlerFunc
	if warningHandlerFunc == nil {
		warningHandlerFunc = func(err error) { _, _ = fmt.Fprintln(stderr, err.Error()) }
	}

	input, err := io.ReadAll(stdin)
	if err != nil {
		return err
	}
	codeGeneratorRequest := &pluginpb.CodeGeneratorRequest{}
	if err := proto.Unmarshal(input, codeGeneratorRequest); err != nil {
		return err
	}
	request, err := newRequest(codeGeneratorRequest)
	if err != nil {
		return err
	}
	responseWriter := newResponseWriter(warningHandlerFunc)
	if err := handler.Handle(ctx, responseWriter, request); err != nil {
		return err
	}
	codeGeneratorResponse, err := responseWriter.toCodeGeneratorResponse()
	if err != nil {
		return err
	}
	data, err := proto.Marshal(codeGeneratorResponse)
	if err != nil {
		return err
	}
	_, err = stdout.Write(data)
	return err
}
