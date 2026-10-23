#!/usr/bin/env python3
"""Loopback-only artifact/task bridge for an isolated UTM test guest."""
import argparse
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
import time


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('directory', type=Path, help='Contains artifacts/ and logs/')
    args = parser.parse_args()
    artifacts = args.directory.resolve() / 'artifacts'
    logs = args.directory.resolve() / 'logs'
    artifacts.mkdir(parents=True, exist_ok=True)
    logs.mkdir(parents=True, exist_ok=True)

    class Handler(SimpleHTTPRequestHandler):
        def __init__(self, *arguments, **kwargs):
            super().__init__(*arguments, directory=str(artifacts), **kwargs)

        def log_message(self, *_):
            # A long test must never block on an undrained terminal output pipe.
            pass

        def do_POST(self):
            if self.path != '/results':
                self.send_error(404)
                return
            length = int(self.headers.get('Content-Length', '0'))
            if not 0 < length <= 10 * 1024 * 1024:
                self.send_error(413)
                return
            body = self.rfile.read(length)
            try:
                json.loads(body)
            except (ValueError, UnicodeDecodeError):
                self.send_error(400)
                return
            destination = logs / f'win7-result-{time.time_ns()}.json'
            temporary = destination.with_suffix('.pending')
            temporary.write_bytes(body)
            temporary.replace(destination)
            self.send_response(200)
            self.send_header('Content-Length', '2')
            self.end_headers()
            self.wfile.write(b'OK')

    print('Win7 test bridge: host 127.0.0.1:48817, UTM guest 10.0.2.2:48817', flush=True)
    ThreadingHTTPServer(('127.0.0.1', 48817), Handler).serve_forever()


if __name__ == '__main__':
    main()
