# System Architecture

## High-Level Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     Client Application                       │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │   SOCKS5     │  │   HTTP       │  │  System      │      │
│  │   Handler    │  │   Handler    │  │  Proxy       │      │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘      │
│         │                 │                 │               │
│         └─────────────────┴─────────────────┘               │
│                           │                                 │
│                    ┌──────▼──────┐                          │
│                    │  Proxy Core │                          │
│                    │  (Go)       │                          │
│                    └──────┬──────┘                          │
└───────────────────────────┼─────────────────────────────────┘
                            │
                    ┌───────▼────────┐
                    │  IP Routing    │
                    │  (Bypass CN)   │
                    └───────┬────────┘
                            │
                    ┌───────▼────────┐
                    │  ECH/TLS       │
                    │  Configuration │
                    └───────┬────────┘
                            │
                    ┌───────▼────────┐
                    │  WebSocket     │
                    │  Client        │
                    └───────┬────────┘
                            │
                    ┌───────▼────────┐
                    │  Internet      │
                    │  (TLS 1.3)     │
                    └───────┬────────┘
                            │
                    ┌───────▼────────┐
                    │  Cloudflare    │
                    │  Worker        │
                    │  (_worker.js)  │
                    └───────┬────────┘
                            │
                    ┌───────▼────────┐
                    │  Target Server │
                    └────────────────┘
```

## Component Details

### 1. Client Application (main.go)

**Responsibilities:**
- Parse command-line arguments
- Initialize components
- Start proxy server
- Handle system signals

**Key Functions:**
```go
func main() {
    // Parse flags
    // Set system proxy
    // Setup safe exit
    // Run proxy server
}

func run() {
    // Create proxy server config
    // Initialize IP loader
    // Initialize ECH
    // Start proxy server
}
```

### 2. Proxy Server (worker/proxyServer.go)

**Responsibilities:**
- Listen for client connections
- Detect protocol type (SOCKS5/HTTP)
- Handle protocol-specific logic
- Establish tunnel to remote server
- Bidirectional data forwarding

**Key Components:**

#### Connection Handler
```go
func (p *ProxyServer) handleConnection(conn net.Conn) {
    // Read first byte
    // Detect protocol (SOCKS5 vs HTTP)
    // Route to appropriate handler
}
```

#### SOCKS5 Handler
```go
func (p *ProxyServer) handleSOCKS5(conn, clientAddr, firstByte) {
    // Version negotiation
    // Authentication (no auth)
    // Parse CONNECT/UDP ASSOCIATE request
    // Establish tunnel
}
```

#### HTTP Handler
```go
func (p *ProxyServer) handleHTTP(conn, clientAddr, firstByte) {
    // Parse HTTP request
    // Handle CONNECT (HTTPS tunneling)
    // Handle GET/POST/etc (HTTP proxy)
    // Establish tunnel
}
```

#### Tunnel Handler
```go
func (p *ProxyServer) handleTunnel(conn, target, clientAddr, mode, firstFrame) {
    // Check IP routing
    // Establish WebSocket connection
    // Send CONNECT message
    // Wait for CONNECTED response
    // Bidirectional forwarding
}
```

### 3. IP Routing (worker/ipLoader.go)

**Responsibilities:**
- Load Chinese IP lists (IPv4/IPv6)
- Determine if IP should bypass proxy
- Efficient IP range matching

**Key Functions:**
```go
type IPLoader struct {
    ipv4Ranges []IPRange
    ipv6Ranges []IPv6Range
    mode       string
}

func (l *IPLoader) ShouldBypassProxy(host string) bool {
    // Resolve domain to IP
    // Check if IP in Chinese ranges
    // Return true if should bypass
}
```

**IP List Sources:**
- IPv4: `https://raw.githubusercontent.com/mayaxcn/china-ip-list/master/chn_ip.txt`
- IPv6: `https://raw.githubusercontent.com/mayaxcn/china-ip-list/master/chn_ip_v6.txt`

### 4. ECH Configuration (worker/ech.go)

**Responsibilities:**
- Download ECH config via DoH
- Build TLS config with ECH
- Refresh ECH config periodically

**Key Functions:**
```go
type Ech struct {
    dnsServer string
    echDomain string
    echList   []byte
    tlsConfig *tls.Config
}

func (e *Ech) PrepareECH() error {
    // Download ECH config
    // Build TLS config
}

func (e *Ech) GetECHList() ([]byte, error) {
    // Return cached ECH config
}

func (e *Ech) BuildTLSConfigWithECH(addr, echBytes) (*tls.Config, error) {
    // Create TLS 1.3 config
    // Add ECH config
}
```

### 5. Platform Utilities (utils/)

**Responsibilities:**
- Platform-specific system proxy configuration
- TLS configuration helpers
- Connection utilities

**Platform Files:**
- `proxy_windows.go` - Windows (registry)
- `proxy_darwin.go` - macOS (networksetup)
- `proxy_linux.go` - Linux (environment variables)

**Key Functions:**
```go
func SetSystemProxy(enabled bool, listenAddr, routingMode) error {
    // Set system-wide proxy
}

func RestoreProxyState() error {
    // Restore previous proxy settings
}

func BuildTLSConfigWithECH(addr, echBytes) (*tls.Config, error) {
    // Create TLS config with ECH
}
```

### 6. Cloudflare Worker (other/_worker.js)

**Responsibilities:**
- Accept WebSocket connections
- Parse CONNECT/DATA messages
- Establish connection to target server
- Bidirectional data forwarding
- Handle fallback IPs

**Key Components:**

#### Session Handler
```javascript
async function handleSession(webSocket) {
    // Setup cleanup handlers
    // Handle CONNECT requests
    // Handle DATA messages
    // Bidirectional forwarding
    // Heartbeat mechanism
}
```

#### Connection Handler
```javascript
const connectToRemote = async (targetAddr, firstFrameData) => {
    // Parse address
    // Try connection with fallback IPs
    // Send first frame
    // Start data forwarding
};
```

#### Data Pumping
```javascript
const pumpRemoteToWebSocket = async () => {
    // Read from remote socket
    // Send to WebSocket
    // Handle errors
};
```

## Data Flow

### SOCKS5 CONNECT Flow

```
1. Client → Proxy: SOCKS5 CONNECT request
2. Proxy → Client: SOCKS5 success response
3. Proxy → Worker: WebSocket CONNECT message
4. Worker → Target: TCP connection
5. Worker → Proxy: WebSocket CONNECTED message
6. Client ↔ Proxy ↔ Worker ↔ Target: Data transfer
7. Client → Proxy: Close connection
8. Proxy → Worker: WebSocket CLOSE message
9. Worker → Target: Close connection
```

### HTTP CONNECT Flow

```
1. Client → Proxy: HTTP CONNECT request
2. Proxy → Client: HTTP 200 Connection Established
3. Proxy → Worker: WebSocket CONNECT message
4. Worker → Target: TCP connection
5. Worker → Proxy: WebSocket CONNECTED message
6. Client ↔ Proxy ↔ Worker ↔ Target: Data transfer
7. Client → Proxy: Close connection
8. Proxy → Worker: WebSocket CLOSE message
9. Worker → Target: Close connection
```

### HTTP GET Flow

```
1. Client → Proxy: HTTP GET request
2. Proxy → Worker: WebSocket CONNECT message with first frame
3. Worker → Target: TCP connection
4. Worker → Target: Send HTTP GET request
5. Worker → Proxy: WebSocket CONNECTED message
6. Target → Worker: HTTP response
7. Worker → Proxy: WebSocket DATA message
8. Proxy → Client: HTTP response
9. Client → Proxy: Close connection
10. Proxy → Worker: WebSocket CLOSE message
11. Worker → Target: Close connection
```

## Protocol Layers

### Layer 1: Transport Layer
- **Protocol**: TCP
- **Purpose**: Reliable data delivery
- **Ports**: Configurable (default: 30000)

### Layer 2: Proxy Protocol Layer
- **Protocols**: SOCKS5, HTTP
- **Purpose**: Proxy client communication
- **Features**: Authentication (optional), IPv4/IPv6 support

### Layer 3: WebSocket Layer
- **Protocol**: WebSocket (RFC 6455)
- **Purpose**: Tunnel through Cloudflare Workers
- **Features**: Binary/text frames, ping/pong

### Layer 4: TLS Layer
- **Protocol**: TLS 1.3
- **Purpose**: Encryption and authentication
- **Features**: ECH (Encrypted Client Hello)

### Layer 5: Application Layer
- **Protocol**: Custom proxy protocol
- **Purpose**: Control messages and data forwarding
- **Messages**: CONNECT, DATA, CLOSE, PING, PONG

## Security Architecture

### Encryption Layers

1. **WebSocket → Worker**: TLS 1.3 with ECH
2. **Worker → Target**: TLS (if target uses HTTPS)
3. **SNI Protection**: ECH encrypts SNI in TLS handshake

### Authentication

1. **Token-based**: WebSocket subprotocol token
2. **Worker-level**: Token validation in `_worker.js`
3. **No client auth**: SOCKS5 no-auth mode (0x00)

### Privacy Features

1. **ECH**: Encrypts SNI
2. **DoH**: Encrypts DNS queries
3. **TLS 1.3**: Modern encryption
4. **IP Bypass**: Chinese IPs don't leave country

## Performance Architecture

### Connection Pooling

- **WebSocket Reuse**: Single WebSocket per client connection
- **Keep-Alive**: Periodic ping/pong messages
- **Timeout**: Idle connections closed after 5 minutes

### Buffer Management

- **Client → Worker**: 32KB buffers
- **Worker → Target**: Stream-based (no buffering)
- **Max Buffer Size**: 1MB (Worker side)

### Concurrency Model

- **Go**: Goroutines per connection
- **Worker**: Async/await per session
- **Concurrency**: Limited by system resources

## Error Handling

### Client-Side Errors

1. **Connection Failed**: Retry with fallback IPs
2. **ECH Failed**: Refresh ECH config and retry
3. **Timeout**: Close connection gracefully
4. **Protocol Error**: Send error response to client

### Worker-Side Errors

1. **Connection Failed**: Try fallback IPs
2. **Read/Write Error**: Close connection
3. **Timeout**: Close connection
4. **Invalid Message**: Send ERROR message

### Error Recovery

1. **Automatic Retry**: Up to 3 attempts
2. **Fallback**: Disable ECH if it fails
3. **Graceful Close**: Send CLOSE message before closing
4. **Logging**: All errors logged for debugging

## Deployment Architecture

### Client Deployment

**Platforms:**
- Windows (amd64)
- macOS (amd64, arm64)
- Linux (amd64, arm64)

**Deployment Modes:**
- Desktop application (with GUI)
- Command-line tool
- Soft router service
- Docker container

### Worker Deployment

**Platform:**
- Cloudflare Workers

**Configuration:**
- Worker script: `_worker.js`
- Custom domain: Optional
- Token: Required for authentication

**Scaling:**
- Auto-scaling by Cloudflare
- Edge locations worldwide
- No infrastructure management

## Monitoring and Logging

### Client Logging

**Log Levels:**
- `[启动]`: Startup information
- `[代理]`: Proxy operations
- `[分流]`: Routing decisions
- `[ECH]`: ECH operations
- `[HTTP-*]`: HTTP requests
- `[SOCKS5]`: SOCKS5 requests
- `[UDP]`: UDP operations

**Log Output:**
- Console (default)
- File (redirected)
- System journal (systemd)

### Worker Logging

**Log Levels:**
- `[INFO]`: General information
- `[ERROR]`: Error messages
- `[DEBUG]`: Debug information (optional)

**Log Access:**
- Cloudflare Workers dashboard
- Real-time logs
- Historical logs (24 hours)

## Configuration Architecture

### Client Configuration

**Sources:**
1. Command-line arguments
2. Configuration file (JSON)
3. Environment variables

**Priority:**
1. Command-line (highest)
2. Configuration file
3. Environment variables (lowest)

### Worker Configuration

**Sources:**
1. Hardcoded in `_worker.js`
2. Environment variables (Cloudflare)
3. Secrets (Cloudflare)

**Configuration Items:**
- Token
- Fallback IPs
- Timeout values
- Log level

## Extension Points

### Adding New Proxy Protocols

1. Create handler in `worker/proxyServer.go`
2. Add protocol detection logic
3. Implement protocol-specific response
4. Update `utils/utils.go` for response helpers

### Adding New Platforms

1. Create `utils/proxy_<platform>.go`
2. Implement `SetSystemProxy()` and `RestoreProxyState()`
3. Add platform build tags
4. Test platform-specific behavior

### Adding New Routing Modes

1. Update `worker/ipLoader.go`
2. Add new routing logic
3. Add mode constant in `worker/const.go`
4. Update documentation

### Adding New DoH Servers

1. Update `worker/ech.go`
2. Add DoH server URL
3. Test DoH query format
4. Update documentation

## Testing Architecture

### Unit Tests

**Components:**
- IP range matching
- Address parsing
- Protocol detection
- ECH config parsing

### Integration Tests

**Scenarios:**
- SOCKS5 CONNECT
- HTTP CONNECT
- HTTP GET/POST
- IP routing
- ECH fallback

### End-to-End Tests

**Test Matrix:**
- Multiple platforms
- Multiple protocols
- Multiple routing modes
- Error scenarios

## Performance Optimization

### Client-Side Optimizations

1. **Connection Reuse**: Keep WebSocket connections alive
2. **Buffer Tuning**: Optimize buffer sizes
3. **IP Caching**: Cache DNS resolutions
4. **ECH Config Caching**: Cache ECH config

### Worker-Side Optimizations

1. **Minimal Logging**: Reduce log output
2. **Stream Processing**: Avoid buffering
3. **Connection Pooling**: Reuse connections
4. **Edge Caching**: Leverage Cloudflare cache

## Future Enhancements

### Planned Features

1. **UDP Proxy**: Full UDP support (currently DNS only)
2. **HTTP/2**: HTTP/2 proxy support
3. **QUIC**: QUIC protocol support
4. **Load Balancing**: Multiple Workers with load balancing
5. **Metrics**: Connection and performance metrics
6. **Web UI**: Web-based management interface

### Research Areas

1. **DNSSEC**: DNSSEC validation
2. **mTLS**: Mutual TLS authentication
3. **Zero Trust**: Zero Trust architecture
4. **Edge Functions**: Cloudflare Workers integration
5. **WASM**: WebAssembly for protocol parsing