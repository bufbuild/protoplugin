// Copyright 2024-2025 Buf Technologies, Inc.
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
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/pluginpb"
)

func TestValidateAndNormalizeCodeGeneratorResponseFilesWithPotentialEmptyNames(t *testing.T) {
	t.Parallel()
	testValidateAndNormalizeCodeGeneratorResponseFilesWithPotentialEmptyNames(
		t,
		[]*pluginpb.CodeGeneratorResponse_File{},
		[]*pluginpb.CodeGeneratorResponse_File{},
	)
	testValidateAndNormalizeCodeGeneratorResponseFilesWithPotentialEmptyNames(
		t,
		[]*pluginpb.CodeGeneratorResponse_File{
			{
				Name:    proto.String("file1"),
				Content: proto.String("content1"),
			},
		},
		[]*pluginpb.CodeGeneratorResponse_File{
			{
				Name:    proto.String("file1"),
				Content: proto.String("content1"),
			},
		},
	)
	testValidateAndNormalizeCodeGeneratorResponseFilesWithPotentialEmptyNames(
		t,
		[]*pluginpb.CodeGeneratorResponse_File{
			{
				Name:    proto.String("file1"),
				Content: proto.String("content1"),
			},
			{
				Content: proto.String("content2"),
			},
		},
		[]*pluginpb.CodeGeneratorResponse_File{
			{
				Name:    proto.String("file1"),
				Content: proto.String("content1content2"),
			},
		},
	)
	testValidateAndNormalizeCodeGeneratorResponseFilesWithPotentialEmptyNames(
		t,
		[]*pluginpb.CodeGeneratorResponse_File{
			{
				Name:    proto.String("file1"),
				Content: proto.String("content1"),
			},
			{
				Content: proto.String("content2"),
			},
			{
				Name:    proto.String("file3"),
				Content: proto.String("content3"),
			},
		},
		[]*pluginpb.CodeGeneratorResponse_File{
			{
				Name:    proto.String("file1"),
				Content: proto.String("content1content2"),
			},
			{
				Name:    proto.String("file3"),
				Content: proto.String("content3"),
			},
		},
	)
	testValidateAndNormalizeCodeGeneratorResponseFilesWithPotentialEmptyNames(
		t,
		[]*pluginpb.CodeGeneratorResponse_File{
			{
				Name:    proto.String("file1"),
				Content: proto.String("content1"),
			},
			{
				Content: proto.String("content2"),
			},
			{
				Name:    proto.String("file3"),
				Content: proto.String("content3"),
			},
			{
				Content: proto.String("content4"),
			},
		},
		[]*pluginpb.CodeGeneratorResponse_File{
			{
				Name:    proto.String("file1"),
				Content: proto.String("content1content2"),
			},
			{
				Name:    proto.String("file3"),
				Content: proto.String("content3content4"),
			},
		},
	)
	testValidateAndNormalizeCodeGeneratorResponseFilesWithPotentialEmptyNames(
		t,
		[]*pluginpb.CodeGeneratorResponse_File{
			{
				Name:    proto.String("file1"),
				Content: proto.String("content1"),
			},
			{
				Content: proto.String("content2"),
			},
			{
				Name:    proto.String("file3"),
				Content: proto.String("content3"),
			},
			{
				Content: proto.String("content4"),
			},
			{
				Content: proto.String("content5"),
			},
		},
		[]*pluginpb.CodeGeneratorResponse_File{
			{
				Name:    proto.String("file1"),
				Content: proto.String("content1content2"),
			},
			{
				Name:    proto.String("file3"),
				Content: proto.String("content3content4content5"),
			},
		},
	)
}

func testValidateAndNormalizeCodeGeneratorResponseFilesWithPotentialEmptyNames(
	t *testing.T,
	input []*pluginpb.CodeGeneratorResponse_File,
	expected []*pluginpb.CodeGeneratorResponse_File,
) {
	actual, err := validateAndNormalizeCodeGeneratorResponseFilesWithPotentialEmptyNames("file", input)
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}
