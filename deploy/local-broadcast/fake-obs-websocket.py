# A fake obs-websocket 5 server, enough of the protocol for the watcher.
#
# WHY THIS EXISTS. -WhatIf never opens a socket, so every -WhatIf case passes
# against a watcher whose obs-websocket code cannot work at all. That is not a
# hypothetical: the first live run of this watcher failed because ConnectAsync
# left a VoidTaskResult on the pipeline and the caller got an array instead of a
# socket. Nothing under -WhatIf could see it. This server makes the real path
# testable with no OBS.
import base64
import hashlib
import json
import os
import socket
import struct
import sys
import threading

PORT = int(sys.argv[1])
PASSWORD = sys.argv[2]
STATE = sys.argv[3]
JOURNAL = sys.argv[4]
GUID = '258EAFA5-E914-47DA-95CA-C5AB0DC85B11'
SALT = 'lM1GncleQOaCu9lT1yeUZhFYnqhsLLP1G5lAGo3ixaI='
CHALLENGE = '+IxH4CnCiqpX1rM9scsNynZzbOe4KhDeYcTNS3PDaeY='


def note(line):
    with open(JOURNAL, 'a', encoding='utf-8') as handle:
        handle.write(line + '\n')


def streaming():
    return open(STATE, encoding='utf-8').read().strip() == 'true'


def set_streaming(value):
    with open(STATE, 'w', encoding='utf-8') as handle:
        handle.write('true' if value else 'false')


def send(conn, payload):
    data = json.dumps(payload).encode()
    header = bytes([0x81])
    length = len(data)
    if length < 126:
        header += bytes([length])
    elif length < (1 << 16):
        header += bytes([126]) + struct.pack('>H', length)
    else:
        header += bytes([127]) + struct.pack('>Q', length)
    conn.sendall(header + data)


def recv(conn):
    head = conn.recv(2)
    if len(head) < 2:
        return None
    opcode = head[0] & 0x0F
    masked = head[1] & 0x80
    length = head[1] & 0x7F
    if length == 126:
        length = struct.unpack('>H', conn.recv(2))[0]
    elif length == 127:
        length = struct.unpack('>Q', conn.recv(8))[0]
    key = conn.recv(4) if masked else b'\x00\x00\x00\x00'
    body = b''
    while len(body) < length:
        chunk = conn.recv(length - len(body))
        if not chunk:
            break
        body += chunk
    if opcode == 0x8:
        return None
    body = bytes(b ^ key[i % 4] for i, b in enumerate(body))
    return json.loads(body.decode())


def serve(conn):
    request = b''
    while b'\r\n\r\n' not in request:
        chunk = conn.recv(4096)
        if not chunk:
            return
        request += chunk
    key = ''
    for line in request.decode('latin-1').split('\r\n'):
        if line.lower().startswith('sec-websocket-key:'):
            key = line.split(':', 1)[1].strip()
    accept = base64.b64encode(hashlib.sha1((key + GUID).encode()).digest()).decode()
    conn.sendall((
        'HTTP/1.1 101 Switching Protocols\r\n'
        'Upgrade: websocket\r\nConnection: Upgrade\r\n'
        f'Sec-WebSocket-Accept: {accept}\r\n'
        'Sec-WebSocket-Protocol: obswebsocket.json\r\n\r\n'
    ).encode())

    send(conn, {'op': 0, 'd': {'obsWebSocketVersion': '5.6.3', 'rpcVersion': 1,
                               'authentication': {'challenge': CHALLENGE, 'salt': SALT}}})
    identify = recv(conn)
    if not identify or identify.get('op') != 1:
        note('BADIDENTIFY')
        return
    secret = base64.b64encode(hashlib.sha256((PASSWORD + SALT).encode()).digest()).decode()
    expected = base64.b64encode(hashlib.sha256((secret + CHALLENGE).encode()).digest()).decode()
    if identify['d'].get('authentication') != expected:
        note('BADAUTH')
        send(conn, {'op': 7, 'd': {'requestStatus': {'result': False, 'code': 4009}}})
        return
    note('IDENTIFIED')
    send(conn, {'op': 2, 'd': {'negotiatedRpcVersion': 1}})

    while True:
        message = recv(conn)
        if message is None:
            return
        if message.get('op') != 6:
            continue
        kind = message['d']['requestType']
        request_id = message['d']['requestId']
        note(kind)
        data = {}
        if kind == 'GetStreamStatus':
            data = {'outputActive': streaming(), 'outputReconnecting': False}
        elif kind == 'StartStream':
            set_streaming(True)
        elif kind == 'StopStream':
            set_streaming(False)
        else:
            send(conn, {'op': 7, 'd': {'requestType': kind, 'requestId': request_id,
                                       'requestStatus': {'result': False, 'code': 204}}})
            continue
        send(conn, {'op': 7, 'd': {'requestType': kind, 'requestId': request_id,
                                   'requestStatus': {'result': True, 'code': 100},
                                   'responseData': data}})


listener = socket.socket()
listener.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
listener.bind(('127.0.0.1', PORT))
listener.listen(8)
sys.stderr.write('ready\n')
sys.stderr.flush()
while True:
    client, _ = listener.accept()
    threading.Thread(target=lambda c=client: (serve(c), c.close()), daemon=True).start()
