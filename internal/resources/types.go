package resources

import (
	"crypto/tls"
	"net/http"

	"github.com/docker/docker/client"
)

// DockerClientConfig represents the Docker client configuration
type DockerClientConfig struct {
	Host       string
	CertPath   string
	KeyPath    string
	CaPath     string
	APIVersion string
}

// SwarmProviderData holds comprehensive provider configuration
type SwarmProviderData struct {
	NodeConfigs map[string]*DockerClientConfig
}

// createDockerClient creates a Docker client from configuration
func createDockerClient(clientConfig *DockerClientConfig) (*client.Client, error) {
	var httpClient *http.Client
	if clientConfig.CertPath != "" && clientConfig.KeyPath != "" && clientConfig.CaPath != "" {
		tlsConfig := &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
		httpClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsConfig,
			},
		}
	}

	return client.NewClientWithOpts(
		client.WithHost(clientConfig.Host),
		client.WithAPIVersionNegotiation(),
		client.WithHTTPClient(httpClient),
	)
}
