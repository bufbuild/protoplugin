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
	"slices"
	"sync"

	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

// Request wraps a CodeGeneratorRequest.
//
// The backing CodeGeneratorRequest has been validated:
//
//   - The CodeGeneratorRequest will not be nil.
//   - FileToGenerate and ProtoFile will be non-empty.
//   - Each FileDescriptorProto in ProtoFile will have a valid path as the name field.
//   - Each value of FileToGenerate will be a valid path.
//   - Each value of FileToGenerate will have a corresponding value in ProtoFile.
//   - Each FileDescriptorProto in SourceFileDescriptors will have a valid path as the name field.
//   - The values of FileToGenerate will have a 1-1 mapping to the names in SourceFileDescriptors.
//
// Paths are considered valid if they are non-empty, relative, use '/' as the path separator, do not jump context,
// and have `.proto` as the file extension.
type Request struct {
	codeGeneratorRequest *pluginpb.CodeGeneratorRequest

	getFilesToGenerateMap                               func() map[string]struct{}
	getSourceFileDescriptorNameToFileDescriptorProtoMap func() map[string]*descriptorpb.FileDescriptorProto
}

func (r *Request) Parameter() string {
	return r.codeGeneratorRequest.GetParameter()
}

// GenerateFileDescriptors returns the FileDescriptors for the files specified by the
// file_to_generate field.
func (r *Request) GenerateFileDescriptors(options ...GenerateFileDescriptorsOption) ([]protoreflect.FileDescriptor, error) {
	requestFileOptions := newRequestFileOptions()
	for _, option := range options {
		option.applyGenerateFileDescriptorsOption(requestFileOptions)
	}
	files, err := r.allFiles(requestFileOptions.sourceRetentionOptions)
	if err != nil {
		return nil, err
	}
	fileDescriptors := make([]protoreflect.FileDescriptor, len(r.codeGeneratorRequest.GetFileToGenerate()))
	for i, fileToGenerate := range r.codeGeneratorRequest.GetFileToGenerate() {
		fileDescriptor, err := files.FindFileByPath(fileToGenerate)
		if err != nil {
			return nil, err
		}
		fileDescriptors[i] = fileDescriptor
	}
	return fileDescriptors, nil
}

// AllFiles returns the a Files registry for all files in the CodeGeneratorRequest.
func (r *Request) AllFiles(options ...AllFilesOption) (*protoregistry.Files, error) {
	requestFileOptions := newRequestFileOptions()
	for _, option := range options {
		option.applyAllFilesOption(requestFileOptions)
	}
	return r.allFiles(requestFileOptions.sourceRetentionOptions)
}

// GenerateFileDescriptorProtos returns the FileDescriptors for the files specified by the
// file_to_generate field.
func (r *Request) GenerateFileDescriptorProtos(options ...GenerateFileDescriptorProtosOption) []*descriptorpb.FileDescriptorProto {
	requestFileOptions := newRequestFileOptions()
	for _, option := range options {
		option.applyGenerateFileDescriptorProtosOption(requestFileOptions)
	}
	return r.generateFileDescriptorProtos(requestFileOptions.sourceRetentionOptions)
}

// AllFileDescriptorProtos returns the FileDescriptors for all files in the CodeGeneratorRequest.
func (r *Request) AllFileDescriptorProtos(options ...AllFileDescriptorProtosOption) []*descriptorpb.FileDescriptorProto {
	requestFileOptions := newRequestFileOptions()
	for _, option := range options {
		option.applyAllFileDescriptorProtosOption(requestFileOptions)
	}
	return r.allFileDescriptorProtos(requestFileOptions.sourceRetentionOptions)
}

// CompilerVersion returns the specified compiler version on the request, if it is present.
func (r *Request) CompilerVersion() *CompilerVersion {
	if version := r.codeGeneratorRequest.GetCompilerVersion(); version != nil {
		return &CompilerVersion{
			Major:  int(version.GetMajor()),
			Minor:  int(version.GetMinor()),
			Patch:  int(version.GetPatch()),
			Suffix: version.GetSuffix(),
		}
	}
	return nil
}

// CodeGeneratorRequest returns the underlying CodeGeneratorRequest.
func (r *Request) CodeGeneratorRequest() *pluginpb.CodeGeneratorRequest {
	return r.codeGeneratorRequest
}

// *** PRIVATE ***

func newRequest(codeGeneratorRequest *pluginpb.CodeGeneratorRequest) (*Request, error) {
	if err := validateCodeGeneratorRequest(codeGeneratorRequest); err != nil {
		return nil, err
	}
	request := &Request{
		codeGeneratorRequest: codeGeneratorRequest,
	}
	request.getFilesToGenerateMap =
		sync.OnceValue(request.getFilesToGenerateMapUncached)
	request.getSourceFileDescriptorNameToFileDescriptorProtoMap =
		sync.OnceValue(request.getSourceFileDescriptorNameToFileDescriptorProtoMapUncached)
	return request, nil
}

func (r *Request) allFiles(sourceRetentionOptions bool) (*protoregistry.Files, error) {
	return protodesc.NewFiles(
		&descriptorpb.FileDescriptorSet{
			File: r.allFileDescriptorProtos(sourceRetentionOptions),
		},
	)
}

func (r *Request) generateFileDescriptorProtos(sourceRetentionOptions bool) []*descriptorpb.FileDescriptorProto {
	// If we want source-retention options, source_file_descriptors is all we need.
	if sourceRetentionOptions {
		return slices.Clone(r.codeGeneratorRequest.GetSourceFileDescriptors())
	}
	// Otherwise, we need to get the values in proto_file that are in file_to_generate.
	filesToGenerateMap := r.getFilesToGenerateMap()
	fileDescriptorProtos := make([]*descriptorpb.FileDescriptorProto, 0, len(r.codeGeneratorRequest.GetFileToGenerate()))
	for _, protoFile := range r.codeGeneratorRequest.GetProtoFile() {
		if _, ok := filesToGenerateMap[protoFile.GetName()]; ok {
			fileDescriptorProtos = append(fileDescriptorProtos, protoFile)
		}
	}
	return fileDescriptorProtos
}

func (r *Request) allFileDescriptorProtos(sourceRetentionOptions bool) []*descriptorpb.FileDescriptorProto {
	// If we do not want source-retention options, proto_file is all we need.
	if !sourceRetentionOptions {
		return slices.Clone(r.codeGeneratorRequest.GetProtoFile())
	}
	// Otherwise, we need to replace the values in proto_file that are in file_to_generate
	// with the values from source_file_descriptors.
	filesToGenerateMap := r.getFilesToGenerateMap()
	sourceFileDescriptorNameToFileDescriptorProtoMap := r.getSourceFileDescriptorNameToFileDescriptorProtoMap()
	fileDescriptorProtos := make([]*descriptorpb.FileDescriptorProto, len(r.codeGeneratorRequest.GetProtoFile()))
	for i, protoFile := range r.codeGeneratorRequest.GetProtoFile() {
		if _, ok := filesToGenerateMap[protoFile.GetName()]; ok {
			// We assume we've done validation that source_file_descriptors contains file_to_generate.
			protoFile = sourceFileDescriptorNameToFileDescriptorProtoMap[protoFile.GetName()]
		}
		fileDescriptorProtos[i] = protoFile
	}
	return fileDescriptorProtos
}

func (r *Request) getFilesToGenerateMapUncached() map[string]struct{} {
	filesToGenerateMap := make(
		map[string]struct{},
		len(r.codeGeneratorRequest.GetFileToGenerate()),
	)
	for _, fileToGenerate := range r.codeGeneratorRequest.GetFileToGenerate() {
		filesToGenerateMap[fileToGenerate] = struct{}{}
	}
	return filesToGenerateMap
}

func (r *Request) getSourceFileDescriptorNameToFileDescriptorProtoMapUncached() map[string]*descriptorpb.FileDescriptorProto {
	sourceFileDescriptorNameToFileDescriptorProtoMap := make(
		map[string]*descriptorpb.FileDescriptorProto,
		len(r.codeGeneratorRequest.GetSourceFileDescriptors()),
	)
	for _, sourceFileDescriptor := range r.codeGeneratorRequest.GetSourceFileDescriptors() {
		sourceFileDescriptorNameToFileDescriptorProtoMap[sourceFileDescriptor.GetName()] = sourceFileDescriptor
	}
	return sourceFileDescriptorNameToFileDescriptorProtoMap
}
