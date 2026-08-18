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

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
)

// WithCollapsedConstructAliases wraps a provider server so that spec-form
// aliases on an incoming Construct request are collapsed into URN aliases
// before the request is handled. pulumi-go-provider's typed ConstructRequest
// carries aliases as bare URNs, so spec aliases (e.g. `noParent: true`) would
// otherwise be silently discarded and an aliased component would be recreated
// instead of preserved. Once
// https://github.com/pulumi/pulumi-go-provider/issues/572 is fixed this
// wrapper can be removed.
func WithCollapsedConstructAliases(inner pulumirpc.ResourceProviderServer) pulumirpc.ResourceProviderServer {
	return &collapsedConstructAliases{ResourceProviderServer: inner}
}

type collapsedConstructAliases struct {
	pulumirpc.ResourceProviderServer
}

func (s *collapsedConstructAliases) Construct(
	ctx context.Context, req *pulumirpc.ConstructRequest,
) (*pulumirpc.ConstructResponse, error) {
	req.Aliases = collapseSpecAliases(
		req.GetAliases(), req.GetType(), req.GetName(), req.GetParent(), req.GetProject(), req.GetStack())
	return s.ResourceProviderServer.Construct(ctx, req)
}

// collapseSpecAliases resolves each spec-form alias against the constructed
// component's own type, name, parent, project, and stack into the URN it
// denotes, leaving URN-form aliases untouched.
func collapseSpecAliases(aliases []*pulumirpc.Alias, typ, name, parent, project, stack string) []*pulumirpc.Alias {
	collapsed := make([]*pulumirpc.Alias, len(aliases))
	for i, alias := range aliases {
		spec := alias.GetSpec()
		if spec == nil {
			collapsed[i] = alias
			continue
		}
		aliasName := spec.GetName()
		if aliasName == "" {
			aliasName = name
		}
		aliasType := spec.GetType()
		if aliasType == "" {
			aliasType = typ
		}
		aliasParent := spec.GetParentUrn()
		if aliasParent == "" && !spec.GetNoParent() {
			aliasParent = parent
		}
		if spec.GetNoParent() ||
			(aliasParent != "" && resource.URN(aliasParent).QualifiedType() == resource.RootStackType) {
			aliasParent = ""
		}
		aliasProject := spec.GetProject()
		if aliasProject == "" {
			aliasProject = project
		}
		aliasStack := spec.GetStack()
		if aliasStack == "" {
			aliasStack = stack
		}
		urn := resource.CreateURN(aliasName, aliasType, resource.URN(aliasParent), aliasProject, aliasStack)
		collapsed[i] = &pulumirpc.Alias{Alias: &pulumirpc.Alias_Urn{Urn: string(urn)}}
	}
	return collapsed
}
