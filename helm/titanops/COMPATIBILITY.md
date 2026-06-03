# TitanOps Compatibility Matrix

## Overview

This document defines the version compatibility between the TitanOps umbrella chart and its module sub-chart dependencies. The umbrella chart will reject dependency resolution if a module sub-chart does not satisfy the version constraints listed below.

## Current Compatibility Matrix

| Umbrella Chart Version | Tlapix | Earthworm | eBeeControl | Quack | Correlation Engine | API Gateway | Dashboard |
|------------------------|--------|-----------|-------------|-------|--------------------|-------------|-----------|
| 0.1.x                 | >=0.1.0 | >=0.1.0   | >=0.1.0     | >=0.1.0 | >=0.1.0          | >=0.1.0     | >=0.1.0   |

## Shared Library Compatibility

| Umbrella Chart Version | titanops-ai | titanops-k8s | titanops-export | titanops-config |
|------------------------|-------------|--------------|-----------------|-----------------|
| 0.1.x                 | >=0.1.0     | >=0.1.0      | >=0.1.0         | >=0.1.0         |

## Upgrade Rules

### Minor Version Upgrades (e.g., 0.1.x → 0.2.x)

- Sub-charts may add new features
- No breaking changes to existing APIs
- Umbrella chart should be compatible without modification

### Major Version Upgrades (e.g., 0.x → 1.x)

- Sub-charts may contain breaking changes
- Umbrella chart compatibility matrix MUST be updated
- Review release notes for migration steps
- Run `helm diff` before applying

## Validation

The umbrella chart validates sub-chart versions at install time via Chart.yaml dependency constraints. If a version constraint is not satisfied, the install will fail with an error identifying:

- The module that failed version resolution
- The required version range
- The available version

## Adding New Modules

When adding a new module sub-chart:

1. Add the dependency to `Chart.yaml`
2. Add a row to the compatibility matrix above
3. Define the minimum supported version
4. Test with `helm dependency build`
5. Update the umbrella chart minor version
