---
name: ew-proxy-client
description: Specialized development assistance for the ECH Workers Proxy Client project - a cross-platform network proxy client with ECH encryption support. Use this skill when working on: (1) Cloudflare Worker proxy server code (_worker.js), (2) Go-based proxy server implementation (worker/), (3) ECH (Encrypted Client Hello) configuration and TLS handling, (4) SOCKS5 and HTTP proxy protocol implementation, (5) IP routing and bypass logic (worker/ipLoader.go), (6) Cross-platform system proxy configuration (utils/), (7) WebSocket-based proxy communication, or (8) Docker deployment and soft router integration
---

# ECH Workers Proxy Client Development

## Project Overview

This is a cross-platform network proxy client that routes system network requests through Cloudflare Workers using WebSocket connections with ECH encryption for enhanced privacy.

**Key Components:**
- `other/_worker.js` - Cloudflare Worker proxy server (handles remote connections)
- `worker/` - Go proxy server core (SOCKS5/HTTP protocols)
- `utils/` - Platform-specific utilities (system proxy, ECH, DoH)
- `main.go` - Application entry point with CLI

## Architecture

```
Client Application (Go)
    ↓
SOCKS5/HTTP Proxy Server
    ↓
WebSocket Connection (with ECH)
    ↓
Cloudflare Worker (_worker.js)
    ↓
Target Remote Server
```

## Core Protocols

### SOCKS5 Protocol
- Supported commands: CONNECT (0x01), UDP ASSOCIATE (0x03)
- Address types: IPv4 (0x01), Domain (0x03), IPv6 (0x04)
- No authentication required (method 0x00)

### HTTP Proxy
- HTTP CONNECT for HTTPS tunneling
- GET, POST, PUT, DELETE, HEAD, OPTIONS, PATCH, TRACE for HTTP
- Filters Proxy-Connection and Proxy-Authorization headers

## Development Workflow

### When Modifying _worker.js (Cloudflare Worker)

**Critical Constraints:**
- Must use `cloudflare:sockets` API for TCP connections
- WebSocket only - no HTTP server for proxy requests
- Keep code minimal - Workers have execution limits
- Use fallback IPs for connection reliability

**Common Tasks:**

1. **Add new protocol support:**
   - Parse protocol-specific headers from first frame data
   - Handle protocol-specific response formats
   - Update `parseAddress()` if address format differs

2. **Improve connection handling:**
   - Modify `connectToRemote()` for better retry logic
   - Adjust `CF_FALLBACK_IPS` for alternative routes
   - Tune `CONFIG.CONNECT_TIMEOUT` based on network conditions

3. **Add logging/debugging:**
   - Use `logInfo()`, `logError()`, `logDebug()` functions
   - Set `CONFIG.LOG_LEVEL` to 'debug' for detailed output
   - Monitor via Worker logs dashboard

**Key Functions:**
- `handleSession()` - Main WebSocket session handler
- `connectToRemote()` - Establishes connection to target server
- `pumpRemoteToWebSocket()` - Bidirectional data forwarding
- `parseAddress()` - Parses IPv4/IPv6 addresses with ports

### When Modifying Go Proxy Server (worker/)

**File Structure:**
- `proxyServer.go` - Main proxy server, protocol handlers
- `ipLoader.go` - IP routing and bypass logic
- `ech.go` - ECH configuration and TLS setup
- `const.go` - Constants and mode definitions

**Common Tasks:**

1. **Add new proxy protocol:**
   - Add handler function in `proxyServer.go`
   - Update `handleConnection()` protocol detection switch
   - Implement protocol-specific response functions in `utils/utils.go`
   - Add new mode constant in `worker/const.go`

2. **Modify routing logic:**
   - Update `ipLoader.go` for new IP range formats
   - Modify `ShouldBypassProxy()` decision logic
   - Add new routing modes if needed

3. **Enhance ECH/TLS:**
   - Update `ech.go` for new TLS versions or cipher suites
   - Modify `PrepareECH()` for different ECH query domains
   - Add fallback logic for ECH failures

**Key Functions:**
- `handleSOCKS5()` - SOCKS5 protocol handler
- `handleHTTP()` - HTTP protocol handler (CONNECT + methods)
- `handleTunnel()` - Generic tunnel establishment
- `dialWebSocketWithECH()` - WebSocket connection with ECH

### When Modifying Platform Utilities (utils/)

**Platform-Specific Files:**
- `proxy_windows.go` - Windows system proxy (registry)
- `proxy_darwin.go` - macOS system proxy (networksetup)
- `proxy_linux.go` - Linux system proxy (environment variables)

**Common Tasks:**

1. **Add new platform support:**
   - Create `proxy_<platform>.go` with platform tags
   - Implement `SetSystemProxy()` and `RestoreProxyState()`
   - Test platform-specific proxy format requirements

2. **Update ECH configuration:**
   - Modify `doh.go` for new DoH servers
   - Update `utils.go` TLS configuration functions
   - Add new ECH query domains if needed

**Key Functions:**
- `SetSystemProxy()` - Configure system-wide proxy
- `RestoreProxyState()` - Restore previous proxy settings
- `SaveProxyState()` - Save current proxy state
- `BuildTLSConfigWithECH()` - Create TLS config with ECH

## Configuration

### Client Configuration (main.go)

```go
serverAddr   // Cloudflare Worker address (e.g., worker.workers.dev:443)
listenAddr   // Local proxy listen address (e.g., 0.0.0.0:30000)
token        // Authentication token
serverIP     // Optional: Force specific IP for Worker
dnsServer    // DoH server for ECH queries
echDomain    // ECH query domain
routingMode  // Routing mode: global, bypass_cn, none
```

### Worker Configuration (_worker.js)

```javascript
CONFIG = {
  CONNECT_TIMEOUT: 10000,      // Connection timeout (ms)
  IDLE_TIMEOUT: 300000,        // Idle timeout (ms)
  HEARTBEAT_INTERVAL: 30000,   // Heartbeat interval (ms)
  MAX_BUFFER_SIZE: 1048576,    // Max buffer size (1MB)
  ENABLE_IPV6: true,           // Enable IPv6 support
  ENABLE_HTTP2: true,          // Enable HTTP/2 support
  LOG_LEVEL: 'info'            // Log level: info, debug, error
}
```

## Routing Modes

1. **global** - All traffic goes through proxy
2. **bypass_cn** - Chinese IPs use direct connection, others use proxy
3. **none** - No proxy, all direct connections

**IP List Sources:**
- IPv4: `https://raw.githubusercontent.com/mayaxcn/china-ip-list/master/chn_ip.txt`
- IPv6: `https://raw.githubusercontent.com/mayaxcn/china-ip-list/master/chn_ip_v6.txt`

## Testing

### Test Proxy Connection

```bash
# Test SOCKS5 proxy
curl --socks5 127.0.0.1:30000 http://www.google.com

# Test HTTP proxy
curl -x http://127.0.0.1:30000 http://www.google.com

# Test HTTPS tunnel
curl -x http://127.0.0.1:30000 https://www.google.com
```

### Test Worker Deployment

Deploy `_worker.js` to Cloudflare Workers:
```bash
wrangler deploy other/_worker.js
```

## Build and Deploy

### Build Go Client

```bash
# Build for current platform
go build -o ech-workers main.go

# Build for multiple platforms
GOOS=windows GOARCH=amd64 go build -o ech-workers.exe main.go
GOOS=linux GOARCH=amd64 go build -o ech-workers-linux main.go
GOOS=darwin GOARCH=arm64 go build -o ech-workers-macos main.go
```

### Docker Deployment

```bash
docker build -t ech-workers .
docker run -d --name ech-workers --network host \
  -e ARG_F="worker.workers.dev:443" \
  -e ARG_L="0.0.0.0:30000" \
  -e ARG_TOKEN="your-token" \
  -e ARG_ROUTING="global" \
  ech-workers
```

## Common Issues and Solutions

### Worker Connection Fails

**Symptom:** Client cannot connect to Worker

**Check:**
1. Worker is deployed and accessible
2. Token matches between client and worker
3. ECH configuration is valid
4. Firewall allows WebSocket connections

**Solution:**
- Verify Worker URL with `curl https://worker.workers.dev`
- Check token in `main.go` and `_worker.js`
- Try without ECH first (remove `-dns` and `-ech` parameters)

### ECH Configuration Errors

**Symptom:** "ECH" errors in logs

**Check:**
1. DoH server is accessible
2. ECH domain is valid
3. Go version >= 1.23 (required for ECH)

**Solution:**
- Try different DoH server: `-dns dns.google/dns-query`
- Use default ECH domain: `-ech cloudflare-ech.com`
- Update Go: `go version` (must be 1.23+)

### IP Routing Not Working

**Symptom:** Traffic not bypassing China IPs

**Check:**
1. IP list files exist: `chn_ip.txt`, `chn_ip_v6.txt`
2. Routing mode is `bypass_cn`
3. IP lists are downloaded and valid

**Solution:**
- Manually download IP lists if auto-download fails
- Check IP list file sizes (should be > 0)
- Verify routing mode parameter: `-routing bypass_cn`

### System Proxy Not Setting

**Symptom:** System proxy not configured automatically

**Check:**
1. Platform-specific proxy file exists
2. Running with appropriate permissions
3. Proxy format matches platform requirements

**Solution:**
- Windows: Run as Administrator
- macOS: No special permissions needed
- Linux: Manual proxy configuration required

## Performance Optimization

### Client-Side

1. **Use fixed IP for Worker:** `-ip saas.sin.fan` (bypasses DNS)
2. **Adjust buffer sizes:** Modify `utils/utils.go` buffer sizes
3. **Enable connection pooling:** Reuse WebSocket connections

### Worker-Side

1. **Reduce logging:** Set `LOG_LEVEL: 'error'` in production
2. **Optimize timeouts:** Adjust `CONFIG.CONNECT_TIMEOUT`
3. **Use fallback IPs:** Add more IPs to `CF_FALLBACK_IPS`

## Security Considerations

1. **Token Authentication:** Always use unique tokens
2. **ECH Encryption:** Enable ECH for privacy protection
3. **TLS 1.3 Only:** Minimum TLS version is 1.3
4. **No Logs in Production:** Disable sensitive logging
5. **Rate Limiting:** Consider adding rate limiting to Worker

## References

For detailed protocol specifications and implementation details, see:
- [references/protocols.md](references/protocols.md) - SOCKS5 and HTTP proxy protocols
- [references/ech.md](references/ech.md) - ECH implementation details
- [references/architecture.md](references/architecture.md) - Complete system architecture