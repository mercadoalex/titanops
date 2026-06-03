# TitanOps Versioning Policy

## Overview

All TitanOps platform components follow [Semantic Versioning 2.0.0](https://semver.org/). This document defines the versioning strategy across shared libraries, Helm charts, and the dashboard.

## Version Format

```
MAJOR.MINOR.PATCH
```

- **MAJOR**: Incremented for breaking API changes
- **MINOR**: Incremented for new features that are backwards-compatible
- **PATCH**: Incremented for backwards-compatible bug fixes

## Go Module Git Tags

Shared Go libraries use module-prefixed tags as required by the Go module system:

```
shared/titanops-ai/vMAJOR.MINOR.PATCH
shared/titanops-k8s/vMAJOR.MINOR.PATCH
shared/titanops-export/vMAJOR.MINOR.PATCH
shared/titanops-config/vMAJOR.MINOR.PATCH
```

Platform services use their own prefix:

```
correlation/vMAJOR.MINOR.PATCH
gateway/vMAJOR.MINOR.PATCH
```

## Helm Chart Versioning

Helm charts follow semver independently:

- **Umbrella chart**: `helm/titanops/Chart.yaml` → `version: MAJOR.MINOR.PATCH`
- **Module sub-charts**: `helm/charts/<module>/Chart.yaml` → `version: MAJOR.MINOR.PATCH`

The umbrella chart declares minimum compatible versions for each sub-chart in its dependency specification. See `helm/titanops/COMPATIBILITY.md` for the full matrix.

## Dashboard Versioning

The React dashboard is versioned via `dashboard/package.json`:

```
"version": "MAJOR.MINOR.PATCH"
```

## Breaking Changes

A **breaking change** to a shared library is defined as:

- Removal of a public function or type
- Rename of a public function or type
- Change to an existing function signature (parameters or return types)
- Removal of a configuration field

When a breaking change is introduced:
1. The MAJOR version number is incremented
2. Release notes document the breaking change with a migration guide
3. The umbrella chart compatibility matrix is updated

## Deprecation Policy

When a public API element is deprecated:

1. The element is annotated with `// Deprecated: use X instead. Will be removed in vN+2.0.0.`
2. The deprecated element is retained for **at least one minor version** release
3. Removal happens only in a subsequent **major version** increment
4. Deprecation is documented in release notes when first marked

## Release Process

1. Create a release branch: `release/v<VERSION>`
2. Update version references (go.mod comments, Chart.yaml, package.json)
3. Run full test suite: `./scripts/release.sh --dry-run`
4. Tag the release with appropriate module prefix
5. Publish release notes documenting all changes

## Version Constraints

- Module sub-charts declare compatible ranges of shared libraries
- The umbrella chart pins minimum versions via the compatibility matrix
- Go workspace (`go.work`) uses `replace` directives for local development

## Pre-release Versions

Pre-release versions use the format:

```
vMAJOR.MINOR.PATCH-rc.N
vMAJOR.MINOR.PATCH-beta.N
```

Pre-releases are not considered stable and may contain breaking changes.
