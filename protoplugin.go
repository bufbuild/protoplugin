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
	return &warningHandlerOption{warningHandlerFunc: warningHandlerFunc}
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
	// Reserving args so we can use it for flags.
	_ = args

	warningHandlerFunc := runOptions.warningHandlerFunc
	if warningHandlerFunc == nil {
		warningHandlerFunc = func(err error) { _, _ = fmt.Fprintln(stderr, err.Error()) }
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
	responseWriter := newResponseWriter(warningHandlerFunc)
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
}

func newRunOptions() *runOptions {
	return &runOptions{}
}

type warningHandlerOption struct {
	warningHandlerFunc func(error)
}

func (w *warningHandlerOption) applyMainOption(runOptions *runOptions) {
	runOptions.warningHandlerFunc = w.warningHandlerFunc
}

func (w *warningHandlerOption) applyRunOption(runOptions *runOptions) {
	runOptions.warningHandlerFunc = w.warningHandlerFunc
}
