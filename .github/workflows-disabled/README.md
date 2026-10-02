# Disabled workflows

These workflows came with the Crossplane provider template and only make sense
once the provider has releases. GitHub does not run workflows outside of
`.github/workflows`, so they are parked here until then:

- `tag.yml` creates a release tag.
- `promote.yml` promotes released artifacts to a channel. It needs registry
  credentials.
- `backport.yml` and `commands.yml` open backport pull requests to release
  branches and handle comment commands.

To enable one, move it back to `.github/workflows`.
