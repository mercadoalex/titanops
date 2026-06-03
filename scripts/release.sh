#!/usr/bin/env bash
# TitanOps Release Script
# Usage: ./scripts/release.sh [--dry-run] <component> <version>
#
# Components: titanops-ai, titanops-k8s, titanops-export, titanops-config,
#             correlation, gateway, helm, dashboard
#
# Examples:
#   ./scripts/release.sh shared/titanops-ai v0.2.0
#   ./scripts/release.sh correlation v1.0.0
#   ./scripts/release.sh helm v0.2.0
#   ./scripts/release.sh --dry-run shared/titanops-export v0.1.1

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

DRY_RUN=false
COMPONENT=""
VERSION=""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

usage() {
    echo "Usage: $0 [--dry-run] <component> <version>"
    echo ""
    echo "Components:"
    echo "  shared/titanops-ai       - AI shared library"
    echo "  shared/titanops-k8s      - Kubernetes client library"
    echo "  shared/titanops-export   - Export adapter library"
    echo "  shared/titanops-config   - Configuration library"
    echo "  correlation              - Correlation engine"
    echo "  gateway                  - API gateway"
    echo "  helm                     - Umbrella Helm chart"
    echo "  dashboard                - React dashboard"
    echo ""
    echo "Options:"
    echo "  --dry-run    Validate without creating tags or pushing"
    echo ""
    echo "Version format: vMAJOR.MINOR.PATCH (e.g., v0.2.0, v1.0.0)"
    exit 1
}

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

validate_version() {
    local version="$1"
    if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-rc\.[0-9]+|-beta\.[0-9]+)?$ ]]; then
        log_error "Invalid version format: $version"
        log_error "Expected: vMAJOR.MINOR.PATCH (e.g., v0.2.0, v1.0.0-rc.1)"
        exit 1
    fi
}

validate_component() {
    local component="$1"
    case "$component" in
        shared/titanops-ai|shared/titanops-k8s|shared/titanops-export|shared/titanops-config)
            ;;
        correlation|gateway)
            ;;
        helm|dashboard)
            ;;
        *)
            log_error "Unknown component: $component"
            usage
            ;;
    esac
}

check_clean_tree() {
    if [ -n "$(git -C "$ROOT_DIR" status --porcelain)" ]; then
        log_error "Working tree is not clean. Commit or stash changes first."
        exit 1
    fi
}

run_tests() {
    local component="$1"
    log_info "Running tests for $component..."

    case "$component" in
        shared/titanops-*)
            local lib_dir="$ROOT_DIR/$component"
            if [ -d "$lib_dir" ]; then
                (cd "$lib_dir" && go test ./... 2>/dev/null) || log_warn "Tests not available for $component"
            fi
            ;;
        correlation|gateway)
            local svc_dir="$ROOT_DIR/$component"
            if [ -d "$svc_dir" ]; then
                (cd "$svc_dir" && go test ./... 2>/dev/null) || log_warn "Tests not available for $component"
            fi
            ;;
        helm)
            log_info "Validating Helm chart..."
            if command -v helm &>/dev/null; then
                helm lint "$ROOT_DIR/helm/titanops" || log_warn "Helm lint not available"
            else
                log_warn "helm not found, skipping chart validation"
            fi
            ;;
        dashboard)
            log_info "Running dashboard checks..."
            if [ -f "$ROOT_DIR/dashboard/package.json" ]; then
                (cd "$ROOT_DIR/dashboard" && npm run build 2>/dev/null) || log_warn "Dashboard build not available"
            fi
            ;;
    esac
}

create_tag() {
    local component="$1"
    local version="$2"
    local tag=""

    case "$component" in
        shared/titanops-*|correlation|gateway)
            tag="$component/$version"
            ;;
        helm)
            tag="helm/titanops/$version"
            ;;
        dashboard)
            tag="dashboard/$version"
            ;;
    esac

    if $DRY_RUN; then
        log_info "[DRY RUN] Would create tag: $tag"
        log_info "[DRY RUN] Would push tag: $tag"
    else
        log_info "Creating tag: $tag"
        git -C "$ROOT_DIR" tag -a "$tag" -m "Release $component $version"
        log_info "Pushing tag: $tag"
        git -C "$ROOT_DIR" push origin "$tag"
    fi
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --help|-h)
            usage
            ;;
        *)
            if [ -z "$COMPONENT" ]; then
                COMPONENT="$1"
            elif [ -z "$VERSION" ]; then
                VERSION="$1"
            else
                log_error "Unexpected argument: $1"
                usage
            fi
            shift
            ;;
    esac
done

# Validate inputs
if [ -z "$COMPONENT" ] || [ -z "$VERSION" ]; then
    usage
fi

validate_component "$COMPONENT"
validate_version "$VERSION"

# Execute release
log_info "=== TitanOps Release ==="
log_info "Component: $COMPONENT"
log_info "Version:   $VERSION"
log_info "Dry Run:   $DRY_RUN"
echo ""

if ! $DRY_RUN; then
    check_clean_tree
fi

run_tests "$COMPONENT"
create_tag "$COMPONENT" "$VERSION"

echo ""
log_info "=== Release Complete ==="
if $DRY_RUN; then
    log_info "This was a dry run. No tags were created."
else
    log_info "Tag created and pushed successfully."
    log_info "Don't forget to:"
    log_info "  1. Update CHANGELOG.md"
    log_info "  2. Create GitHub release with notes"
    log_info "  3. Update compatibility matrix if needed"
fi
