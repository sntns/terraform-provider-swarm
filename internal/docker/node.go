package docker

import (
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	tfTypes "github.com/hashicorp/terraform-plugin-framework/types"
)

// Config represents the configuration needed to connect to a Docker node
type TfNode struct {
	Host         tfTypes.String `tfsdk:"host"`
	Context      tfTypes.String `tfsdk:"context"`
	SSHOpts      tfTypes.List   `tfsdk:"ssh_opts"`
	CertMaterial tfTypes.String `tfsdk:"cert_material"`
	KeyMaterial  tfTypes.String `tfsdk:"key_material"`
	CaMaterial   tfTypes.String `tfsdk:"ca_material"`
	CertPath     tfTypes.String `tfsdk:"cert_path"`
}

var NodeSchema = schema.SingleNestedAttribute{
	Description: "Docker connection configuration for this node.",
	Required:    true,
	Attributes: map[string]schema.Attribute{
		"host": schema.StringAttribute{
			Description: "Docker daemon host for this node",
			Required:    true,
		},
		"context": schema.StringAttribute{
			Description: "Docker context to use for this node",
			Optional:    true,
		},
		"ssh_opts": schema.ListAttribute{
			Description: "SSH options for connecting to the Docker host",
			ElementType: tfTypes.StringType,
			Optional:    true,
		},
		"ca_material": schema.StringAttribute{
			Description: "PEM-encoded content of Docker host CA certificate",
			Optional:    true,
		},
		"cert_material": schema.StringAttribute{
			Description: "PEM-encoded content of Docker client certificate",
			Optional:    true,
		},
		"key_material": schema.StringAttribute{
			Description: "PEM-encoded content of Docker client private key",
			Optional:    true,
		},
		"cert_path": schema.StringAttribute{
			Description: "Path to directory with Docker TLS config",
			Optional:    true,
		},
	},
}

func ExtractConfig(node TfNode) Config {
	sshOpts := []string{}
	if !node.SSHOpts.IsNull() && !node.SSHOpts.IsUnknown() {
		for _, opt := range node.SSHOpts.Elements() {
			if strVal, ok := opt.(tfTypes.String); ok && !strVal.IsNull() {
				sshOpts = append(sshOpts, strVal.ValueString())
			}
		}
	}
	return Config{
		Host:     node.Host.ValueString(),
		SSHOpts:  sshOpts,
		Cert:     node.CertMaterial.ValueString(),
		Key:      node.KeyMaterial.ValueString(),
		Ca:       node.CaMaterial.ValueString(),
		CertPath: node.CertPath.ValueString(),
	}
}
