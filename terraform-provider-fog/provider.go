package main

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ provider.Provider = (*fogProvider)(nil)

func New() provider.Provider {
	return &fogProvider{}
}

type fogProvider struct {
	config providerConfig
}

type providerConfig struct {
	SSHUser       types.String `tfsdk:"ssh_user"`
	SSHPrivateKey types.String `tfsdk:"ssh_private_key"`
	SSHPort       types.Int64  `tfsdk:"ssh_port"`
}

func (p *fogProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "fog"
}

func (p *fogProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Provider for fog/edge bare-metal nodes.",
		Attributes: map[string]schema.Attribute{
			"ssh_user": schema.StringAttribute{
				Description: "SSH user for all fog nodes (can be overridden per resource).",
				Optional:    true,
			},
			"ssh_private_key": schema.StringAttribute{
				Description: "SSH private key in PEM format.",
				Optional:    true,
				Sensitive:   true,
			},
			"ssh_port": schema.Int64Attribute{
				Description: "SSH port. Defaults to 22.",
				Optional:    true,
			},
		},
	}
}

func (p *fogProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config providerConfig
	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	p.config = config

	resp.ResourceData = p
	resp.DataSourceData = p
}

func (p *fogProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		func() resource.Resource { return &systemdServiceResource{} },
	}
}

func (p *fogProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return nil
}
