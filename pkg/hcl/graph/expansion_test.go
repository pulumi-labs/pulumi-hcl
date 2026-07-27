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
	"sync"
	"testing"
	"time"

	"github.com/pulumi/pulumi/pkg/v3/util/pdag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// eventLog records execution order across concurrent walk goroutines.
type eventLog struct {
	mu    sync.Mutex
	order []string
}

func (e *eventLog) add(s string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.order = append(e.order, s)
}

func record(e *eventLog, name string) func(context.Context) error {
	return func(context.Context) error {
		e.add(name)
		return nil
	}
}

// consumerNode adds an exec node with the given prerequisites, armed
// immediately.
func consumerNode(t *testing.T, g *Graph, exec func(context.Context) error, deps ...pdag.Node) pdag.Node {
	t.Helper()
	n, arm := g.dag.NewNode(dagNode{exec: exec})
	for _, dep := range deps {
		require.NoError(t, g.dag.NewEdge(dep, n))
	}
	arm()
	return n
}

// A consumer gated on one instance runs as soon as that instance finishes,
// before a delayed sibling instance; a consumer bound to Complete waits for
// every instance.
func TestBlockExpansionGateRunsBeforeDelayedSibling(t *testing.T) {
	t.Parallel()
	g := NewGraph()
	log := &eventLog{}

	narrowDone := make(chan struct{})

	var b *BlockExpansion
	b = g.NewBlockExpansion(nk("order_resource.a"), false, func(context.Context) error {
		if err := b.AddInstance(`["x"]`, record(log, "a-x")); err != nil {
			return err
		}
		return b.AddInstance(`["y"]`, func(context.Context) error {
			// Simulate the delayed sibling: hold the instance open until the
			// narrow consumer has run. If the gate wrongly waits for the whole
			// block this times out and the recorded order flips.
			select {
			case <-narrowDone:
			case <-time.After(5 * time.Second):
				log.add("timeout")
			}
			log.add("a-y")
			return nil
		})
	})

	consumerNode(t, g, func(context.Context) error {
		log.add("narrow")
		close(narrowDone)
		return nil
	}, b.Gate(`["x"]`))
	consumerNode(t, g, record(log, "whole"), b.Complete())

	b.Arm()
	require.NoError(t, g.dag.Walk(t.Context(), func(ctx context.Context, n dagNode) error {
		return n.exec(ctx)
	}, pdag.MaxProcs(4)))

	assert.Equal(t, []string{"a-x", "narrow", "a-y", "whole"}, log.order)
}

// A gate whose suffix matches no instance falls back to whole-block ordering:
// the consumer runs only after every instance completes.
func TestBlockExpansionUnmatchedGateFallsBack(t *testing.T) {
	t.Parallel()
	g := NewGraph()
	log := &eventLog{}

	var b *BlockExpansion
	b = g.NewBlockExpansion(nk("order_resource.a"), false, func(context.Context) error {
		return b.AddInstance(`[0]`, record(log, "a-0"))
	})

	consumerNode(t, g, record(log, "consumer"), b.Gate(`[5]`))

	b.Arm()
	require.NoError(t, g.dag.Walk(t.Context(), func(ctx context.Context, n dagNode) error {
		return n.exec(ctx)
	}, pdag.MaxProcs(4)))

	assert.Equal(t, []string{"a-0", "consumer"}, log.order)
}

// An expand exec that errors must not strand its gates, complete node, or
// consumers: the walk terminates and reports the error.
func TestBlockExpansionExpandErrorDoesNotHang(t *testing.T) {
	t.Parallel()
	g := NewGraph()

	expandErr := errors.New("count evaluation failed")
	var b *BlockExpansion
	b = g.NewBlockExpansion(nk("order_resource.a"), false, func(context.Context) error {
		if err := b.AddInstance(`[0]`, func(context.Context) error { return nil }); err != nil {
			return err
		}
		return expandErr
	})

	consumerNode(t, g, func(context.Context) error { return nil }, b.Gate(`["missing"]`))
	consumerNode(t, g, func(context.Context) error { return nil }, b.Complete())

	b.Arm()
	err := g.dag.Walk(t.Context(), func(ctx context.Context, n dagNode) error {
		return n.exec(ctx)
	}, pdag.MaxProcs(4))
	require.ErrorIs(t, err, expandErr)
}

// A static expansion interns its expand node so pre-walk passes (Validate,
// InjectAfter) can see it.
func TestBlockExpansionStaticInterning(t *testing.T) {
	t.Parallel()
	g := NewGraph()

	b := g.NewBlockExpansion(nk("order_resource.a"), true, func(context.Context) error { return nil })
	b.Arm()

	interned, ok := g.seen[nk("order_resource.a!expand")]
	require.True(t, ok)
	assert.Equal(t, &Node{Key: nk("order_resource.a!expand"), Type: NodeTypeBuiltin}, interned.n)
	assert.Empty(t, g.Validate())
}
