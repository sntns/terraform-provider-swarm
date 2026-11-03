package provider

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/docker/cli/cli/connhelper"
	"github.com/moby/moby/api/types/system"
	"github.com/moby/moby/client"
)

// attemptTimeout bounds one attempt to reach a daemon, and retryInterval
// separates two attempts while a daemon does not answer yet.
var (
	attemptTimeout = 10 * time.Second
	retryInterval  = 5 * time.Second
)

// daemon is a node's Docker that answered, and what it said of itself.
type daemon struct {
	api  *client.Client
	info system.Info
}

// connect returns the client of a Docker endpoint: tcp://, with the TLS
// when it is given, ssh://, through the ssh command as the docker CLI, or
// unix://.
func connect(endpoint string, given *tlsModel) (*client.Client, error) {
	helper, err := connhelper.GetConnectionHelper(endpoint)
	if err != nil {
		return nil, err
	}
	if helper != nil {
		return client.New(client.WithHost(helper.Host), client.WithDialContext(helper.Dialer))
	}
	var options []client.Opt
	if given != nil && strings.HasPrefix(endpoint, "tcp://") {
		httpClient, err := tlsClient(given)
		if err != nil {
			return nil, err
		}
		// The host comes after the HTTP client, which it sets up.
		options = append(options, client.WithHTTPClient(httpClient))
	}
	return client.New(append(options, client.WithHost(endpoint))...)
}

// tlsClient is the HTTP client of a daemon in mTLS: its CA, else the
// system's, and the client's certificate.
func tlsClient(given *tlsModel) (*http.Client, error) {
	config := &tls.Config{MinVersion: tls.VersionTLS12}
	if ca := given.CAPEM.ValueString(); ca != "" {
		config.RootCAs = x509.NewCertPool()
		if !config.RootCAs.AppendCertsFromPEM([]byte(ca)) {
			return nil, errors.New("tls.ca_pem is not a PEM certificate")
		}
	}
	if cert, key := given.CertPEM.ValueString(), given.KeyPEM.ValueString(); cert != "" || key != "" {
		certificate, err := tls.X509KeyPair([]byte(cert), []byte(key))
		if err != nil {
			return nil, fmt.Errorf("tls.cert_pem and tls.key_pem are not a PEM pair: %w", err)
		}
		config.Certificates = []tls.Certificate{certificate}
	}
	return &http.Client{Transport: &http.Transport{TLSClientConfig: config}}, nil
}

// reach returns the daemon of an endpoint once it answers. With wait, it
// tries again until the context ends, for a machine that is still booting;
// without, it tries once.
func reach(ctx context.Context, endpoint string, given *tlsModel, wait bool) (*daemon, error) {
	api, err := connect(endpoint, given)
	if err != nil {
		return nil, fmt.Errorf("the endpoint %s: %w", endpoint, err)
	}
	for {
		attempt, cancel := context.WithTimeout(ctx, attemptTimeout)
		result, err := api.Info(attempt, client.InfoOptions{})
		cancel()
		if err == nil {
			return &daemon{api: api, info: result.Info}, nil
		}
		if !wait {
			_ = api.Close()
			return nil, fmt.Errorf("reaching the Docker at %s: %w", endpoint, err)
		}
		select {
		case <-ctx.Done():
			_ = api.Close()
			return nil, fmt.Errorf("reaching the Docker at %s, until the timeout: %w", endpoint, err)
		case <-time.After(retryInterval):
		}
	}
}
