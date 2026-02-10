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

// Package packages handles Pulumi package schema loading and type mapping.
package packages

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
)

var ErrNotFound = errors.New("not found")

type InvalidToken struct {
	token, reason string
}

func (err InvalidToken) Error() string {
	var b strings.Builder
	b.WriteString("invalid token")
	if err.token != "" {
		b.WriteRune(' ')
		b.WriteString(strconv.Quote(err.token))
	}
	if err.reason != "" {
		b.WriteRune(' ')
		b.WriteString(err.reason)
	}
	return b.String()
}

func ResolveResource(ctx context.Context, loader schema.ReferenceLoader, token string) (*schema.Resource, error) {
	parts := strings.Split(token, "_")
	if len(parts) < 2 {
		return nil, InvalidToken{token: token, reason: "Pulumi HCL tokens must have at least 2 parts"}
	}

	// TODO: Thread through sufficient information to be deterministic:
	// - Version
	// - DownloadURL
	// - Parameterization

	descriptor := &schema.PackageDescriptor{Name: parts[0]}

	pkg, err := loader.LoadPackageReferenceV2(ctx, descriptor)
	if err != nil {
		return nil, fmt.Errorf("unable to load schema from %s: %w", descriptor, err)
	}

	key := strings.Join(parts[1:], "")
	for iter := pkg.Resources().Range(); iter.Next(); {
		mod := pkg.TokenToModule(iter.Token())
		name := strings.Split(iter.Token(), ":")[2]
		if strings.ToLower(mod+name) == key {
			return iter.Resource()
		}
	}
	return nil, ErrNotFound
}

func ResolveFunction(ctx context.Context, loader schema.ReferenceLoader, token string) (*schema.Function, error) {
	parts := strings.Split(token, "_")
	if len(parts) < 2 {
		return nil, InvalidToken{token: token, reason: "Pulumi HCL tokens must have at least 2 parts"}
	}

	// TODO: Thread through sufficient information to be deterministic:
	// - Version
	// - DownloadURL
	// - Parameterization
	pkg, err := loader.LoadPackageReferenceV2(ctx, &schema.PackageDescriptor{Name: parts[0]})
	if err != nil {
		return nil, fmt.Errorf("unable to load schema from %q: %w", parts[0], err)
	}

	key := strings.Join(parts[1:], "")
	for iter := pkg.Functions().Range(); iter.Next(); {
		mod := pkg.TokenToModule(iter.Token())
		name := strings.Split(iter.Token(), ":")[2]
		if strings.ToLower(mod+name) == key {
			return iter.Function()
		}
	}
	return nil, ErrNotFound
}
