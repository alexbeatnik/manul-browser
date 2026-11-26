"""The application under test: a small local site with a JSON API.

Everything the demo automates lives in ``site/``. It is served from one origin
so the hunts are deterministic and need no network access, and so MOCK and
WAIT FOR RESPONSE have a real API to intercept.

    python -m demo.app_server            # serve on http://localhost:8765
"""

from __future__ import annotations

import json
import os
import threading
import time
from functools import partial
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

SITE_DIR = Path(__file__).resolve().parent.parent / "site"
DEFAULT_PORT = int(os.environ.get("DEMO_PORT", "8765"))

PRODUCTS = [
    {"name": "Manul Plush", "price": 24.50},
    {"name": "Steppe Map", "price": 12.00},
    {"name": "Snow Boots", "price": 89.99},
]

# A tiny in-memory "backend". CALL HOST handlers reset and query it over HTTP,
# the way a real suite seeds and checks a test database.
STATE: dict[str, list] = {"orders": []}


class _Handler(SimpleHTTPRequestHandler):
    def log_message(self, *_args) -> None:  # keep the demo output readable
        pass

    def _json(self, payload: object, status: int = 200) -> None:
        body = json.dumps(payload).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Cache-Control", "no-store")
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self) -> None:  # noqa: N802 — http.server naming
        path = self.path.split("?", 1)[0]
        if path == "/api/products":
            time.sleep(1.5)  # slower than a CLICK step, so WAIT FOR RESPONSE sees it arrive
            return self._json(PRODUCTS)
        if path == "/api/health":
            return self._json({"status": "ok"})
        if path == "/api/orders":
            return self._json(STATE["orders"])
        return super().do_GET()

    def do_POST(self) -> None:  # noqa: N802
        length = int(self.headers.get("Content-Length") or 0)
        body = json.loads(self.rfile.read(length) or b"{}")
        if self.path == "/api/orders":
            STATE["orders"].append(body)
            return self._json({"id": len(STATE["orders"])}, 201)
        if self.path == "/api/reset":
            STATE["orders"].clear()
            return self._json({"reset": True})
        self._json({"error": "not found"}, 404)


class AppServer:
    """Run the site on a background thread for the length of a ``with`` block."""

    def __init__(self, port: int = DEFAULT_PORT) -> None:
        handler = partial(_Handler, directory=str(SITE_DIR))
        self._httpd = ThreadingHTTPServer(("127.0.0.1", port), handler)
        self.port = self._httpd.server_address[1]
        self.base_url = f"http://localhost:{self.port}"
        self._thread = threading.Thread(target=self._httpd.serve_forever, daemon=True)

    def __enter__(self) -> "AppServer":
        self._thread.start()
        return self

    def __exit__(self, *_exc: object) -> None:
        self._httpd.shutdown()
        self._httpd.server_close()


if __name__ == "__main__":
    with AppServer() as srv:
        print(f"serving {SITE_DIR} on {srv.base_url} — Ctrl+C to stop")
        try:
            while True:
                time.sleep(3600)
        except KeyboardInterrupt:
            pass
