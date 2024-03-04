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
	"path/filepath"
	"strings"

	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

func validateCodeGeneratorRequest(request *pluginpb.CodeGeneratorRequest) error {
	if request == nil {
		return errors.New("CodeGeneratorRequest: nil")
	}
	if len(request.GetProtoFile()) == 0 {
		return errors.New("CodeGeneratorRequest.proto_file: empty")
	}
	if len(request.GetFileToGenerate()) == 0 {
		return errors.New("CodeGeneratorRequest.file_to_generate: empty")
	}
	if err := validateProtoPaths(request.GetFileToGenerate()); err != nil {
		return fmt.Errorf("CodeGeneratorRequest.file_to_generate: %w", err)
	}
	if err := validateCodeGeneratorRequestFileDescriptorProtos(
		"proto_file",
		request.GetProtoFile(),
		request.GetFileToGenerate(),
		false,
	); err != nil {
		return err
	}
	if len(request.GetSourceFileDescriptors()) > 0 {
		if err := validateCodeGeneratorRequestFileDescriptorProtos(
			"source_file_descriptors",
			request.GetSourceFileDescriptors(),
			request.GetFileToGenerate(),
			true,
		); err != nil {
			return err
		}
	}
	if version := request.GetCompilerVersion(); version != nil {
		if major := version.GetMajor(); major < 0 {
			return fmt.Errorf("CodeGeneratorRequest.compiler_version.major is negative: %d", int(major))
		}
		if minor := version.GetMinor(); minor < 0 {
			return fmt.Errorf("CodeGeneratorRequest.compiler_version.minor is negative: %d", int(minor))
		}
		if patch := version.GetPatch(); patch < 0 {
			return fmt.Errorf("CodeGeneratorRequest.compiler_version.patch is negative: %d", int(patch))
		}
	}
	return nil
}

func validateCodeGeneratorRequestFileDescriptorProtos(
	fieldName string,
	fileDescriptorProtos []*descriptorpb.FileDescriptorProto,
	filesToGenerate []string,
	// If true, the FileDescriptorProto Names should be equal to the names in filesToGenerate.
	// If false, the FileDescriptorProto Names should be a superset of the names in filesToGenerate.
	// This is true for source_file_descriptors, false for proto_file.
	equalToOrSupersetOfFilesToGenerate bool,
) error {
	fileDescriptorProtoNameMap := make(map[string]struct{}, len(fileDescriptorProtos))
	for _, fileDescriptorProto := range fileDescriptorProtos {
		if err := validateFileDescriptorProto(fileDescriptorProto); err != nil {
			return fmt.Errorf("CodeGeneratorRequest.%s: %w", fieldName, err)
		}
		fileDescriptorProtoNameMap[fileDescriptorProto.GetName()] = struct{}{}
	}
	for _, fileToGenerate := range filesToGenerate {
		if _, ok := fileDescriptorProtoNameMap[fileToGenerate]; !ok {
			return fmt.Errorf("CodeGeneratorRequest.file_to_generate: path %q is not contained within %s", fileToGenerate, fieldName)
		}
	}
	if equalToOrSupersetOfFilesToGenerate {
		// Since we already checked if fileDescriptorProtoNameMap contains filesToGenerate, if
		// filesToGenerate contains fileDescriptorProtoNameMap, we are equal.
		filesToGenerateMap := make(map[string]struct{}, len(filesToGenerate))
		for _, fileToGenerate := range filesToGenerate {
			filesToGenerateMap[fileToGenerate] = struct{}{}
		}
		for fileDescriptorProtoName := range fileDescriptorProtoNameMap {
			if _, ok := filesToGenerateMap[fileDescriptorProtoName]; !ok {
				return fmt.Errorf("CodeGeneratorRequest.%s: path %q is not contained within file_to_generate", fieldName, fileDescriptorProtoName)
			}
		}
	}
	return nil
}

func validateFileDescriptorProto(fileDescriptorProto *descriptorpb.FileDescriptorProto) error {
	if fileDescriptorProto == nil {
		return errors.New("FileDescriptorProto: nil")
	}
	if err := validateProtoPath(fileDescriptorProto.GetName()); err != nil {
		return fmt.Errorf("FileDescriptorProto.name: %w", err)
	}
	if err := validateProtoPaths(fileDescriptorProto.GetDependency()); err != nil {
		return fmt.Errorf("FileDescriptorProto.dependency %w", err)
	}
	return nil
}

func validateProtoPaths(paths []string) error {
	for _, path := range paths {
		if err := validateProtoPath(path); err != nil {
			return err
		}
	}
	return nil
}

// validateProtoPath validates that the path is non-empty, relative, uses '/' as the
// path separator, is equal to filepath.ToSlash(filepath.Clean(path)),
// and has .proto as the file extension.
func validateProtoPath(path string) error {
	if err := validateAndCheckNormalizedPath(path); err != nil {
		return err
	}
	if filepath.Ext(path) != ".proto" {
		return fmt.Errorf("expected path %q to have the .proto file extension", path)
	}
	return nil
}

// validateAndCheckNormalizedPath validates that the path is non-empty, relative, and uses '/' as the
// path separator, and is equal to filepath.ToSlash(filepath.Clean(path)).
func validateAndCheckNormalizedPath(path string) error {
	if path == "" {
		return errors.New("expected path to be non-empty")
	}
	normalizedPath, err := validateAndNormalizePath(path)
	if err != nil {
		return err
	}
	if path != normalizedPath {
		return fmt.Errorf("expected path %q to be given as %q", path, normalizedPath)
	}
	return nil
}

// validateAndNormalizePath validates that the path is non-empty, relative, and uses '/' as the
// path separator, and returns filepath.ToSlash(filepath.Clean(path)). It does not
// validate that the path is equal to the normalized value.
func validateAndNormalizePath(path string) (string, error) {
	if path == "" {
		return "", errors.New("expected path to be non-empty")
	}
	normalizedPath := filepath.ToSlash(filepath.Clean(path))
	if filepath.IsAbs(normalizedPath) {
		return "", fmt.Errorf("expected path %q to be relative", path)
	}
	// https://github.com/bufbuild/buf/issues/51
	if strings.HasPrefix(normalizedPath, "../") {
		return "", fmt.Errorf("expected path %q to not jump context", path)
	}
	return normalizedPath, nil
}
