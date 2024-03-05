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
	"fmt"
	"strconv"
	"sync"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

const allSupportedFeaturesMask = uint64(
	pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL |
		pluginpb.CodeGeneratorResponse_FEATURE_SUPPORTS_EDITIONS,
)

// ResponseWriter is used by implementations of Handler to construct CodeGeneratorResponses.
type ResponseWriter struct {
	warningHandlerFunc func(error)

	responseFileNames         map[string]struct{}
	responseFiles             []*pluginpb.CodeGeneratorResponse_File
	responseErrorMessage      string
	responseSupportedFeatures uint64
	responseMinimumEdition    uint32
	responseMaximumEdition    uint32

	systemErrors []error
	lock         sync.RWMutex
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
	r.responseErrorMessage = message
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
	r.SetMinimumEdition(uint32(minimumEdition))
	r.SetMaximumEdition(uint32(maximumEdition))
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

	for _, file := range files {
		name := file.GetName()
		if name == "" {
			r.addSystemError(fmt.Errorf("CodeGeneratorResponse.File: name: empty"))
			return
		}
		normalizedName, err := validateAndNormalizePath(name)
		if err != nil {
			r.addSystemError(fmt.Errorf("CodeGeneratorResponse.File: %w", err))
			return
		}
		if normalizedName != name {
			r.warnUnnormalizedName(name, normalizedName)
			// We will coerce this into a normalized name if it is valid via normalizePath.
			name = normalizedName
			file.Name = proto.String(name)
		}

		if r.isDuplicate(file) {
			r.warnDuplicateName(name)
			return
		}
		r.responseFileNames[name] = struct{}{}
		r.responseFiles = append(r.responseFiles, file)
	}
}

// SetSupportedFeaturessets the given features on the response.
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

	if supportedFeatures|allSupportedFeaturesMask != allSupportedFeaturesMask {
		r.addSystemError(fmt.Errorf("specified supported features contains unknown CodeGeneratorResponse.Features: %s", strconv.FormatUint(supportedFeatures, 2)))
		return
	}
	r.responseSupportedFeatures = supportedFeatures
}

// SetMinimumEdition sets the minimum edition.
//
// If you want to specify that you are supporting editions, it is likely easier to use
// SetFeatureSupportsEditions. This function is for those callers needing to have lower-level access.
//
// The plugin will exit with a non-zero exit code if the minimum edition is greater than the maximum edition.
func (r *ResponseWriter) SetMinimumEdition(minimumEdition uint32) {
	r.responseMinimumEdition = minimumEdition
}

// SetMaximumEdition sets the maximum edition.
//
// If you want to specify that you are supporting editions, it is likely easier to use
// SetFeatureSupportsEditions. This function is for those callers needing to have lower-level access.
//
// The plugin will exit with a non-zero exit code if the minimum edition is greater than the maximum edition.
func (r *ResponseWriter) SetMaximumEdition(maximumEdition uint32) {
	r.responseMaximumEdition = maximumEdition
}

// *** PRIVATE ***

func newResponseWriter(warningHandlerFunc func(error)) *ResponseWriter {
	return &ResponseWriter{
		warningHandlerFunc: warningHandlerFunc,
		responseFileNames:  make(map[string]struct{}),
	}
}

func (r *ResponseWriter) addSupportedFeatures(supportedFeatures uint64) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if supportedFeatures|allSupportedFeaturesMask != allSupportedFeaturesMask {
		r.addSystemError(fmt.Errorf("specified supported features contains unknown CodeGeneratorResponse.Features: %s", strconv.FormatUint(supportedFeatures, 2)))
		return
	}
	r.responseSupportedFeatures |= supportedFeatures
}

func (r *ResponseWriter) toCodeGeneratorResponse() (*pluginpb.CodeGeneratorResponse, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	if r.responseSupportedFeatures&uint64(pluginpb.CodeGeneratorResponse_FEATURE_SUPPORTS_EDITIONS) != 0 {
		if r.responseMinimumEdition == 0 {
			r.addSystemError(
				errors.New("CodeGeneratorResponse: FEATURE_SUPPORTS_EDITIONS specified but no minimum_edition set"),
			)
		}
		if r.responseMaximumEdition == 0 {
			r.addSystemError(
				errors.New("CodeGeneratorResponse: FEATURE_SUPPORTS_EDITIONS specified but no maximum_edition set"),
			)
		}
		if r.responseMinimumEdition > r.responseMaximumEdition {
			r.addSystemError(
				fmt.Errorf(
					"CodeGeneratorResponse: minimum_edition %d is greater than maximum_edition %d",
					r.responseMinimumEdition,
					r.responseMaximumEdition,
				),
			)
		}
	}

	if len(r.systemErrors) > 0 {
		return nil, errors.Join(r.systemErrors...)
	}
	response := &pluginpb.CodeGeneratorResponse{
		File: r.responseFiles,
	}
	if r.responseErrorMessage != "" {
		response.Error = proto.String(r.responseErrorMessage)
	}
	if r.responseSupportedFeatures != 0 {
		response.SupportedFeatures = proto.Uint64(r.responseSupportedFeatures)
	}
	return response, nil
}

// isDuplicate determines if the given file is a duplicate file.
// Insertion points are intentionally ignored because they must
// always reference duplicate files in order to take effect.
//
// Note that we do not acquire the lock here because this helper
// is only called within the context of r.AddFile.
func (r *ResponseWriter) isDuplicate(file *pluginpb.CodeGeneratorResponse_File) bool {
	if file.GetInsertionPoint() != "" {
		return false
	}
	_, ok := r.responseFileNames[file.GetName()]
	return ok
}

func (r *ResponseWriter) warnUnnormalizedName(name string, expectedName string) {
	r.warnf(newUnvalidatedNameError(fmt.Errorf("expected %q to equal %q", name, expectedName)).Error())
}

func (r *ResponseWriter) warnDuplicateName(name string) {
	r.warnf(`Duplicate generated file name %q. Generation will continue without error here and drop the second occurrence of this file, but please raise an issue with the maintainer of the plugin.`, name)
}

func (r *ResponseWriter) warnf(message string, args ...any) {
	r.warningHandlerFunc(fmt.Errorf("Warning: "+message, args...))
}

func (r *ResponseWriter) addSystemError(err error) {
	r.systemErrors = append(r.systemErrors, err)
}

func newUnvalidatedNameError(err error) error {
	return fmt.Errorf(
		`file name does not conform to the Protobuf generation specification. Note that the file name must be non-empty, relative, use "/" instead of "\" as the path separator, and not use "." or ".." as part of the file name. Generation will continue without error here, but please raise an issue with the maintainer of the plugin and reference https://github.com/protocolbuffers/protobuf/blob/95e6c5b4746dd7474d540ce4fb375e3f79a086f8/src/google/protobuf/compiler/plugin.proto#L122: %w`,
		err,
	)
}
