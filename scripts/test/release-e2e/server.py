#!/usr/bin/env python3
"""Serve mutable release fixtures on a random localhost port."""

from __future__ import annotations

import http.server
import json
import pathlib
import sys


class NoCacheHandler(http.server.SimpleHTTPRequestHandler):
    def end_headers(self) -> None:
        self.send_header("Cache-Control", "no-store")
        super().end_headers()

    def log_message(self, message: str, *args: object) -> None:
        print(message % args, flush=True)

    def do_GET(self) -> None:
        if self.path.rstrip("/") == "/v1/models":
            payload = json.dumps(
                {"object": "list", "data": [{"id": "e2e-model", "object": "model"}]}
            ).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(payload)))
            self.end_headers()
            self.wfile.write(payload)
            return
        super().do_GET()

    def do_POST(self) -> None:
        if self.path.rstrip("/") != "/v1/chat/completions":
            self.send_error(404)
            return
        length = int(self.headers.get("Content-Length", "0"))
        request = json.loads(self.rfile.read(length) or b"{}")
        content = "TASK_E2E_OK"
        if request.get("stream"):
            chunks = (
                f'data: {json.dumps({"choices": [{"delta": {"content": content}}]})}\n\n'
                "data: [DONE]\n\n"
            ).encode()
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Cache-Control", "no-store")
            self.send_header("Content-Length", str(len(chunks)))
            self.end_headers()
            self.wfile.write(chunks)
            return
        payload = json.dumps(
            {
                "choices": [{"message": {"role": "assistant", "content": content}}],
                "model": "e2e-model",
            }
        ).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)


def main() -> None:
    if len(sys.argv) != 3:
        raise SystemExit("usage: server.py <fixture-root> <port-file>")

    root = pathlib.Path(sys.argv[1]).resolve()
    port_file = pathlib.Path(sys.argv[2])
    handler = lambda *args, **kwargs: NoCacheHandler(  # noqa: E731
        *args, directory=str(root), **kwargs
    )
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), handler)
    port_file.write_text(str(server.server_port), encoding="utf-8")
    server.serve_forever()


if __name__ == "__main__":
    main()
