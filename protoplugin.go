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
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/pluginpb"
)

var (
	// OSEnv is the os-based Env used in Main.
	OSEnv = Env{
		Args:    os.Args[1:],
		Environ: os.Environ(),
		Stdin:   os.Stdin,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
	}
	interruptSignals = append([]os.Signal{os.Interrupt}, extraInterruptSignals...)
)

// Main simplifies the authoring of main functions to invoke Handler.
//
// Main will handle interrupt signals, and exit with a non-zero exit code if the Handler
// returns an error.
//
//	func main() {
//	  protoplugin.Main(newHandler())
//	}
func Main(handler Handler, options ...MainOption) {
	opts := newOpts()
	for _, option := range options {
		option.applyMainOption(opts)
	}
	ctx, cancel := withCancelInterruptSignal(context.Background())
	if err := run(ctx, OSEnv, handler, opts); err != nil {
		exitError := &exec.ExitError{}
		if errors.As(err, &exitError) {
			cancel()
			// Swallow error message - it was printed via os.Stderr redirection.
			os.Exit(exitError.ExitCode())
		}
		if errString := err.Error(); errString != "" {
			_, _ = fmt.Fprintln(os.Stderr, errString)
		}
		cancel()
		os.Exit(1)
	}
	cancel()
}

// Run runs the plugin using the Handler for the given stdio.
//
// This is the function that Main calls to invoke Handlers. However, Run gives you control over
// the environment, and does not provide additional functionality such as handling interrupts. Run is useful
// when writing plugin tests, or if you want to use your own custom logic for main functions.
func Run(
	ctx context.Context,
	env Env,
	handler Handler,
	options ...RunOption,
) error {
	opts := newOpts()
	for _, option := range options {
		option.applyRunOption(opts)
	}
	return run(ctx, env, handler, opts)
}

// Env represents an environment for a plugin to run within.
//
// This wraps items like args, environment variables, and stdio.
//
// When calling Main, this uses the values from the os package: os.Args[1:], os.Environ,
// os.Stdin, os.Stdout, and os.Stderr.
type Env struct {
	// Args are the program arguments.
	//
	// Does not include the program name.
	Args []string
	// Environment are the environment variables.
	Environ []string
	// Stdin is the stdin for the plugin.
	Stdin io.Reader
	// Stdout is the stdout for the plugin.
	Stdout io.Writer
	// Stderr is the stderr for the plugin.
	Stderr io.Writer
}

// MainOption is an option for Main.
type MainOption interface {
	applyMainOption(opts *opts)
}

// RunOption is an option for Run.
//
// Note that RunOptions are also MainOptions, so all RunOptions can also be passed to Main.
type RunOption interface {
	MainOption

	applyRunOption(opts *opts)
}

// WithVersion returns a new RunOption that will result in the given version string being printed
// to stdout if the plugin is given the --version flag.
//
// This can be passed to Main or Run,
//
// The default is no version flag is installed.
func WithVersion(version string) RunOption {
	return optsFunc(func(opts *opts) {
		opts.version = version
	})
}

// GenerateOption is an option for Generate.
//
// Note that GenerateOptions are also RunOptions, and therefore MainOptions, so all GenerateOptions
// can also be passed to Run or Main.
type GenerateOption interface {
	RunOption

	applyGenerateOption(opts *opts)
}

// WithWarningHandler returns a new GenerateOption that says to handle warnings with the given function.
//
// This can be passed to either Main, Run, or Generate.
//
// The default is to write warnings to stderr.
//
// Implementers of warningHandlerFunc can assume that errors passed will be non-nil and have non-empty
// values for err.Error().
func WithWarningHandler(warningHandlerFunc func(error)) GenerateOption {
	return optsFunc(func(opts *opts) {
		opts.warningHandlerFunc = warningHandlerFunc
	})
}

/// *** PRIVATE ***

func run(
	ctx context.Context,
	env Env,
	handler Handler,
	opts *opts,
) error {
	switch len(env.Args) {
	case 0:
	case 1:
		if opts.version != "" && env.Args[0] == "--version" {
			_, err := fmt.Fprintln(env.Stdout, opts.version)
			return err
		}
		return newUnknownArgumentsError(env.Args)
	default:
		return newUnknownArgumentsError(env.Args)
	}

	input, err := io.ReadAll(env.Stdin)
	if err != nil {
		return err
	}
	codeGeneratorRequest := &pluginpb.CodeGeneratorRequest{}
	if err := proto.Unmarshal(input, codeGeneratorRequest); err != nil {
		return err
	}
	codeGeneratorResponse, err := generate(ctx, env.Environ, env.Stderr, handler, codeGeneratorRequest, opts)
	if err != nil {
		return err
	}
	data, err := proto.Marshal(codeGeneratorResponse)
	if err != nil {
		return err
	}
	_, err = env.Stdout.Write(data)
	return err
}

func generate(
	ctx context.Context,
	environ []string,
	stderr io.Writer,
	handler Handler,
	codeGeneratorRequest *pluginpb.CodeGeneratorRequest,
	opts *opts,
) (*pluginpb.CodeGeneratorResponse, error) {
	if opts.warningHandlerFunc == nil {
		opts.warningHandlerFunc = func(err error) { _, _ = fmt.Fprintln(stderr, err.Error()) }
	}

	request, err := newRequest(codeGeneratorRequest)
	if err != nil {
		return nil, err
	}
	responseWriter := newResponseWriter(opts.warningHandlerFunc)
	if err := handler.Handle(ctx, responseWriter, request); err != nil {
		return nil, err
	}
	return responseWriter.toCodeGeneratorResponse()
}

// withCancelInterruptSignal returns a context that is cancelled if interrupt signals are sent.
func withCancelInterruptSignal(ctx context.Context) (context.Context, context.CancelFunc) {
	interruptSignalC, closer := newInterruptSignalChannel()
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		<-interruptSignalC
		closer()
		cancel()
	}()
	return ctx, cancel
}

// newInterruptSignalChannel returns a new channel for interrupt signals.
//
// Call the returned function to cancel sending to this channel.
func newInterruptSignalChannel() (<-chan os.Signal, func()) {
	signalC := make(chan os.Signal, 1)
	signal.Notify(signalC, interruptSignals...)
	return signalC, func() {
		signal.Stop(signalC)
		close(signalC)
	}
}

type opts struct {
	warningHandlerFunc func(error)
	version            string
}

func newOpts() *opts {
	return &opts{}
}

type optsFunc func(*opts)

func (f optsFunc) applyMainOption(opts *opts) {
	f(opts)
}

func (f optsFunc) applyRunOption(opts *opts) {
	f(opts)
}

func (f optsFunc) applyGenerateOption(opts *opts) {
	f(opts)
}
