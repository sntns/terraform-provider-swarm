# Terraform Provider Swarm

[![Tests](https://github.com/sntns/terraform-provider-swarm/workflows/Tests/badge.svg)](https://github.com/sntns/terraform-provider-swarm/actions?query=workflow%3ATests)
[![Release](https://github.com/sntns/terraform-provider-swarm/workflows/Release/badge.svg)](https://github.com/sntns/terraform-provider-swarm/actions?query=workflow%3ARelease)

A Terraform provider for managing Docker Swarm resources.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.19

## Building The Provider

1. Clone the repository
2. Enter the repository directory
3. Build the provider using the Go `install` command:

```shell
go install
```

## Adding Dependencies

This provider uses [Go modules](https://github.com/golang/go/wiki/Modules).
Please see the Go documentation for the most up to date information about using Go modules.

To add a new dependency `github.com/author/dependency` to your Terraform provider:

```shell
go get github.com/author/dependency
go mod tidy
```

Then commit the changes to `go.mod` and `go.sum`.

## Using the provider

Fill this in for each provider

## Developing the Provider

If you wish to work on the provider, you'll first need [Go](http://www.golang.org) installed on your machine (see [Requirements](#requirements) above).

To compile the provider, run `go install`. This will build the provider and put the provider binary in the `$GOPATH/bin` directory.

To generate or update documentation, run `go generate`.

In order to run the full suite of Acceptance tests, run `make testacc`.

*Note:* Acceptance tests create real resources, and often cost money to run.

```shell
make testacc
```

## Example Usage

```hcl
terraform {
  required_providers {
    swarm = {
      source = "sntns/swarm"
    }
  }
}

provider "swarm" {
  endpoint = "unix:///var/run/docker.sock"
}

resource "swarm_service" "example" {
  name     = "example-service"
  image    = "nginx:latest"
  replicas = 3
}

data "swarm_service" "example" {
  id = swarm_service.example.id
}
```

## Development

### Running Tests

```shell
# Run unit tests
make test

# Run acceptance tests (requires Docker Swarm)
make testacc
```

### Building for Release

```shell
# Build for all platforms
make release
```

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -am 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the Mozilla Public License 2.0 - see the [LICENSE](LICENSE) file for details.