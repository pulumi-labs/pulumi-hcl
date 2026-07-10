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

package runtime

import (
	"fmt"
	"os"
)

type stderrUIOutput struct{}

func (stderrUIOutput) Output(msg string) {
	fmt.Fprintln(os.Stderr, msg)
}

// suppressedOutputMsg is emitted once in place of a provisioner's streamed
// output when its configuration references a sensitive value, so the value
// cannot leak through command or connection logging.
const suppressedOutputMsg = "(output suppressed due to sensitive value in config)"

type discardUIOutput struct{}

func (discardUIOutput) Output(string) {}
