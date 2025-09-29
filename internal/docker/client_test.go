package docker

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
)

func TestConfig_NewClient(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "default unix socket",
			config: Config{
				Host: "unix:///var/run/docker.sock",
			},
			wantErr: false,
		},
		{
			name: "tcp without certs",
			config: Config{
				Host: "tcp://localhost:2376",
			},
			wantErr: false,
		},
		{
			name: "ssh connection",
			config: Config{
				Host:    "ssh://user@host",
				SSHOpts: []string{"-o", "StrictHostKeyChecking=no"},
			},
			wantErr: false,
		},
		{
			name: "cert material only cert provided",
			config: Config{
				Host: "tcp://localhost:2376",
				Cert: "cert-content",
			},
			wantErr: true, // Should fail because key is missing
		},
		{
			name: "cert material both provided - invalid format",
			config: Config{
				Host: "tcp://localhost:2376",
				Cert: "cert-content", 
				Key:  "key-content",
			},
			wantErr: true, // Should fail because cert content is not valid PEM
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := tt.config.NewClient()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, client)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
				if client != nil {
					client.Close()
				}
			}
		})
	}
}

func TestExtractConfig(t *testing.T) {
	node := TfNode{
		Host:         types.StringValue("ssh://user@host"),
		Context:      types.StringValue("remote"),
		SSHOpts:      types.ListNull(types.StringType), // Simplified for testing
		CertMaterial: types.StringValue("cert-content"),
		KeyMaterial:  types.StringValue("key-content"),
		CaMaterial:   types.StringValue("ca-content"),
		CertPath:     types.StringValue("/path/to/certs"),
	}

	config := ExtractConfig(node)

	assert.Equal(t, "ssh://user@host", config.Host)
	assert.Equal(t, "cert-content", config.Cert)
	assert.Equal(t, "key-content", config.Key)
	assert.Equal(t, "ca-content", config.Ca)
	assert.Equal(t, "/path/to/certs", config.CertPath)
	assert.Empty(t, config.SSHOpts) // Should be empty for null list
}