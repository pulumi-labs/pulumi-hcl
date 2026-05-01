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

// Package version provides version information for the HCL language plugin.
package version

import (
	"github.com/blang/semver"
)

// version is the current version of the HCL language plugin.
// This is set at build time via ldflags.
var version string

var devVersion = semver.Version{Pre: []semver.PRVersion{{VersionStr: "dev"}}}

// Version returns the current version as a semver.Version.
// If the version string is invalid or empty, it returns a development version.
func Version() semver.Version {
	v, err := semver.ParseTolerant(version)
	if err != nil {
		return devVersion
	}
	return v
}
