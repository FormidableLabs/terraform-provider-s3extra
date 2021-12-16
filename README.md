# Terraform Provider S3 Extra

S3 Extra provides extra resources for interacting with S3-compatible object storage.

## Maintenance status

**Experimental:** This project is new. We're not sure what our ongoing maintenance plan for this project will be. Bug reports, feature requests and pull requests are welcome. If you like this project, let us know!

[maintenance-image]: https://img.shields.io/badge/maintenance-experimental-blueviolet.svg

## Usage

See detailed resource documentation at the provider's Terraform Registry page.

## Requirements

- [Terraform](https://www.terraform.io/downloads.html) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.17

## Building The Provider

1. Clone the repository
1. Enter the repository directory
1. Build the provider using the Go `install` command:

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

See the `examples` directory for resource-specific examples.

## Developing the Provider

If you wish to work on the provider, you'll first need [Go](http://www.golang.org) installed on your machine (see [Requirements](#requirements) above).

To compile the provider, run `go install`. This will build the provider and put the provider binary in the `$GOPATH/bin` directory.

To generate or update documentation, run `go generate`.

In order to run the full suite of acceptance tests, run `make testacc`. You will need AWS credentials in your shell environment to run the acceptance tests. We recommend [`aws-vault`][aws-vault] to populate AWS credentials while storing them securely in your OS keychain.

*Note:* Acceptance tests create real resources and cost money to run.

```shell
# With credentials already in environment
make testacc

# With `aws-vault`
aws-vault exec profile-name -- make testacc
```

If you are a Formidable contributor, reach out to the `#operations` channel and tag a maintainer for assistance. We can provide you credentials to our least-privilege AWS account dedicated to running this provider's acceptance tests in automation.

[aws-vault]: https://github.com/99designs/aws-vault