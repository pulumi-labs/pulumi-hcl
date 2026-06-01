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

package pulexec

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// captureT implements pulumitest.PT and converts Fail/FailNow into a flag +
// log buffer so pt.Up / pt.Preview return without aborting the caller. Logs
// are still forwarded to the wrapped *testing.T.
type captureT struct {
	t      *testing.T
	mu     sync.Mutex
	failed atomic.Bool
	logs   strings.Builder
}

func newCaptureT(t *testing.T) *captureT {
	return &captureT{t: t}
}

func (c *captureT) Name() string    { return c.t.Name() }
func (c *captureT) TempDir() string { return c.t.TempDir() }
func (c *captureT) Helper()         { c.t.Helper() }
func (c *captureT) Cleanup(f func()) {
	c.t.Cleanup(f)
}

func (c *captureT) Log(args ...any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	msg := fmt.Sprintln(args...)
	c.logs.WriteString(msg)
	c.t.Log(args...)
}

func (c *captureT) Fail() {
	c.failed.Store(true)
}

func (c *captureT) FailNow() {
	c.Fail()
	runtime.Goexit()
}

func (c *captureT) Deadline() (time.Time, bool) {
	return c.t.Deadline()
}

func (c *captureT) Failed() bool {
	return c.failed.Load()
}

// Logs returns the buffered log messages, which include the pulumitest fatal
// description when one occurred.
func (c *captureT) Logs() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return strings.TrimSpace(c.logs.String())
}
