package main

import (
	"context"
	"fmt"
	"os"

	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
)

type Echo struct{}

type EchoArgs struct {
	Value string `pulumi:"value"`
}

type EchoState struct {
	EchoArgs
	Result string `pulumi:"result"`
}

func (Echo) Create(ctx context.Context, req infer.CreateRequest[EchoArgs]) (infer.CreateResponse[EchoState], error) {
	return infer.CreateResponse[EchoState]{
		ID: req.Name,
		Output: EchoState{
			EchoArgs: req.Inputs,
			Result:   req.Inputs.Value,
		},
	}, nil
}

func main() {
	provider, err := infer.NewProviderBuilder().
		WithResources(infer.Resource(Echo{})).
		WithModuleMap(map[tokens.ModuleName]tokens.ModuleName{
			"smoketest": "index",
		}).
		Build()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
	if err := provider.Run(context.Background(), "smoketest", "1.0.0"); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}
