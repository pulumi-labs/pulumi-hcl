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

package graph

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDropSpuriousCancel(t *testing.T) {
	t.Parallel()

	real := errors.New("precondition failed")

	t.Run("drops cancellation joined onto a real error", func(t *testing.T) {
		t.Parallel()
		got := dropSpuriousCancel(errors.Join(real, context.Canceled))
		assert.EqualError(t, got, "precondition failed")
	})

	t.Run("keeps a cancellation-only failure", func(t *testing.T) {
		t.Parallel()
		got := dropSpuriousCancel(errors.Join(context.Canceled))
		assert.ErrorIs(t, got, context.Canceled)
	})

	t.Run("passes a lone real error through", func(t *testing.T) {
		t.Parallel()
		got := dropSpuriousCancel(real)
		assert.Same(t, real, got)
	})

	t.Run("passes nil through", func(t *testing.T) {
		t.Parallel()
		assert.NoError(t, dropSpuriousCancel(nil))
	})
}
