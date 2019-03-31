# buildkite-gcp-scaler

buildkite-gcp-scaler is a simple scaling utility for running buildkite jobs on
Google Cloud Platform. It is designed to be ran inside a periodic scheduler such
as Nomad.

It uses Unmanaged Instance Groups to allow for self-terminating single-use
instances in public cloud infrastructure.

Authentication is managed by default credentials in the Google Cloud Go SDK.

## TODO

- [ ] Dynamic Token Generation with the GraphQL API. This is currently
      unimplemented as it's not _strictly_ necessary for my current use case,
      and because tokens currently have no way to be strictly single-use.

