#!/usr/bin/env python3
"""Serve mutable release fixtures on a random localhost port."""

from __future__ import annotations

import http.server
import pathlib
import sys


class NoCacheHandler(http.server.SimpleHTTPRequestHandler):
    def end_headers(self) -> None:
        self.send_header("Cache-Control", "no-store")
        super().end_headers()

    def log_message(self, message: str, *args: object) -> None:
        print(message % args, flush=True)


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
