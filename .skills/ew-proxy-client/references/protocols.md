# Proxy Protocol Specifications

## SOCKS5 Protocol

### Connection Flow

```
Client → Server: Version + Methods
Server → Client: Version + Selected Method
Client → Server: Version + Command + Address Type + Address + Port
Server → Client: Version + Reply + Reserved + Address Type + Address + Port
```

### Message Formats

#### Initial Greeting

```
+----+----------+----------+
|VER | NMETHODS | METHODS  |
+----+----------+----------+
| 1  |    1     | 1 to 255 |
+----+----------+----------+
```

- **VER**: Protocol version (0x05 for SOCKS5)
- **NMETHODS**: Number of authentication methods
- **METHODS**: List of supported methods (0x00 = no auth)

#### Server Response

```
+----+--------+
|VER | METHOD |
+----+--------+
| 1  |   1    |
+----+--------+
```

#### Connection Request

```
+----+-----+-------+------+----------+----------+
|VER | CMD |  RSV  | ATYP | DST.ADDR | DST.PORT |
+----+-----+-------+------+----------+----------+
| 1  |  1  | X'00' |  1   | Variable |    2     |
+----+-----+-------+------+----------+----------+
```

- **CMD**: Command (0x01 = CONNECT, 0x03 = UDP ASSOCIATE)
- **ATYP**: Address type (0x01 = IPv4, 0x03 = Domain, 0x04 = IPv6)

#### Server Response

```
+----+-----+-------+------+----------+----------+
|VER | REP |  RSV  | ATYP | BND.ADDR | BND.PORT |
+----+-----+-------+------+----------+----------+
| 1  |  1  | X'00' |  1   | Variable |    2     |
+----+-----+-------+------+----------+----------+
```

- **REP**: Reply code (0x00 = success, 0x04 = host unreachable, 0x05 = connection refused)

### UDP ASSOCIATE

#### UDP Request Format

```
+----+------+------+----------+----------+----------+
|RSV | FRAG | ATYP | DST.ADDR | DST.PORT |   DATA   |
+----+------+------+----------+----------+----------+
| 2  |  1   |  1   | Variable |    2     | Variable |
+----+------+------+----------+----------+----------+
```

- **RSV**: Reserved (0x0000)
- **FRAG**: Fragment number (0x00 = first/only fragment)
- **ATYP**: Address type (0x01 = IPv4, 0x03 = Domain, 0x04 = IPv6)

## HTTP Proxy Protocol

### HTTP CONNECT

Used for HTTPS tunneling.

#### Request Format

```
CONNECT server:port HTTP/1.1\r\n
Host: server:port\r\n
[Other headers]\r\n
\r\n
```

#### Response Format

```
HTTP/1.1 200 Connection Established\r\n
[Other headers]\r\n
\r\n
```

After successful response, the connection becomes a transparent tunnel.

### HTTP Methods

For HTTP requests, the proxy forwards requests with modified headers.

#### Request Format

```
METHOD http://server:port/path HTTP/1.1\r\n
Host: server:port\r\n
[Other headers]\r\n
\r\n
[Request body if present]
```

#### Headers to Filter

- `Proxy-Connection` - Should be removed or converted to `Connection`
- `Proxy-Authorization` - Should be removed (proxy authentication not supported)

## WebSocket Protocol

### Handshake

```
Client → Server:
GET /path HTTP/1.1\r\n
Host: server:port\r\n
Upgrade: websocket\r\n
Connection: Upgrade\r\n
Sec-WebSocket-Key: <base64>\r\n
Sec-WebSocket-Version: 13\r\n
Sec-WebSocket-Protocol: <token>\r\n
\r\n

Server → Client:
HTTP/1.1 101 Switching Protocols\r\n
Upgrade: websocket\r\n
Connection: Upgrade\r\n
Sec-WebSocket-Accept: <base64>\r\n
Sec-WebSocket-Protocol: <token>\r\n
\r\n
```

### Frame Format

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-------+-+-------------+-------------------------------+
|F|R|R|R| opcode|M| Payload len |    Extended payload length    |
|I|S|S|S|  (4)  |A|     (7)     |             (16/64)           |
|N|V|V|V|       |S|             |   (if payload len==126/127)   |
| |1|2|3|       |K|             |                               |
+-+-+-+-+-------+-+-------------+ - - - - - - - - - - - - - - - +
|     Extended payload length continued, if payload len == 127  |
+ - - - - - - - - - - - - - - - +-------------------------------+
|                               |Masking-key, if MASK set to 1  |
+-------------------------------+-------------------------------+
| Masking-key (continued)       |          Payload Data         |
+-------------------------------- - - - - - - - - - - - - - - - +
:                     Payload Data continued ...                :
+ - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - +
|                     Payload Data continued ...                |
+---------------------------------------------------------------+
```

### Opcodes

- **0x0**: Continuation frame
- **0x1**: Text frame
- **0x2**: Binary frame
- **0x8**: Close frame
- **0x9**: Ping frame
- **0xA**: Pong frame

## Custom Protocol Messages

### CONNECT Message

```
CONNECT:<address>|<firstFrameData>
```

- **address**: Target server address (e.g., `example.com:443` or `[::1]:443`)
- **firstFrameData**: Optional initial data to send after connection

### DATA Message

```
DATA:<base64-encoded-data>
```

- **base64-encoded-data**: Data to forward to remote server

### Control Messages

- `CONNECTED` - Connection established successfully
- `CLOSE` - Close connection
- `ERROR:<message>` - Error occurred
- `PING` - Heartbeat ping
- `PONG` - Heartbeat pong

## Address Parsing

### IPv4 Format

```
host:port
```

Example: `192.168.1.1:8080`

### IPv6 Format

```
[host]:port
```

Example: `[2001:db8::1]:8080`

### Domain Format

```
domain:port
```

Example: `example.com:443`

## Error Handling

### Common Error Codes

**SOCKS5:**
- 0x01: General SOCKS server failure
- 0x02: Connection not allowed by ruleset
- 0x03: Network unreachable
- 0x04: Host unreachable
- 0x05: Connection refused
- 0x06: TTL expired
- 0x07: Command not supported
- 0x08: Address type not supported

**HTTP:**
- 400: Bad Request
- 401: Unauthorized
- 403: Forbidden
- 404: Not Found
- 407: Proxy Authentication Required
- 502: Bad Gateway
- 503: Service Unavailable