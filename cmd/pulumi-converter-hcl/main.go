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

// pulumi-converter-hcl converts between HCL and PCL (Pulumi Configuration Language).
package main

import (
	"fmt"
	"os"

	"github.com/pulumi/pulumi-language-hcl/pkg/version"
	"github.com/spf13/cobra"
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "pulumi-converter-hcl",
		Short: "Convert between HCL and PCL",
		Long: `pulumi-converter-hcl converts between HCL (HashiCorp Configuration Language)
and PCL (Pulumi Configuration Language).`,
		Run: func(cmd *cobra.Command, args []string) {
			// TODO: Implement converter gRPC server
			fmt.Fprintln(os.Stderr, "Converter not yet implemented")
			os.Exit(1)
		},
	}

	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Print the version number",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(version.GetVersion())
		},
	}

	rootCmd.AddCommand(versionCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
