package provider

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/api/types/swarm"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testAccProtoV6ProviderFactories serve the provider to the Terraform CLI of
// each acceptance test.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"swarm": providerserver.NewProtocol6WithError(New("test")()),
}

// testEndpoint is the Docker the acceptance tests init and leave swarms on:
// DOCKER_HOST, else the local socket.
func testEndpoint() string {
	return cmp.Or(os.Getenv("DOCKER_HOST"), "unix:///var/run/docker.sock")
}

// testAccPreCheck checks that the test's Docker answers and runs no swarm,
// which the tests would leave, and makes it leave the test's swarm when
// the test ends.
func testAccPreCheck(t *testing.T) {
	t.Helper()
	api := testDocker(t)
	result, err := api.Info(context.Background(), client.InfoOptions{})
	require.NoError(t, err, "the acceptance tests init swarms on %s", testEndpoint())
	require.Equal(t, swarm.LocalNodeStateInactive, result.Info.Swarm.LocalNodeState,
		"the acceptance tests init and leave swarms: give them a Docker without one in DOCKER_HOST, such as a docker:dind")
	t.Cleanup(func() { _, _ = api.SwarmLeave(context.Background(), client.SwarmLeaveOptions{Force: true}) })
}

// testDocker is a client of the test's Docker.
func testDocker(t *testing.T) *client.Client {
	t.Helper()
	api, err := connect(testEndpoint(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = api.Close() })
	return api
}

// loadTestConfiguration reads the configuration of an acceptance test, in
// testdata/resources/<resource>/<test>.tf.
func loadTestConfiguration(t *testing.T, resourceName, testName string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "resources", resourceName, testName+".tf"))
	require.NoError(t, err)
	return string(data)
}

// switchableProxy forwards TCP connections to the test's Docker until it is
// switched off: then it closes them at once, as a daemon that is down.
type switchableProxy struct {
	listener net.Listener
	on       atomic.Bool
}

func newSwitchableProxy(t *testing.T) *switchableProxy {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	proxy := &switchableProxy{listener: listener}
	proxy.on.Store(true)
	network, address, _ := strings.Cut(testEndpoint(), "://")
	if network != "unix" {
		network = "tcp"
	}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				if !proxy.on.Load() {
					return
				}
				upstream, err := net.Dial(network, address)
				if err != nil {
					return
				}
				defer func() { _ = upstream.Close() }()
				go func() { _, _ = io.Copy(upstream, conn) }()
				_, _ = io.Copy(conn, upstream)
			}()
		}
	}()
	t.Cleanup(func() { _ = listener.Close() })
	return proxy
}

func (proxy *switchableProxy) endpoint() string {
	return "tcp://" + proxy.listener.Addr().String()
}

// testNode is a Docker daemon in a docker:dind container, for the tests of
// several nodes: the test reaches it at its endpoint, the other nodes at
// its address.
type testNode struct {
	container, endpoint, address string
}

// startNodes starts n daemons in docker:dind containers on one network of
// the test's Docker, and removes them when the test ends.
func startNodes(t *testing.T, n int) []testNode {
	t.Helper()
	ctx := context.Background()
	api := testDocker(t)
	const image = "docker:29-dind"
	if _, err := api.ImageInspect(ctx, image); err != nil {
		pull, err := api.ImagePull(ctx, image, client.ImagePullOptions{})
		require.NoError(t, err)
		_, _ = io.Copy(io.Discard, pull)
		_ = pull.Close()
	}
	name := fmt.Sprintf("swarm-acc-%d", time.Now().UnixNano())
	created, err := api.NetworkCreate(ctx, name, client.NetworkCreateOptions{Driver: "bridge"})
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = api.NetworkRemove(context.Background(), created.ID, client.NetworkRemoveOptions{}) })

	port := network.MustParsePort("2375/tcp")
	nodes := make([]testNode, n)
	for i := range nodes {
		c, err := api.ContainerCreate(ctx, client.ContainerCreateOptions{
			Name:   fmt.Sprintf("%s-%d", name, i+1),
			Config: &container.Config{Image: image, Env: []string{"DOCKER_TLS_CERTDIR="}, ExposedPorts: network.PortSet{port: {}}},
			HostConfig: &container.HostConfig{
				Privileged:   true,
				PortBindings: network.PortMap{port: {{HostIP: netip.MustParseAddr("127.0.0.1")}}},
			},
			NetworkingConfig: &network.NetworkingConfig{EndpointsConfig: map[string]*network.EndpointSettings{name: {}}},
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = api.ContainerRemove(context.Background(), c.ID, client.ContainerRemoveOptions{Force: true, RemoveVolumes: true})
		})
		_, err = api.ContainerStart(ctx, c.ID, client.ContainerStartOptions{})
		require.NoError(t, err)
		inspected, err := api.ContainerInspect(ctx, c.ID, client.ContainerInspectOptions{})
		require.NoError(t, err)
		nodes[i] = testNode{
			container: c.ID,
			endpoint:  "tcp://127.0.0.1:" + inspected.Container.NetworkSettings.Ports[port][0].HostPort,
			address:   inspected.Container.NetworkSettings.Networks[name].IPAddress.String(),
		}
	}
	ready, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	for _, node := range nodes {
		d, err := reach(ready, node.endpoint, nil, true)
		require.NoError(t, err, "the daemon of a test node")
		_ = d.api.Close()
	}
	return nodes
}

func TestSwarmProvider_schema(t *testing.T) {
	t.Parallel()
	var resp provider.SchemaResponse
	New("test")().Schema(context.Background(), provider.SchemaRequest{}, &resp)
	assert.False(t, resp.Diagnostics.HasError(), resp.Diagnostics)
	assert.False(t, resp.Schema.ValidateImplementation(context.Background()).HasError(), "a schema the framework serves")
}
