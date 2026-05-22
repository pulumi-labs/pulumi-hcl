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

// Package sshd starts a containerized OpenSSH server for tests that need a
// real SSH endpoint. Docker must be reachable; tests fail (not skip) when
// it isn't.
package sshd

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Pinned to a specific tag so container behavior is stable across runs.
const Image = "lscr.io/linuxserver/openssh-server:10.2_p1-r0-ls226"

type Container struct {
	Host     string
	Port     int
	User     string
	Password string
}

func Start(ctx context.Context, t *testing.T) *Container {
	t.Helper()

	const (
		user     = "tcuser"
		password = "tcpass"
		intPort  = "2222/tcp" // linuxserver/openssh-server default
	)

	req := testcontainers.ContainerRequest{
		Image:        Image,
		ExposedPorts: []string{intPort},
		Env: map[string]string{
			"PUID":            "1000",
			"PGID":            "1000",
			"USER_NAME":       user,
			"USER_PASSWORD":   password,
			"PASSWORD_ACCESS": "true",
			"SUDO_ACCESS":     "false",
		},
		WaitingFor: wait.ForListeningPort(intPort).WithStartupTimeout(90 * time.Second),
	}

	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "starting sshd container")
	t.Cleanup(func() {
		// Fresh context: the test's may already be cancelled.
		stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = c.Terminate(stopCtx)
	})

	host, err := c.Host(ctx)
	require.NoError(t, err)
	mapped, err := c.MappedPort(ctx, intPort)
	require.NoError(t, err)

	return &Container{
		Host:     host,
		Port:     int(mapped.Num()),
		User:     user,
		Password: password,
	}
}
