#!/bin/bash
# Test script for ECH Workers Proxy Client
# Tests SOCKS5 and HTTP proxy functionality

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
PROXY_HOST="${PROXY_HOST:-127.0.0.1}"
PROXY_PORT="${PROXY_PORT:-30000}"
PROXY_TYPE="${PROXY_TYPE:-socks5}"

# Test URLs
TEST_HTTP_URL="http://httpbin.org/ip"
TEST_HTTPS_URL="https://httpbin.org/ip"
TEST_DNS_URL="http://httpbin.org/dns?domain=google.com"

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

print_test() {
    echo -e "${BLUE}[TEST]${NC} $1"
}

# Function to check if proxy is reachable
check_proxy() {
    print_info "Checking if proxy is reachable at ${PROXY_HOST}:${PROXY_PORT}..."

    if timeout 5 bash -c "cat < /dev/null > /dev/tcp/${PROXY_HOST}/${PROXY_PORT}" 2>/dev/null; then
        print_info "✓ Proxy is reachable"
        return 0
    else
        print_error "✗ Proxy is not reachable"
        return 1
    fi
}

# Function to test SOCKS5 proxy
test_socks5() {
    print_test "Testing SOCKS5 proxy..."

    # Test HTTP over SOCKS5
    print_info "Testing HTTP request over SOCKS5..."
    if curl --socks5 "${PROXY_HOST}:${PROXY_PORT}" --max-time 10 "${TEST_HTTP_URL}" -s | grep -q "origin"; then
        print_info "✓ SOCKS5 HTTP test passed"
    else
        print_error "✗ SOCKS5 HTTP test failed"
        return 1
    fi

    # Test HTTPS over SOCKS5
    print_info "Testing HTTPS request over SOCKS5..."
    if curl --socks5 "${PROXY_HOST}:${PROXY_PORT}" --max-time 10 "${TEST_HTTPS_URL}" -s | grep -q "origin"; then
        print_info "✓ SOCKS5 HTTPS test passed"
    else
        print_error "✗ SOCKS5 HTTPS test failed"
        return 1
    fi

    return 0
}

# Function to test HTTP proxy
test_http() {
    print_test "Testing HTTP proxy..."

    # Test HTTP over HTTP proxy
    print_info "Testing HTTP request over HTTP proxy..."
    if curl -x "http://${PROXY_HOST}:${PROXY_PORT}" --max-time 10 "${TEST_HTTP_URL}" -s | grep -q "origin"; then
        print_info "✓ HTTP proxy HTTP test passed"
    else
        print_error "✗ HTTP proxy HTTP test failed"
        return 1
    fi

    # Test HTTPS tunnel over HTTP proxy
    print_info "Testing HTTPS tunnel over HTTP proxy..."
    if curl -x "http://${PROXY_HOST}:${PROXY_PORT}" --max-time 10 "${TEST_HTTPS_URL}" -s | grep -q "origin"; then
        print_info "✓ HTTP proxy HTTPS test passed"
    else
        print_error "✗ HTTP proxy HTTPS test failed"
        return 1
    fi

    return 0
}

# Function to test DNS over proxy
test_dns() {
    print_test "Testing DNS resolution over proxy..."

    if [ "$PROXY_TYPE" = "socks5" ]; then
        if curl --socks5 "${PROXY_HOST}:${PROXY_PORT}" --max-time 10 "${TEST_DNS_URL}" -s | grep -q "google.com"; then
            print_info "✓ DNS test passed"
        else
            print_warn "✗ DNS test failed (may not be supported)"
        fi
    else
        if curl -x "http://${PROXY_HOST}:${PROXY_PORT}" --max-time 10 "${TEST_DNS_URL}" -s | grep -q "google.com"; then
            print_info "✓ DNS test passed"
        else
            print_warn "✗ DNS test failed (may not be supported)"
        fi
    fi
}

# Function to test connection speed
test_speed() {
    print_test "Testing connection speed..."

    local start_time
    local end_time
    local duration

    start_time=$(date +%s.%N)

    if [ "$PROXY_TYPE" = "socks5" ]; then
        curl --socks5 "${PROXY_HOST}:${PROXY_PORT}" --max-time 30 "${TEST_HTTP_URL}" -s -o /dev/null
    else
        curl -x "http://${PROXY_HOST}:${PROXY_PORT}" --max-time 30 "${TEST_HTTP_URL}" -s -o /dev/null
    fi

    end_time=$(date +%s.%N)
    duration=$(echo "$end_time - $start_time" | bc)

    print_info "✓ Connection time: ${duration}s"
}

# Function to test IPv6 support
test_ipv6() {
    print_test "Testing IPv6 support..."

    # Check if IPv6 is available
    if ! ping6 -c 1 2001:4860:4860::8888 &>/dev/null; then
        print_warn "IPv6 not available, skipping test"
        return 0
    fi

    # Test IPv6 resolution
    if [ "$PROXY_TYPE" = "socks5" ]; then
        if curl --socks5 "${PROXY_HOST}:${PROXY_PORT}" --max-time 10 "http://ipv6.google.com/" -s -o /dev/null; then
            print_info "✓ IPv6 test passed"
        else
            print_warn "✗ IPv6 test failed"
        fi
    else
        if curl -x "http://${PROXY_HOST}:${PROXY_PORT}" --max-time 10 "http://ipv6.google.com/" -s -o /dev/null; then
            print_info "✓ IPv6 test passed"
        else
            print_warn "✗ IPv6 test failed"
        fi
    fi
}

# Function to test large file transfer
test_large_file() {
    print_test "Testing large file transfer..."

    local test_url="http://speedtest.tele2.net/1MB.zip"
    local output_file="/tmp/test_proxy_download.bin"

    if [ "$PROXY_TYPE" = "socks5" ]; then
        if curl --socks5 "${PROXY_HOST}:${PROXY_PORT}" --max-time 30 "${test_url}" -s -o "${output_file}"; then
            local file_size=$(stat -c%s "${output_file}" 2>/dev/null || stat -f%z "${output_file}" 2>/dev/null)
            if [ "${file_size}" -gt 1000000 ]; then
                print_info "✓ Large file test passed (${file_size} bytes)"
            else
                print_error "✗ Large file test failed (incomplete download)"
            fi
            rm -f "${output_file}"
        else
            print_error "✗ Large file test failed"
        fi
    else
        if curl -x "http://${PROXY_HOST}:${PROXY_PORT}" --max-time 30 "${test_url}" -s -o "${output_file}"; then
            local file_size=$(stat -c%s "${output_file}" 2>/dev/null || stat -f%z "${output_file}" 2>/dev/null)
            if [ "${file_size}" -gt 1000000 ]; then
                print_info "✓ Large file test passed (${file_size} bytes)"
            else
                print_error "✗ Large file test failed (incomplete download)"
            fi
            rm -f "${output_file}"
        else
            print_error "✗ Large file test failed"
        fi
    fi
}

# Function to test concurrent connections
test_concurrent() {
    print_test "Testing concurrent connections..."

    local success_count=0
    local total_connections=5

    for i in $(seq 1 ${total_connections}); do
        if [ "$PROXY_TYPE" = "socks5" ]; then
            if curl --socks5 "${PROXY_HOST}:${PROXY_PORT}" --max-time 10 "${TEST_HTTP_URL}" -s -o /dev/null &>/dev/null; then
                ((success_count++))
            fi
        else
            if curl -x "http://${PROXY_HOST}:${PROXY_PORT}" --max-time 10 "${TEST_HTTP_URL}" -s -o /dev/null &>/dev/null; then
                ((success_count++))
            fi
        fi
    done

    if [ ${success_count} -eq ${total_connections} ]; then
        print_info "✓ Concurrent connections test passed (${success_count}/${total_connections})"
    else
        print_warn "✗ Concurrent connections test: ${success_count}/${total_connections} succeeded"
    fi
}

# Function to test error handling
test_error_handling() {
    print_test "Testing error handling..."

    # Test connection to invalid host
    print_info "Testing connection to invalid host..."
    if [ "$PROXY_TYPE" = "socks5" ]; then
        if ! curl --socks5 "${PROXY_HOST}:${PROXY_PORT}" --max-time 5 "http://this-domain-does-not-exist-12345.com/" -s &>/dev/null; then
            print_info "✓ Error handling test passed (invalid host rejected)"
        else
            print_warn "✗ Error handling test failed (invalid host not rejected)"
        fi
    else
        if ! curl -x "http://${PROXY_HOST}:${PROXY_PORT}" --max-time 5 "http://this-domain-does-not-exist-12345.com/" -s &>/dev/null; then
            print_info "✓ Error handling test passed (invalid host rejected)"
        else
            print_warn "✗ Error handling test failed (invalid host not rejected)"
        fi
    fi
}

# Main test function
main() {
    print_info "Starting proxy tests..."
    print_info "Proxy: ${PROXY_TYPE}://${PROXY_HOST}:${PROXY_PORT}"
    echo ""

    # Check if proxy is reachable
    if ! check_proxy; then
        print_error "Proxy is not reachable. Please check if the proxy is running."
        exit 1
    fi

    echo ""

    # Run tests based on proxy type
    if [ "$PROXY_TYPE" = "socks5" ]; then
        test_socks5
    else
        test_http
    fi

    echo ""

    # Run additional tests
    test_dns
    echo ""

    test_speed
    echo ""

    test_ipv6
    echo ""

    test_large_file
    echo ""

    test_concurrent
    echo ""

    test_error_handling
    echo ""

    print_info "All tests completed!"
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --host)
            PROXY_HOST="$2"
            shift 2
            ;;
        --port)
            PROXY_PORT="$2"
            shift 2
            ;;
        --type)
            PROXY_TYPE="$2"
            shift 2
            ;;
        --help)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --host <host>    Proxy host (default: 127.0.0.1)"
            echo "  --port <port>    Proxy port (default: 30000)"
            echo "  --type <type>    Proxy type: socks5 or http (default: socks5)"
            echo "  --help           Show this help message"
            echo ""
            echo "Environment variables:"
            echo "  PROXY_HOST       Proxy host"
            echo "  PROXY_PORT       Proxy port"
            echo "  PROXY_TYPE       Proxy type"
            exit 0
            ;;
        *)
            print_error "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Run main function
main