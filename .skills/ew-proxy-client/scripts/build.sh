#!/bin/bash
# Build script for ECH Workers Proxy Client
# Supports cross-compilation for multiple platforms

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
APP_NAME="ech-workers"
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS="-X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME}"

# Function to print colored output
print_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to build for specific platform
build_platform() {
    local os=$1
    local arch=$2
    local output=$3

    print_info "Building for ${os}/${arch}..."

    GOOS=${os} GOARCH=${arch} go build \
        -ldflags "${LDFLAGS}" \
        -o "${output}" \
        main.go

    if [ $? -eq 0 ]; then
        print_info "✓ Built ${output}"
        # Get file size
        if [ "$os" = "windows" ]; then
            size=$(powershell -Command "(Get-Item '${output}').length")
        else
            size=$(stat -f%z "${output}" 2>/dev/null || stat -c%s "${output}" 2>/dev/null)
        fi
        print_info "  Size: ${size} bytes"
    else
        print_error "✗ Failed to build ${output}"
        return 1
    fi
}

# Function to create archive
create_archive() {
    local os=$1
    local arch=$2
    local output=$3

    local archive_name="${APP_NAME}-${os}-${arch}"

    print_info "Creating archive ${archive_name}..."

    case $os in
        windows)
            zip "${archive_name}.zip" "${output}"
            print_info "✓ Created ${archive_name}.zip"
            ;;
        *)
            tar -czf "${archive_name}.tar.gz" "${output}"
            print_info "✓ Created ${archive_name}.tar.gz"
            ;;
    esac
}

# Main build process
main() {
    print_info "Starting build process..."
    print_info "Version: ${VERSION}"
    print_info "Build time: ${BUILD_TIME}"
    echo ""

    # Check if Go is installed
    if ! command -v go &> /dev/null; then
        print_error "Go is not installed or not in PATH"
        exit 1
    fi

    # Check Go version (requires 1.23+ for ECH)
    GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
    print_info "Go version: ${GO_VERSION}"

    # Parse command line arguments
    PLATFORMS=""
    CREATE_ARCHIVES=false

    while [[ $# -gt 0 ]]; do
        case $1 in
            --all)
                PLATFORMS="all"
                shift
                ;;
            --archive)
                CREATE_ARCHIVES=true
                shift
                ;;
            --platform)
                PLATFORMS="$2"
                shift 2
                ;;
            --help)
                echo "Usage: $0 [OPTIONS]"
                echo ""
                echo "Options:"
                echo "  --all              Build for all platforms"
                echo "  --archive          Create archives after building"
                echo "  --platform <os>    Build for specific platform (windows, linux, darwin)"
                echo "  --help             Show this help message"
                echo ""
                echo "Examples:"
                echo "  $0 --all --archive"
                echo "  $0 --platform linux"
                exit 0
                ;;
            *)
                print_error "Unknown option: $1"
                exit 1
                ;;
        esac
    done

    # Default to current platform if not specified
    if [ -z "$PLATFORMS" ]; then
        PLATFORMS="current"
    fi

    # Create output directory
    mkdir -p dist

    # Build based on platform selection
    if [ "$PLATFORMS" = "all" ]; then
        # Build for all platforms
        print_info "Building for all platforms..."
        echo ""

        # Windows AMD64
        build_platform "windows" "amd64" "dist/${APP_NAME}-windows-amd64.exe"
        if [ "$CREATE_ARCHIVES" = true ]; then
            create_archive "windows" "amd64" "dist/${APP_NAME}-windows-amd64.exe"
        fi
        echo ""

        # Linux AMD64
        build_platform "linux" "amd64" "dist/${APP_NAME}-linux-amd64"
        if [ "$CREATE_ARCHIVES" = true ]; then
            create_archive "linux" "amd64" "dist/${APP_NAME}-linux-amd64"
        fi
        echo ""

        # Linux ARM64
        build_platform "linux" "arm64" "dist/${APP_NAME}-linux-arm64"
        if [ "$CREATE_ARCHIVES" = true ]; then
            create_archive "linux" "arm64" "dist/${APP_NAME}-linux-arm64"
        fi
        echo ""

        # macOS AMD64
        build_platform "darwin" "amd64" "dist/${APP_NAME}-darwin-amd64"
        if [ "$CREATE_ARCHIVES" = true ]; then
            create_archive "darwin" "amd64" "dist/${APP_NAME}-darwin-amd64"
        fi
        echo ""

        # macOS ARM64
        build_platform "darwin" "arm64" "dist/${APP_NAME}-darwin-arm64"
        if [ "$CREATE_ARCHIVES" = true ]; then
            create_archive "darwin" "arm64" "dist/${APP_NAME}-darwin-arm64"
        fi

    elif [ "$PLATFORMS" = "current" ]; then
        # Build for current platform
        print_info "Building for current platform..."
        echo ""

        CURRENT_OS=$(go env GOOS)
        CURRENT_ARCH=$(go env GOARCH)

        if [ "$CURRENT_OS" = "windows" ]; then
            build_platform "$CURRENT_OS" "$CURRENT_ARCH" "dist/${APP_NAME}.exe"
        else
            build_platform "$CURRENT_OS" "$CURRENT_ARCH" "dist/${APP_NAME}"
        fi

    else
        # Build for specified platform
        print_info "Building for platform: ${PLATFORMS}"
        echo ""

        case $PLATFORMS in
            windows)
                build_platform "windows" "amd64" "dist/${APP_NAME}-windows-amd64.exe"
                ;;
            linux)
                build_platform "linux" "amd64" "dist/${APP_NAME}-linux-amd64"
                ;;
            darwin)
                build_platform "darwin" "amd64" "dist/${APP_NAME}-darwin-amd64"
                ;;
            *)
                print_error "Unsupported platform: ${PLATFORMS}"
                exit 1
                ;;
        esac
    fi

    echo ""
    print_info "Build completed successfully!"
    print_info "Output directory: dist/"
}

# Run main function
main "$@"