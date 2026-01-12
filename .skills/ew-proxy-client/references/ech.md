# ECH (Encrypted Client Hello) Implementation

## Overview

ECH (Encrypted Client Hello) is a TLS 1.3 extension that encrypts the Server Name Indication (SNI) field in the TLS handshake, providing enhanced privacy by preventing network observers from seeing which website is being accessed.

## How ECH Works

### Traditional TLS Handshake

```
Client → Server: ClientHello (includes SNI in plaintext)
Server → Client: ServerHello, Certificate, ServerKeyExchange
Client → Server: ClientKeyExchange, ChangeCipherSpec, Finished
Server → Client: ChangeCipherSpec, Finished
```

**Problem:** SNI is visible to network observers.

### ECH-Enabled TLS Handshake

```
Client → Server: ClientHello (SNI encrypted in ECH extension)
Server → Client: ServerHello (with ECH acceptance)
Client → Server: ClientKeyExchange, ChangeCipherSpec, Finished
Server → Client: ChangeCipherSpec, Finished
```

**Benefit:** SNI is encrypted and protected from observation.

## ECH Components

### ECH Config

The ECH configuration contains:
- **Public Key**: Used to encrypt the ClientHello
- **Cipher Suites**: Supported encryption algorithms
- **Max Name Length**: Maximum length for encrypted SNI
- **Extensions**: Additional ECH parameters

### DoH Query

The ECH config is obtained via DNS over HTTPS (DoH) query to a specific domain.

```
GET https://<ech-domain>/dns-query?name=<server-name>&type=HTTPS
```

## Implementation in ew-proxy-client

### ECH Configuration (worker/ech.go)

```go
type Ech struct {
    dnsServer string
    echDomain string
    echList   []byte
    tlsConfig *tls.Config
}

func NewEch(dnsServer, echDomain string) *Ech {
    return &Ech{
        dnsServer: dnsServer,
        echDomain: echDomain,
    }
}
```

### ECH Preparation

```go
func (e *Ech) PrepareECH() error {
    // 1. Download ECH config via DoH
    e.echList = e.downloadECHConfig()

    // 2. Build TLS config with ECH
    e.tlsConfig = e.BuildTLSConfigWithECH(e.echList)

    return nil
}
```

### DoH Query for ECH Config

```go
func (e *Ech) downloadECHConfig() ([]byte, error) {
    // Query DoH server for HTTPS records
    // Parse response to extract ECH config
    // Return ECH config bytes
}
```

### TLS Config with ECH

```go
func (e *Ech) BuildTLSConfigWithECH(echBytes []byte) (*tls.Config, error) {
    config := &tls.Config{
        MinVersion: tls.VersionTLS13,
        ServerName: host,
    }

    // Add ECH config
    // Go 1.23+ supports ECH via crypto/tls

    return config, nil
}
```

## DoH Servers

### Common DoH Servers

1. **Cloudflare**
   - URL: `https://cloudflare-dns.com/dns-query`
   - Supports: DNSSEC, IPv6

2. **Google**
   - URL: `https://dns.google/dns-query`
   - Supports: DNSSEC, IPv6

3. **AliDNS** (Recommended for China)
   - URL: `https://dns.alidns.com/dns-query`
   - Supports: IPv6

4. **Quad9**
   - URL: `https://dns.quad9.net/dns-query`
   - Supports: Security filtering

### ECH Query Domains

1. **cloudflare-ech.com** (Default)
   - Cloudflare's official ECH testing domain
   - Widely supported

2. **Other domains**
   - Can be configured via `-ech` parameter
   - Must support HTTPS DNS records

## ECH in Cloudflare Worker

### Worker Limitations

Cloudflare Workers have limitations for ECH:
- Cannot directly use ECH in outbound connections
- Must rely on Cloudflare's internal ECH support
- Fallback to plaintext SNI if ECH fails

### Worker Implementation

```javascript
// In _worker.js
remoteSocket = connect({
  hostname: connectHost,
  port: port
});

// Cloudflare Workers automatically handle ECH
// if the target server supports it
```

## ECH Fallback Strategy

### Client-Side Fallback

```go
func (p *ProxyServer) dialWebSocketWithECH(maxRetries int) (*WebSocketWrap, error) {
    for attempt := 1; attempt <= maxRetries; attempt++ {
        echBytes, err := p.Ech.GetECHList()
        if err != nil {
            // Refresh ECH config
            p.Ech.RefreshECH()
            continue
        }

        tlsCfg := p.Ech.BuildTLSConfigWithECH(p.Config.ServerAddr, echBytes)

        // Try connection with ECH
        wsConn, err := dialer.Dial(wsURL, nil)
        if err != nil {
            // Fallback without ECH
            if strings.Contains(err.Error(), "ECH") {
                log.Println("ECH failed, retrying...")
                continue
            }
        }

        return wsConn, nil
    }

    // Last attempt without ECH
    return p.dialWithoutECH()
}
```

### Worker-Side Fallback

```javascript
const connectToRemote = async (targetAddr, firstFrameData) => {
  const attempts = [null, ...CF_FALLBACK_IPS];

  for (let i = 0; i < attempts.length; i++) {
    try {
      remoteSocket = connect({
        hostname: attempts[i] || host,
        port: port
      });

      // Cloudflare handles ECH automatically
      // if ECH fails, connection falls back to plaintext

      return;
    } catch (err) {
      // Try next IP
    }
  }
};
```

## ECH Verification

### Test ECH Support

```bash
# Check if server supports ECH
curl -v --ech https://example.com

# Use openssl to check ECH
openssl s_client -connect example.com:443 -ech
```

### Verify ECH in Logs

Enable debug logging to see ECH status:

```go
// In main.go
log.SetLevel("debug")
```

Look for:
- `ECH config downloaded`
- `TLS config with ECH created`
- `Connection established with ECH`

## Common ECH Issues

### ECH Config Download Failed

**Symptoms:**
- `Failed to download ECH config`
- `DoH query timeout`

**Solutions:**
1. Check DoH server accessibility
2. Try different DoH server
3. Check network connectivity
4. Verify ECH domain is valid

### ECH Not Supported by Server

**Symptoms:**
- `ECH not supported by server`
- Connection falls back to plaintext

**Solutions:**
1. Verify target server supports ECH
2. Check TLS 1.3 is enabled
3. Update Go to 1.23+ (required for ECH)
4. Use Cloudflare Workers (they support ECH)

### ECH Handshake Failure

**Symptoms:**
- `ECH handshake failed`
- `Invalid ECH config`

**Solutions:**
1. Refresh ECH config
2. Check ECH config expiration
3. Verify ECH public key
4. Try different ECH domain

## ECH Best Practices

1. **Always Use ECH**: Enable ECH for all connections when possible
2. **Multiple DoH Servers**: Configure fallback DoH servers
3. **Refresh ECH Config**: Periodically refresh ECH configuration
4. **Monitor ECH Status**: Log ECH success/failure rates
5. **Fallback Gracefully**: Always have fallback without ECH

## ECH and DNS

### DNS over HTTPS (DoH)

ECH config is retrieved via DoH, which also provides:
- Encrypted DNS queries
- Protection against DNS spoofing
- Privacy for DNS lookups

### DNS over TLS (DoT)

Alternative to DoH, but not used in this project:
- Uses TCP port 853
- Similar privacy benefits
- Less widely supported than DoH

## ECH Standards

### RFC 9220

"Exported Authenticators for TLS 1.3"

### RFC 9250

"TLS Encrypted Client Hello"

### Cloudflare ECH

Cloudflare's ECH implementation:
- Supports all major browsers
- Automatic fallback
- Wide server support

## Testing ECH

### Local Testing

```bash
# Test with curl
curl --ech https://example.com

# Test with openssl
openssl s_client -connect example.com:443 -ech -servername example.com
```

### Online Testing

- https://www.cloudflare.com/ssl/encrypted-sni/
- https://crypto.stanford.edu/ech/

## ECH and Privacy

### What ECH Protects

- **SNI**: Server name in TLS handshake
- **Domain Name**: Which website is being accessed
- **Privacy**: Prevents ISP/network from seeing visited sites

### What ECH Doesn't Protect

- **IP Address**: Destination IP is still visible
- **Traffic Patterns**: Timing and size of requests
- **DNS Queries**: Unless using DoH/DoT
- **Application Data**: Content is encrypted by TLS anyway

### Complete Privacy Stack

For maximum privacy, combine:
1. **ECH**: Encrypt SNI
2. **DoH**: Encrypt DNS queries
3. **TLS 1.3**: Encrypt all traffic
4. **Proxy/VPN**: Hide IP address

## ECH Performance Impact

### Overhead

- **Additional Round Trip**: ECH adds 1-2 RTT to handshake
- **CPU Usage**: Slight increase for encryption
- **Memory**: Minimal additional memory usage

### Optimization

1. **Reuse ECH Config**: Cache ECH config locally
2. **Parallel Connections**: Use connection pooling
3. **Early Fallback**: Fail fast if ECH not supported

## ECH Compliance

### Regulatory Compliance

ECH may be subject to:
- Local laws and regulations
- Corporate policies
- Export controls

### Best Practices

1. **Comply with Laws**: Follow local regulations
2. **User Consent**: Inform users about ECH usage
3. **Transparency**: Document ECH implementation
4. **Audit**: Regular security audits

## References

- RFC 9220: Exported Authenticators for TLS 1.3
- RFC 9250: TLS Encrypted Client Hello
- Cloudflare ECH: https://blog.cloudflare.com/encrypted-client-hello/
- SSL Labs ECH Test: https://www.ssllabs.com/ssltest/