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

// ValidateCodeGeneratorRequest validates that the CodeGeneratorRequest conforms to the following:
//
//   - The CodeGeneratorRequest will not be nil.
//   - file_to_generate and proto_file will be non-empty.
//   - Each FileDescriptorProto in proto_file and source_file_descriptors will have valid paths
//     as the name and dependency fields.
//   - Each FileDescriptorProto in proto_file and source_file_descriptors will have unique name fields.
//   - Each FileDescriptorProto in proto_file and source_file_descriptors will have unique values of their
//     dependency fields, that is there will be no duplicates within a single FileDescriptorProto.
//   - source_file_descriptors is either empty, or the values of file_to_generate will have a 1-1 mapping
//     to the names in source_file_descriptors.
//   - Each value of file_to_generate will be a valid path.
//   - Each value of file_to_generate will have a corresponding value in proto_file.
//   - The major, minor, and patch versions of compiler_version will be non-negative.
//
// Paths are considered valid if they are non-empty, relative, use '/' as the path separator, do not jump context,
// and have `.proto` as the file extension.
func ValidateCodeGeneratorRequest(request *pluginpb.CodeGeneratorRequest) error {
	if err := validateCodeGeneratorRequest(request); err != nil {
		return fmt.Errorf("CodeGeneratorRequest: %w", err)
	}
	return nil
}

func validateCodeGeneratorRequest(request *pluginpb.CodeGeneratorRequest) error {
	if request == nil {
		return errors.New("nil")
	}
	if len(request.GetProtoFile()) == 0 {
		return errors.New("proto_file: empty")
	}
	if len(request.GetFileToGenerate()) == 0 {
		return errors.New("file_to_generate: empty")
	}
	if err := validateProtoPaths("file_to_generate", request.GetFileToGenerate()); err != nil {
		return err
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
			return fmt.Errorf("compiler_version.major: negative: %d", int(major))
		}
		if minor := version.GetMinor(); minor < 0 {
			return fmt.Errorf("compiler_version.minor: negative: %d", int(minor))
		}
		if patch := version.GetPatch(); patch < 0 {
			return fmt.Errorf("compiler_version.patch: negative: %d", int(patch))
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
		if err := validateFileDescriptorProto(fieldName, fileDescriptorProto); err != nil {
			return err
		}
		fileDescriptorProtoName := fileDescriptorProto.GetName()
		if _, ok := fileDescriptorProtoNameMap[fileDescriptorProtoName]; ok {
			return fmt.Errorf("%s: duplicate path %q", fieldName, fileDescriptorProtoName)
		}
		fileDescriptorProtoNameMap[fileDescriptorProtoName] = struct{}{}
	}
	for _, fileToGenerate := range filesToGenerate {
		if _, ok := fileDescriptorProtoNameMap[fileToGenerate]; !ok {
			return fmt.Errorf("file_to_generate: path %q is not contained within %s", fileToGenerate, fieldName)
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
				return fmt.Errorf("%s: path %q is not contained within file_to_generate", fieldName, fileDescriptorProtoName)
			}
		}
	}
	return nil
}

func validateFileDescriptorProto(fieldName string, fileDescriptorProto *descriptorpb.FileDescriptorProto) error {
	if fileDescriptorProto == nil {
		return fmt.Errorf("%s: nil", fieldName)
	}
	if err := validateProtoPath(fieldName+".name", fileDescriptorProto.GetName()); err != nil {
		return err
	}
	if err := validateProtoPaths(fieldName+".dependency", fileDescriptorProto.GetDependency()); err != nil {
		return err
	}
	return nil
}

// validateProtoPaths validates with validateProtoPaths, and ensures that the paths are unique.
func validateProtoPaths(fieldName string, paths []string) error {
	pathMap := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		if err := validateProtoPath(fieldName, path); err != nil {
			return err
		}
		if _, ok := pathMap[path]; ok {
			return fmt.Errorf("%s: duplicate path %q", fieldName, path)
		}
		pathMap[path] = struct{}{}
	}
	return nil
}

// validateProtoPath validates that the path is non-empty, relative, uses '/' as the
// path separator, is equal to filepath.ToSlash(filepath.Clean(path)),
// and has .proto as the file extension.
func validateProtoPath(fieldName string, path string) error {
	if err := validateAndCheckNormalizedPath(path); err != nil {
		return fmt.Errorf("%s: %w", fieldName, err)
	}
	if filepath.Ext(path) != ".proto" {
		return fmt.Errorf("%s: expected path %q to have the .proto file extension", fieldName, path)
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
