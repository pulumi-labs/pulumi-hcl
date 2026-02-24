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

	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi/pulumi/pkg/v3/codegen/pcl"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
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

func PulumiTokenToHCL(token string) (string, hcl.Diagnostics) {
	if token == "pulumi:pulumi:StackReference" {
		return "pulumi_stackreference", nil
	}
	pkg, mod, name, diag := pcl.DecomposeToken(token, hcl.Range{})
	if diag.HasErrors() {
		return "", diag
	}
	hclToken := pkg
	if mod != "index" && mod != "" {
		hclToken += "_" + strings.ToLower(strings.ReplaceAll(mod, "/", "_"))
	}
	return hclToken + "_" + strings.ToLower(strings.ReplaceAll(name, "/", "_")), nil
}

func ResolveResource(ctx context.Context, loader schema.ReferenceLoader, token string) (*schema.Resource, error) {
	parts := strings.Split(token, "_")
	if len(parts) < 2 {
		return nil, InvalidToken{token: token, reason: "Pulumi HCL tokens must have at least 2 parts"}
	}

	// transform the default provider token into something the
	if provider, ok := strings.CutPrefix(token, "pulumi_providers_"); ok {
		pkg, err := resolvePackage(ctx, loader, &schema.PackageDescriptor{Name: provider})
		if err != nil {
			return nil, err
		}
		return pkg.Provider()
	}

	// TODO: Thread through sufficient information to be deterministic:
	// - Version
	// - DownloadURL
	// - Parameterization
	pkg, err := resolvePackage(ctx, loader, &schema.PackageDescriptor{Name: parts[0]})
	if err != nil {
		return nil, err
	}

	// Prevent users from needing to write pulumi_pulumi_stackreference
	if token == "pulumi_stackreference" {
		r, ok, err := pkg.Resources().Get("pulumi:pulumi:StackReference")
		contract.Assertf(ok, "stack references are there")
		return r, err
	}

	// Caveats:
	//
	// - Fails totally if '_' are in tokens
	//
	// - Looses information on where the separator between module & name are:
	//
	//	"ab:c" is the same as "a:bc"

	key := strings.Join(parts[1:], "")
	for iter := pkg.Resources().Range(); iter.Next(); {
		mod := pkg.TokenToModule(iter.Token())
		name := strings.Split(iter.Token(), ":")[2]
		rKey := strings.ReplaceAll(strings.ToLower(mod+name), "/", "")
		if rKey == key {
			return iter.Resource()
		}
	}
	return nil, ErrNotFound
}

func resolvePackage(ctx context.Context, loader schema.ReferenceLoader, descriptor *schema.PackageDescriptor) (schema.PackageReference, error) {
	if descriptor.Name == "pulumi" {
		return schema.DefaultPulumiPackage.Reference(), nil
	}

	pkg, err := loader.LoadPackageReferenceV2(ctx, descriptor)
	if err != nil {
		return nil, fmt.Errorf("unable to load schema from %s: %w", descriptor, err)
	}
	return pkg, nil

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

	pkg, err := resolvePackage(ctx, loader, &schema.PackageDescriptor{Name: parts[0]})
	if err != nil {
		return nil, err
	}

	key := strings.Join(parts[1:], "")
	for iter := pkg.Functions().Range(); iter.Next(); {
		mod := pkg.TokenToModule(iter.Token())
		name := strings.Split(iter.Token(), ":")[2]
		if strings.ToLower(mod+name) == key {
			return iter.Function()
		}
	}

	// Allow omitting the "get" on Pulumi datasources.

	key = parts[1] + "get" + strings.Join(parts[2:], "")
	for iter := pkg.Functions().Range(); iter.Next(); {
		mod := pkg.TokenToModule(iter.Token())
		name := strings.Split(iter.Token(), ":")[2]
		if strings.ToLower(mod+name) == key {
			return iter.Function()
		}
	}

	return nil, ErrNotFound
}
