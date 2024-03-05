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
	"slices"
	"sync"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

// Request wraps a CodeGeneratorRequest.
//
// The backing CodeGeneratorRequest has been validated to conform to the following:
//
//   - The CodeGeneratorRequest will not be nil.
//   - FileToGenerate and ProtoFile will be non-empty.
//   - Each FileDescriptorProto in ProtoFile will have a valid path as the name field.
//   - Each value of FileToGenerate will be a valid path.
//   - Each value of FileToGenerate will have a corresponding value in ProtoFile.
//   - Each FileDescriptorProto in SourceFileDescriptors will have a valid path as the name field.
//   - SourceFileDescriptors is either empty, or the values of FileToGenerate will have a 1-1 mapping
//     to the names in SourceFileDescriptors.
//
// Paths are considered valid if they are non-empty, relative, use '/' as the path separator, do not jump context,
// and have `.proto` as the file extension.
type Request struct {
	codeGeneratorRequest *pluginpb.CodeGeneratorRequest

	getFilesToGenerateMap                               func() map[string]struct{}
	getSourceFileDescriptorNameToFileDescriptorProtoMap func() map[string]*descriptorpb.FileDescriptorProto
}

// Parameter returns the value of the parameter field on the CodeGeneratorRequest.
func (r *Request) Parameter() string {
	return r.codeGeneratorRequest.GetParameter()
}

// ToGenerateFileDescriptors returns the FileDescriptors for the files specified by the
// file_to_generate field on the CodeGeneratorRequest.
//
// If WithSourceRetentionOptions is specified and the source_file_descriptors field was
// not present on the CodeGeneratorRequest, an error is returned.
//
// The caller can assume that all FileDescriptors have a valid path as the name field.
//
// Paths are considered valid if they are non-empty, relative, use '/' as the path separator, do not jump context,
// and have `.proto` as the file extension.
func (r *Request) ToGenerateFileDescriptors(options ...RequestFileOption) ([]protoreflect.FileDescriptor, error) {
	requestFileOptions := newRequestFileOptions()
	for _, option := range options {
		option.applyRequestFileOption(requestFileOptions)
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
//
// This matches with the proto_file field on the CodeGeneratorRequest, with the FileDescriptorProtos
// from the source_file_descriptors field used for the files in file_to_geneate if WithSourceRetentionOptions
// is specified.
//
// If WithSourceRetentionOptions is specified and the source_file_descriptors field was
// not present on the CodeGeneratorRequest, an error is returned.
//
// The caller can assume that all FileDescriptors have a valid path as the name field.
//
// Paths are considered valid if they are non-empty, relative, use '/' as the path separator, do not jump context,
// and have `.proto` as the file extension.
func (r *Request) AllFiles(options ...RequestFileOption) (*protoregistry.Files, error) {
	requestFileOptions := newRequestFileOptions()
	for _, option := range options {
		option.applyRequestFileOption(requestFileOptions)
	}
	return r.allFiles(requestFileOptions.sourceRetentionOptions)
}

// ToGenerateFileDescriptorProtos returns the FileDescriptors for the files specified by the
// file_to_generate field.
//
// If WithSourceRetentionOptions is specified and the source_file_descriptors field was
// not present on the CodeGeneratorRequest, an error is returned.
//
// The caller can assume that all FileDescriptorProtoss have a valid path as the name field.
//
// Paths are considered valid if they are non-empty, relative, use '/' as the path separator, do not jump context,
// and have `.proto` as the file extension.
func (r *Request) ToGenerateFileDescriptorProtos(options ...RequestFileOption) ([]*descriptorpb.FileDescriptorProto, error) {
	requestFileOptions := newRequestFileOptions()
	for _, option := range options {
		option.applyRequestFileOption(requestFileOptions)
	}
	return r.generateFileDescriptorProtos(requestFileOptions.sourceRetentionOptions)
}

// AllFileDescriptorProtos returns the FileDescriptorProtos for all files in the CodeGeneratorRequest.
//
// This matches with the proto_file field on the CodeGeneratorRequest, with the FileDescriptorProtos
// from the source_file_descriptors field used for the files in file_to_geneate if WithSourceRetentionOptions
// is specified.
//
// If WithSourceRetentionOptions is specified and the source_file_descriptors field was
// not present on the CodeGeneratorRequest, an error is returned.
//
// The caller can assume that all FileDescriptorProtoss have a valid path as the name field.
//
// Paths are considered valid if they are non-empty, relative, use '/' as the path separator, do not jump context,
// and have `.proto` as the file extension.
func (r *Request) AllFileDescriptorProtos(options ...RequestFileOption) ([]*descriptorpb.FileDescriptorProto, error) {
	requestFileOptions := newRequestFileOptions()
	for _, option := range options {
		option.applyRequestFileOption(requestFileOptions)
	}
	return r.allFileDescriptorProtos(requestFileOptions.sourceRetentionOptions)
}

// CompilerVersion returns the specified compiler_version on the CodeGeneratorRequest.
//
// If the compiler_version field was not present, nil is returned.
//
// The caller can assume that the major, minor, and patch values are non-negative.
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

// CodeGeneratorRequest returns the raw underlying CodeGeneratorRequest.
//
// The returned CodeGeneratorRequest is a copy - you can freely modiify it.
func (r *Request) CodeGeneratorRequest() (*pluginpb.CodeGeneratorRequest, error) {
	clone, ok := proto.Clone(r.codeGeneratorRequest).(*pluginpb.CodeGeneratorRequest)
	if !ok {
		return nil, fmt.Errorf("proto.Clone on %T returned a %T", r.codeGeneratorRequest, clone)
	}
	return clone, nil
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
	fileDescriptorProtos, err := r.allFileDescriptorProtos(sourceRetentionOptions)
	if err != nil {
		return nil, err
	}
	return protodesc.NewFiles(&descriptorpb.FileDescriptorSet{File: fileDescriptorProtos})
}

func (r *Request) generateFileDescriptorProtos(sourceRetentionOptions bool) ([]*descriptorpb.FileDescriptorProto, error) {
	// If we want source-retention options, source_file_descriptors is all we need.
	if sourceRetentionOptions {
		if err := r.validateSourceFileDescriptorsPresent(); err != nil {
			return nil, err
		}
		return slices.Clone(r.codeGeneratorRequest.GetSourceFileDescriptors()), nil
	}
	// Otherwise, we need to get the values in proto_file that are in file_to_generate.
	filesToGenerateMap := r.getFilesToGenerateMap()
	fileDescriptorProtos := make([]*descriptorpb.FileDescriptorProto, 0, len(r.codeGeneratorRequest.GetFileToGenerate()))
	for _, protoFile := range r.codeGeneratorRequest.GetProtoFile() {
		if _, ok := filesToGenerateMap[protoFile.GetName()]; ok {
			fileDescriptorProtos = append(fileDescriptorProtos, protoFile)
		}
	}
	return fileDescriptorProtos, nil
}

func (r *Request) allFileDescriptorProtos(sourceRetentionOptions bool) ([]*descriptorpb.FileDescriptorProto, error) {
	// If we do not want source-retention options, proto_file is all we need.
	if !sourceRetentionOptions {
		return slices.Clone(r.codeGeneratorRequest.GetProtoFile()), nil
	}
	if err := r.validateSourceFileDescriptorsPresent(); err != nil {
		return nil, err
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
	return fileDescriptorProtos, nil
}

func (r *Request) validateSourceFileDescriptorsPresent() error {
	if len(r.codeGeneratorRequest.GetSourceFileDescriptors()) == 0 &&
		len(r.codeGeneratorRequest.GetProtoFile()) > 0 {
		return errors.New("source_file_descriptors not set on CodeGeneratorRequest but source-retention options requested - you likely need to upgrade your protobuf compiler")
	}
	return nil
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
