const WS_READY_STATE_OPEN = 1;
const WS_READY_STATE_CLOSING = 2;
const CF_FALLBACK_IPS = ['ProxyIP.CMLiussss.net'];// cm维护

// 复用 TextEncoder，避免重复创建
const encoder = new TextEncoder();

import { connect } from 'cloudflare:sockets';

export default {
  async fetch(request, env, ctx) {
    try {
      const token = 'jmrx';
      const upgradeHeader = request.headers.get('Upgrade');

      if (!upgradeHeader || upgradeHeader.toLowerCase() !== 'websocket') {
        return new URL(request.url).pathname === '/'
          ? new Response('WebSocket Proxy Server', { status: 200 })
          : new Response('Expected WebSocket', { status: 426 });
      }

      if (token && request.headers.get('Sec-WebSocket-Protocol') !== token) {
        return new Response('Unauthorized', { status: 401 });
      }

      const [client, server] = Object.values(new WebSocketPair());
      server.accept();

      handleSession(server).catch(() => safeCloseWebSocket(server));

      const responseInit = {
        status: 101,
        webSocket: client
      };

      if (token) {
        responseInit.headers = { 'Sec-WebSocket-Protocol': token };
      }

      return new Response(null, responseInit);

    } catch (err) {
      return new Response(err.toString(), { status: 500 });
    }
  },
};

async function handleSession(webSocket) {
  let remoteSocket, remoteWriter, remoteReader;
  let isClosed = false;

  const cleanup = () => {
    if (isClosed) return;
    isClosed = true;

    try { remoteWriter?.releaseLock(); } catch { }
    try { remoteReader?.releaseLock(); } catch { }
    try { remoteSocket?.close(); } catch { }

    remoteWriter = remoteReader = remoteSocket = null;
    safeCloseWebSocket(webSocket);
  };

  const pumpRemoteToWebSocket = async () => {
    try {
      while (!isClosed && remoteReader) {
        const { done, value } = await remoteReader.read();

        if (done) break;
        if (webSocket.readyState !== WS_READY_STATE_OPEN) break;
        if (value?.byteLength > 0) webSocket.send(value);
      }
    } catch { }

    if (!isClosed) {
      try { webSocket.send('CLOSE'); } catch { }
      cleanup();
    }
  };

  const parseAddress = (addr) => {
    if (addr[0] === '[') {
      const end = addr.indexOf(']');
      return {
        host: addr.substring(1, end),
        port: parseInt(addr.substring(end + 2), 10)
      };
    }
    const sep = addr.lastIndexOf(':');
    return {
      host: addr.substring(0, sep),
      port: parseInt(addr.substring(sep + 1), 10)
    };
  };

  const isCFError = (err) => {
    const msg = err?.message?.toLowerCase() || '';
    return msg.includes('proxy request') ||
      msg.includes('cannot connect') ||
      msg.includes('cloudflare');
  };

  const connectToRemote = async (targetAddr, firstFrameData) => {
    const { host, port } = parseAddress(targetAddr);

    // 使用 connect API
    const attempts = [null, ...CF_FALLBACK_IPS];

    for (let i = 0; i < attempts.length; i++) {
      try {
        // Cloudflare Workers 的 connect 需要使用 IP 地址
        // 如果是域名，需要先解析
        let connectHost = attempts[i] || host;

        remoteSocket = connect({
          hostname: connectHost,
          port
        });

        if (remoteSocket.opened) await remoteSocket.opened;

        remoteWriter = remoteSocket.writable.getWriter();
        remoteReader = remoteSocket.readable.getReader();

        // 发送首帧数据（如果有）
        if (firstFrameData && firstFrameData.length > 0) {
          // 检查是否是 Base64 编码（SOCKS5 二进制数据）
          let dataToSend = firstFrameData;
          if (firstFrameData.startsWith('base64:')) {
            // Base64 解码
            dataToSend = atob(firstFrameData.substring(7));
          }
          await remoteWriter.write(encoder.encode(dataToSend));
        }

        webSocket.send('CONNECTED');
        pumpRemoteToWebSocket();
        return;

      } catch (err) {
        // 清理失败的连接
        try { remoteWriter?.releaseLock(); } catch { }
        try { remoteReader?.releaseLock(); } catch { }
        try { remoteSocket?.close(); } catch { }
        remoteWriter = remoteReader = remoteSocket = null;

        // 如果不是 CF 错误或已是最后尝试，抛出错误
        if (!isCFError(err) || i === attempts.length - 1) {
          throw err;
        }
      }
    }
  };

  webSocket.addEventListener('message', async (event) => {
    if (isClosed) return;

    try {
      const data = event.data;

      if (typeof data === 'string') {
        if (data.startsWith('CONNECT:')) {
          const sep = data.indexOf('|', 8);
          await connectToRemote(
            data.substring(8, sep),
            data.substring(sep + 1)
          );
        }
        else if (data.startsWith('DATA:')) {
          if (remoteWriter) {
            await remoteWriter.write(encoder.encode(data.substring(5)));
          }
        }
        else if (data === 'CLOSE') {
          cleanup();
        }
      }
      else if (data instanceof ArrayBuffer && remoteWriter) {
        await remoteWriter.write(new Uint8Array(data));
      }
    } catch (err) {
      try { webSocket.send('ERROR:' + err.message); } catch { }
      cleanup();
    }
  });

  webSocket.addEventListener('close', cleanup);
  webSocket.addEventListener('error', cleanup);
}

function safeCloseWebSocket(ws) {
  try {
    if (ws.readyState === WS_READY_STATE_OPEN ||
      ws.readyState === WS_READY_STATE_CLOSING) {
      ws.close(1000, 'Server closed');
    }
  } catch { }
}