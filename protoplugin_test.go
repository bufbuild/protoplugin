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
	"bytes"
	"context"
	"io"
	"io/fs"
	"sort"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/protoutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/pluginpb"
)

func TestBasic(t *testing.T) {
	t.Parallel()

	testBasic(
		t,
		[]string{
			"a.proto",
		},
		map[string][]byte{
			"a.proto": []byte(`syntax = "proto3"; package foo; message A1 {} message A2 {}`),
			"b.proto": []byte(`syntax = "proto3"; package foo; message B {}`),
		},
		HandlerFunc(
			func(
				_ context.Context,
				_ PluginEnv,
				responseWriter ResponseWriter,
				request Request,
			) error {
				for _, fileDescriptorProto := range request.FileDescriptorProtosToGenerate() {
					topLevelMessageNames := make([]string, len(fileDescriptorProto.GetMessageType()))
					for i, descriptorProto := range fileDescriptorProto.GetMessageType() {
						topLevelMessageNames[i] = descriptorProto.GetName()
					}
					responseWriter.AddFile(
						fileDescriptorProto.GetName()+".txt",
						strings.Join(topLevelMessageNames, "\n")+"\n",
					)
				}
				return nil
			},
		),
		map[string]string{
			"a.proto.txt": "A1\nA2\n",
		},
	)
}

func TestWithVersionOption(t *testing.T) {
	t.Parallel()

	run := func(args []string, runOptions ...RunOption) (string, error) {
		stdout := bytes.NewBuffer(nil)
		err := Run(
			context.Background(),
			Env{
				Args:    args,
				Environ: nil,
				Stdin:   iotest.ErrReader(io.EOF),
				Stdout:  stdout,
				Stderr:  io.Discard,
			},
			HandlerFunc(func(_ context.Context, _ PluginEnv, _ ResponseWriter, _ Request) error { return nil }),
			runOptions...,
		)
		return stdout.String(), err
	}

	var unknownArgumentsError *unknownArgumentsError
	_, err := run([]string{"--unsupported"})
	require.ErrorAs(t, err, &unknownArgumentsError)
	_, err = run([]string{"--unsupported"}, WithVersion("0.0.1"))
	require.ErrorAs(t, err, &unknownArgumentsError)
	_, err = run([]string{"--version"})
	require.ErrorAs(t, err, &unknownArgumentsError)
	_, err = run([]string{"--foo", "--bar"})
	require.ErrorAs(t, err, &unknownArgumentsError)

	out, err := run([]string{"--version"}, WithVersion("0.0.1"))
	require.NoError(t, err)
	require.Equal(t, "0.0.1\n", out)
}

func TestWithExtensionTypeResolverOption(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	fakeDescriptorProto := []byte(`
		syntax = "proto2";
		package google.protobuf;
		message FieldOptions { extensions 1000 to max; }
	`)
	extensionDeclarationProto := []byte(`
		syntax = "proto3";
		package foo;
		import "google/protobuf/descriptor.proto";
		extend google.protobuf.FieldOptions { fixed32 my_extension = 1000; }
	`)
	extFiles, err := (&protocompile.Compiler{
		Resolver: &protocompile.SourceResolver{
			Accessor: func(path string) (io.ReadCloser, error) {
				var source []byte
				switch path {
				case "google/protobuf/descriptor.proto":
					source = fakeDescriptorProto
				case "a.proto":
					source = extensionDeclarationProto
				default:
					t.Fatal("Unexpected path ", path)
				}
				return io.NopCloser(bytes.NewReader(source)), nil
			},
		},
	}).Compile(ctx, "a.proto")
	require.NoError(t, err)

	extensionFile := extFiles.FindFileByPath("a.proto")
	require.NotNil(t, extensionFile)

	resolver := &protoregistry.Types{}
	extensionDescriptor := extensionFile.Extensions().Get(0)
	extensionType := dynamicpb.NewExtensionType(extensionDescriptor)
	err = resolver.RegisterExtension(extensionType)
	require.NoError(t, err)

	fileDescriptorProtos, err := compile(ctx, map[string][]byte{
		"google/protobuf/descriptor.proto": fakeDescriptorProto,
		"a.proto": []byte(`
			syntax = "proto3";
			package foo;
			import "google/protobuf/descriptor.proto";
			extend google.protobuf.FieldOptions { float new_extension = 1000; }
			message A { int32 field = 1 [(new_extension) = 1.0]; }
		`),
	})
	require.NoError(t, err)

	codeGeneratorRequest := &pluginpb.CodeGeneratorRequest{
		FileToGenerate: []string{"a.proto"},
		ProtoFile:      fileDescriptorProtos,
	}
	codeGeneratorRequestData, err := proto.Marshal(codeGeneratorRequest)
	require.NoError(t, err)

	stdin := bytes.NewReader(codeGeneratorRequestData)
	stdout := bytes.NewBuffer(nil)

	err = Run(
		ctx,
		Env{Stdin: stdin, Stdout: stdout, Stderr: io.Discard},
		HandlerFunc(func(
			_ context.Context,
			_ PluginEnv,
			_ ResponseWriter,
			request Request,
		) error {
			files, err := request.AllFiles()
			require.NoError(t, err)

			descriptor, err := files.FindDescriptorByName("foo.A.field")
			require.NoError(t, err)

			options, ok := descriptor.Options().(*descriptorpb.FieldOptions)
			require.True(t, ok)
			require.Empty(t, options.ProtoReflect().GetUnknown())

			extensionValue := options.ProtoReflect().Get(extensionDescriptor)
			require.Equal(t, uint64(0x3f800000), extensionValue.Uint())

			return nil
		}),
		WithExtensionTypeResolver(resolver),
	)
	require.NoError(t, err)
}

func testBasic(
	t *testing.T,
	fileToGenerate []string,
	pathToData map[string][]byte,
	handler Handler,
	expectedPathToContent map[string]string,
) {
	ctx := context.Background()

	fileDescriptorProtos, err := compile(ctx, pathToData)
	require.NoError(t, err)

	codeGeneratorRequest := &pluginpb.CodeGeneratorRequest{
		FileToGenerate: fileToGenerate,
		ProtoFile:      fileDescriptorProtos,
	}
	codeGeneratorRequestData, err := proto.Marshal(codeGeneratorRequest)
	require.NoError(t, err)

	stdin := bytes.NewReader(codeGeneratorRequestData)
	stdout := bytes.NewBuffer(nil)

	err = Run(
		ctx,
		Env{
			Args:    nil,
			Environ: nil,
			Stdin:   stdin,
			Stdout:  stdout,
			Stderr:  io.Discard,
		},
		handler,
	)
	require.NoError(t, err)

	codeGeneratorResponse := &pluginpb.CodeGeneratorResponse{}
	err = proto.Unmarshal(stdout.Bytes(), codeGeneratorResponse)
	require.NoError(t, err)
	require.Nil(t, codeGeneratorResponse.Error)

	pathToContent := make(map[string]string)
	for _, file := range codeGeneratorResponse.File {
		require.NotEmpty(t, file.GetName())
		pathToContent[file.GetName()] = file.GetContent()
	}

	require.Equal(t, expectedPathToContent, pathToContent)
}

func compile(ctx context.Context, pathToData map[string][]byte) ([]*descriptorpb.FileDescriptorProto, error) {
	compiler := protocompile.Compiler{
		Resolver: &protocompile.SourceResolver{
			Accessor: func(path string) (io.ReadCloser, error) {
				data, ok := pathToData[path]
				if !ok {
					return nil, &fs.PathError{Op: "read", Path: path, Err: fs.ErrNotExist}
				}
				return io.NopCloser(bytes.NewReader(data)), nil
			},
		},
	}
	paths := make([]string, 0, len(pathToData))
	for path := range pathToData {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	files, err := compiler.Compile(ctx, paths...)
	if err != nil {
		return nil, err
	}
	fileDescriptorProtos := make([]*descriptorpb.FileDescriptorProto, len(files))
	for i, file := range files {
		fileDescriptorProtos[i] = protoutil.ProtoFromFileDescriptor(file)
	}
	return fileDescriptorProtos, nil
}
