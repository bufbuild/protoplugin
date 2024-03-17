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

package protoplugin

import (
	"errors"
	"sync"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

// ResponseWriter is used by implementations of Handler to construct CodeGeneratorResponses.
type ResponseWriter struct {
	codeGeneratorResponse *pluginpb.CodeGeneratorResponse
	written               bool

	lenientResponseValidateErrorFunc func(error)

	lock sync.RWMutex
}

// AddFile adds the file with the given content to the response.
//
// This takes care of the most common case of adding a CodeGeneratorResponse.File with content. If you need add a
// CodeGeneratorResponse.File with insertion points or any other feature, use AddCodeGeneratorResponseFiles.
//
// The plugin will exit with a non-zero exit code if the name is an invalid path.
// Paths are considered valid if they are non-empty, relative, use '/' as the path separator, and do not jump context.
//
// If a file with the same name was already added, or the file name is not cleaned, a warning will be produced.
func (r *ResponseWriter) AddFile(name string, content string) {
	r.AddCodeGeneratorResponseFiles(
		&pluginpb.CodeGeneratorResponse_File{
			Name:    proto.String(name),
			Content: proto.String(content),
		},
	)
}

// SetError sets the error message on the response.
//
// If there is an error with the actual input .proto files that results in your plugin's business logic not being able to be executed
// (for example, a missing option), this error should be added to the response via SetError. If there is a system error, the
// Handler should return error, which will result in the plugin exiting with a non-zero exit code.
//
// If there is an existing error message already added, it will be overwritten.
// Note that empty error messages will be ignored (ie it will be as if no error was set).
func (r *ResponseWriter) SetError(message string) {
	r.lock.Lock()
	defer r.lock.Unlock()

	// plugin.proto specifies that only non-empty errors are considered errors.
	// This is also consistent with protoc's behavior.
	// Ref: https://github.com/protocolbuffers/protobuf/blob/069f989b483e63005f87ab309de130677718bbec/src/google/protobuf/compiler/plugin.proto#L100-L108.
	if message == "" {
		return
	}
	r.codeGeneratorResponse.Error = proto.String(message)
}

// SetFeatureProto3Optional sets the FEATURE_PROTO3_OPTIONAL feature on the response.
//
// This function should be preferred over SetSupportedFeatures. Use SetSupportedFeatures only if you need low-level access.
func (r *ResponseWriter) SetFeatureProto3Optional() {
	r.addSupportedFeatures(uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL))
}

// SetFeatureSupportsEditions sets the FEATURE_SUPPORTS_EDITIONS feature on the response along
// with the given min and max editions.
//
// This function should be preferred over calling SetSupportedFeatures, SetMinimumEdition, and SetMaximumEdition separately.
// Use SetSupportedFeatures, SetMinimumEdition, and SetMaximumEdition only if you need low-level access.
//
// The plugin will exit with a non-zero exit code if the minimum edition is greater than the maximum edition.
func (r *ResponseWriter) SetFeatureSupportsEditions(
	minimumEdition descriptorpb.Edition,
	maximumEdition descriptorpb.Edition,
) {
	r.addSupportedFeatures(uint64(pluginpb.CodeGeneratorResponse_FEATURE_SUPPORTS_EDITIONS))
	r.SetMinimumEdition(int32(minimumEdition))
	r.SetMaximumEdition(int32(maximumEdition))
}

// AddCodeGeneratorResponseFiles adds the CodeGeneratorResponse.Files to the response.
//
// See the documentation on CodeGeneratorResponse.File for the exact semantics.
//
// If you are just adding file content, use the simpler AddFile. This function is for lower-level access.
//
// The plugin will exit with a non-zero exit code if any of the following are true:
//
// - The CodeGeneratorResponse_File is nil.
// - The name is an invalid path.
//
// Paths are considered valid if they are non-empty, relative, use '/' as the path separator, and do not jump context.
//
// If a file with the same name was already added, or the file name is not cleaned, a warning will be produced.
func (r *ResponseWriter) AddCodeGeneratorResponseFiles(files ...*pluginpb.CodeGeneratorResponse_File) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.codeGeneratorResponse.File = append(r.codeGeneratorResponse.GetFile(), files...)
}

// SetSupportedFeatures the given features on the response.
//
// You likely want to use the specific feature functions instead of this function.
// This function is for lower-level access.
//
// If there are existing features already added, they will be overwritten.
//
// If the features are not represented in the known CodeGeneratorResponse.Features,
// the plugin will exit with a non-zero exit code.
func (r *ResponseWriter) SetSupportedFeatures(supportedFeatures uint64) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if supportedFeatures == 0 {
		r.codeGeneratorResponse.SupportedFeatures = nil
	} else {
		r.codeGeneratorResponse.SupportedFeatures = proto.Uint64(supportedFeatures)
	}
}

// SetMinimumEdition sets the minimum edition.
//
// If you want to specify that you are supporting editions, it is likely easier to use
// SetFeatureSupportsEditions. This function is for those callers needing to have lower-level access.
//
// The plugin will exit with a non-zero exit code if the minimum edition is greater than the maximum edition.
func (r *ResponseWriter) SetMinimumEdition(minimumEdition int32) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.codeGeneratorResponse.MinimumEdition = proto.Int32(minimumEdition)
}

// SetMaximumEdition sets the maximum edition.
//
// If you want to specify that you are supporting editions, it is likely easier to use
// SetFeatureSupportsEditions. This function is for those callers needing to have lower-level access.
//
// The plugin will exit with a non-zero exit code if the minimum edition is greater than the maximum edition.
func (r *ResponseWriter) SetMaximumEdition(maximumEdition int32) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.codeGeneratorResponse.MaximumEdition = proto.Int32(maximumEdition)
}

// *** PRIVATE ***

func newResponseWriter(lenientResponseValidateErrorFunc func(error)) *ResponseWriter {
	return &ResponseWriter{
		codeGeneratorResponse:            &pluginpb.CodeGeneratorResponse{},
		lenientResponseValidateErrorFunc: lenientResponseValidateErrorFunc,
	}
}

func (r *ResponseWriter) addSupportedFeatures(supportedFeatures uint64) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.codeGeneratorResponse.SupportedFeatures = proto.Uint64(r.codeGeneratorResponse.GetSupportedFeatures() | supportedFeatures)
}

func (r *ResponseWriter) toCodeGeneratorResponse() (*pluginpb.CodeGeneratorResponse, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	if r.written {
		// We do modifications of the CodeGeneratorResponse in validateAndNormalizeCodeGeneratorResponse, so if someone were
		// to somehow reuse a ResponseWriter, they may get unexpected results in the future.
		//
		// This is an edge case - ResponseWriters are given to Handlers, so to reuse one would be very weird.
		return nil, errors.New("ResponseWriter cannot be reused")
	}
	r.written = true

	if err := validateAndNormalizeCodeGeneratorResponse(r.codeGeneratorResponse, r.lenientResponseValidateErrorFunc); err != nil {
		return nil, err
	}
	return r.codeGeneratorResponse, nil
}
