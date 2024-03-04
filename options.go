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

// RequestFileOption is an option for any of the file accessors on a Request.
type RequestFileOption interface {
	GenerateFileDescriptorProtosOption
	AllFilesOption
	GenerateFileDescriptorProtosOption
	AllFileDescriptorProtosOption
}

// WithSourceRetentionOptions returns a new RequestFileOption that says to include
// source-retention options on generate files.
//
// By default, only runtime-retention options are included on generate files. Note that
// source-retention options are always included on non-generate files.
func WithSourceRetentionOptions() RequestFileOption {
	return withSourceRetentionOptions{}
}

// GenerateFileDescriptorsOption is an option for Request.GenerateFileDescriptors.
type GenerateFileDescriptorsOption interface {
	applyGenerateFileDescriptorsOption(requestFileOptions *requestFileOptions)
}

// AllFilesOption is an option for Request.AllFiles.
type AllFilesOption interface {
	applyAllFilesOption(requestFileOptions *requestFileOptions)
}

// GenerateFileDescriptorProtosOption is an option for Request.GenerateFileDescriptorProtos.
type GenerateFileDescriptorProtosOption interface {
	applyGenerateFileDescriptorProtosOption(requestFileOptions *requestFileOptions)
}

// AllFileDescriptorProtosOption is an option for Request.AllFileDescriptorProtos.
type AllFileDescriptorProtosOption interface {
	applyAllFileDescriptorProtosOption(requestFileOptions *requestFileOptions)
}

// RunOption is an option for Main or Run.
type RunOption interface {
	applyRunOption(runOptions *runOptions)
}

// WithWarningHandler returns a new Option that says to handle warnings with the given function.
//
// The default is to write warnings to stderr.
//
// Implementers of warningHandlerFunc can assume that errors passed will be non-nil and have non-empty
// values for err.Error().
func WithWarningHandler(warningHandlerFunc func(error)) RunOption {
	return &warningHandlerOption{warningHandlerFunc: warningHandlerFunc}
}

/// *** PRIVATE ***

type requestFileOptions struct {
	sourceRetentionOptions bool
}

func newRequestFileOptions() *requestFileOptions {
	return &requestFileOptions{}
}

type withSourceRetentionOptions struct{}

func (withSourceRetentionOptions) applyGenerateFileDescriptorsOption(requestFileOptions *requestFileOptions) {
	requestFileOptions.sourceRetentionOptions = true
}

func (withSourceRetentionOptions) applyAllFilesOption(requestFileOptions *requestFileOptions) {
	requestFileOptions.sourceRetentionOptions = true
}

func (withSourceRetentionOptions) applyGenerateFileDescriptorProtosOption(requestFileOptions *requestFileOptions) {
	requestFileOptions.sourceRetentionOptions = true
}

func (withSourceRetentionOptions) applyAllFileDescriptorProtosOption(requestFileOptions *requestFileOptions) {
	requestFileOptions.sourceRetentionOptions = true
}

type runOptions struct {
	warningHandlerFunc func(error)
}

func newRunOptions() *runOptions {
	return &runOptions{}
}

type warningHandlerOption struct {
	warningHandlerFunc func(error)
}

func (w *warningHandlerOption) applyRunOption(runOptions *runOptions) {
	runOptions.warningHandlerFunc = w.warningHandlerFunc
}
