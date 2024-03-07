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
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/pluginpb"
)

var interruptSignals = append([]os.Signal{os.Interrupt}, extraInterruptSignals...)

// Main simplifies the authoring of main functions to invoke Handler.
//
// Main will handle interrupt signals, and exit with a non-zero exit code if the Handler
// returns an error.
//
//	func main() {
//	  protoplugin.Main(newHandler())
//	}
func Main(handler Handler, options ...MainOption) {
	runOptions := newRunOptions()
	for _, option := range options {
		option.applyMainOption(runOptions)
	}
	ctx, cancel := withCancelInterruptSignal(context.Background())
	if err := run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, handler, runOptions); err != nil {
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
// stdio, and does not provide additional functionality such as handling interrupts. Run is useful
// when writing plugin tests, or if you want to use your own custom logic for main functions.
func Run(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer, handler Handler, options ...RunOption) error {
	runOptions := newRunOptions()
	for _, option := range options {
		option.applyRunOption(runOptions)
	}
	return run(ctx, args, stdin, stdout, stderr, handler, runOptions)
}

// MainOption is an option for Main.
//
// Note that MainOptions are also RunOptions, so all MainOptions can also be passed to Run.
type MainOption interface {
	RunOption

	applyMainOption(runOptions *runOptions)
}

// WithWarningHandler returns a new MainOption that says to handle warnings with the given function.
// This can be passed to either Main or to Run, as MainOptions are also RunOptions.
//
// The default is to write warnings to stderr.
//
// Implementers of warningHandlerFunc can assume that errors passed will be non-nil and have non-empty
// values for err.Error().
func WithWarningHandler(warningHandlerFunc func(error)) MainOption {
	return mainOptionsFunc(func(runOptions *runOptions) {
		runOptions.warningHandlerFunc = warningHandlerFunc
	})
}

// WithVersion returns a new MainOption that will result in the given version string being printed
// to stdout if the plugin is given the --version flag.
//
// The default is no version flag is installed.
func WithVersion(version string) MainOption {
	return mainOptionsFunc(func(runOptions *runOptions) {
		runOptions.version = version
	})
}

// RunOption is an option for Run.
//
// Note that MainOptions are also RunOptions, so all MainOptions can also be passed to Run.
type RunOption interface {
	applyRunOption(runOptions *runOptions)
}

/// *** PRIVATE ***

func run(
	ctx context.Context,
	args []string,
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
	handler Handler,
	runOptions *runOptions,
) error {
	switch len(args) {
	case 0:
	case 1:
		if runOptions.version != "" && args[0] == "--version" {
			_, err := fmt.Fprintln(stdout, runOptions.version)
			return err
		}
		return newUnknownArgumentsError(args)
	default:
		return newUnknownArgumentsError(args)
	}

	if runOptions.warningHandlerFunc == nil {
		runOptions.warningHandlerFunc = func(err error) { _, _ = fmt.Fprintln(stderr, err.Error()) }
	}

	input, err := io.ReadAll(stdin)
	if err != nil {
		return err
	}
	codeGeneratorRequest := &pluginpb.CodeGeneratorRequest{}
	if err := proto.Unmarshal(input, codeGeneratorRequest); err != nil {
		return err
	}
	request, err := newRequest(codeGeneratorRequest)
	if err != nil {
		return err
	}
	responseWriter := newResponseWriter(runOptions.warningHandlerFunc)
	if err := handler.Handle(ctx, responseWriter, request); err != nil {
		return err
	}
	codeGeneratorResponse, err := responseWriter.toCodeGeneratorResponse()
	if err != nil {
		return err
	}
	data, err := proto.Marshal(codeGeneratorResponse)
	if err != nil {
		return err
	}
	_, err = stdout.Write(data)
	return err
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

type runOptions struct {
	warningHandlerFunc func(error)
	version            string
}

func newRunOptions() *runOptions {
	return &runOptions{}
}

type mainOptionsFunc func(*runOptions)

func (f mainOptionsFunc) applyMainOption(runOptions *runOptions) {
	f(runOptions)
}

func (f mainOptionsFunc) applyRunOption(runOptions *runOptions) {
	f(runOptions)
}

type unknownArgumentsError struct {
	args []string
}

func newUnknownArgumentsError(arguments []string) error {
	return &unknownArgumentsError{args: arguments}
}

func (e *unknownArgumentsError) Error() string {
	if len(e.args) == 1 {
		return fmt.Sprintf("unknown argument: %s", e.args[0])
	}
	return fmt.Sprintf("unknown arguments: %s", strings.Join(e.args, " "))
}
