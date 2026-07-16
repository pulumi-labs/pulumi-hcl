// Copyright 2026, Pulumi Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package server

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/modules"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/packages"
)

// statusError attaches a gRPC status code to err without altering its message.
// grpc-go's status.FromError finds GRPCStatus through wrapped chains, so the
// code survives further fmt.Errorf("%w") wrapping on the way to the wire.
type statusError struct {
	code codes.Code
	err  error
}

func (e statusError) Error() string              { return e.err.Error() }
func (e statusError) Unwrap() error              { return e.err }
func (e statusError) GRPCStatus() *status.Status { return status.New(e.code, e.err.Error()) }

func errorf(code codes.Code, format string, args ...any) error {
	return statusError{code: code, err: fmt.Errorf(format, args...)}
}

// withStatus assigns err the gRPC status code implied by its chain, falling
// back to fallback when nothing in the chain classifies it. An error already
// carrying a status — including one returned by a downstream RPC — keeps it.
//
// The codes are the machine-readable contract for callers like
// `pulumi package get-schema`: Unavailable and DeadlineExceeded are retriable,
// everything else is not.
func withStatus(err error, fallback codes.Code) error {
	var grpcStatus interface{ GRPCStatus() *status.Status }
	switch {
	case err == nil:
		return nil
	case errors.As(err, &grpcStatus):
		return err
	case errors.Is(err, context.Canceled):
		return statusError{codes.Canceled, err}
	case errors.Is(err, context.DeadlineExceeded):
		return statusError{codes.DeadlineExceeded, err}
	case errors.Is(err, modules.ErrNotFound):
		return statusError{codes.NotFound, err}
	case errors.Is(err, modules.ErrUnauthenticated):
		return statusError{codes.Unauthenticated, err}
	case errors.Is(err, modules.ErrPermissionDenied):
		return statusError{codes.PermissionDenied, err}
	case errors.Is(err, modules.ErrTransient):
		return statusError{codes.Unavailable, err}
	case errors.Is(err, modules.ErrInvalid):
		return statusError{codes.InvalidArgument, err}
	case errors.Is(err, packages.ErrNotFound):
		// The module references a type the resolved provider schema does not
		// carry: converting this module is unsupported, not retriable.
		return statusError{codes.Unimplemented, err}
	default:
		return statusError{fallback, err}
	}
}

// classifyErrors wraps a provider method so every error it returns leaves with
// a gRPC status code.
func classifyErrors[Req, Resp any](f func(context.Context, Req) (Resp, error)) func(context.Context, Req) (Resp, error) {
	return func(ctx context.Context, req Req) (Resp, error) {
		resp, err := f(ctx, req)
		return resp, withStatus(err, codes.Unknown)
	}
}
