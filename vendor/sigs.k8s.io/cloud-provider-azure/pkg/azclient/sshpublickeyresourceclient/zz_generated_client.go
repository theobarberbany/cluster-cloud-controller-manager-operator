// /*
// Copyright The Kubernetes Authors.
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
// */

// Code generated by client-gen. DO NOT EDIT.
package sshpublickeyresourceclient

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/tracing"
	armcompute "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v6"

	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/metrics"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/utils"
)

type Client struct {
	*armcompute.SSHPublicKeysClient
	subscriptionID string
	tracer         tracing.Tracer
}

func New(subscriptionID string, credential azcore.TokenCredential, options *arm.ClientOptions) (Interface, error) {
	if options == nil {
		options = utils.GetDefaultOption()
	}
	tr := options.TracingProvider.NewTracer(utils.ModuleName, utils.ModuleVersion)

	client, err := armcompute.NewSSHPublicKeysClient(subscriptionID, credential, options)
	if err != nil {
		return nil, err
	}
	return &Client{
		SSHPublicKeysClient: client,
		subscriptionID:      subscriptionID,
		tracer:              tr,
	}, nil
}

const GetOperationName = "SSHPublicKeysClient.Get"

// Get gets the SSHPublicKeyResource
func (client *Client) Get(ctx context.Context, resourceGroupName string, sshpublickeyresourceName string) (result *armcompute.SSHPublicKeyResource, err error) {

	metricsCtx := metrics.BeginARMRequest(client.subscriptionID, resourceGroupName, "SSHPublicKeyResource", "get")
	defer func() { metricsCtx.Observe(ctx, err) }()
	ctx, endSpan := runtime.StartSpan(ctx, GetOperationName, client.tracer, nil)
	defer endSpan(err)
	resp, err := client.SSHPublicKeysClient.Get(ctx, resourceGroupName, sshpublickeyresourceName, nil)
	if err != nil {
		return nil, err
	}
	//handle statuscode
	return &resp.SSHPublicKeyResource, nil
}

const ListOperationName = "SSHPublicKeysClient.List"

// List gets a list of SSHPublicKeyResource in the resource group.
func (client *Client) List(ctx context.Context, resourceGroupName string) (result []*armcompute.SSHPublicKeyResource, err error) {
	metricsCtx := metrics.BeginARMRequest(client.subscriptionID, resourceGroupName, "SSHPublicKeyResource", "list")
	defer func() { metricsCtx.Observe(ctx, err) }()
	ctx, endSpan := runtime.StartSpan(ctx, ListOperationName, client.tracer, nil)
	defer endSpan(err)
	pager := client.SSHPublicKeysClient.NewListByResourceGroupPager(resourceGroupName, nil)
	for pager.More() {
		nextResult, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		result = append(result, nextResult.Value...)
	}
	return result, nil
}
