"""txn-api: a tiny fake transactions service used for the platform exercise."""

import json
import logging
import os
import random
import secrets
import sys
import threading
import time
from datetime import datetime, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from prometheus_client import CONTENT_TYPE_LATEST, Counter, Histogram, generate_latest

HTTP_REQUESTS = Counter(
    "http_requests", "HTTP requests by method, route and status code.", ["method", "route", "status"]
)
HTTP_DURATION = Histogram(
    "http_request_duration_seconds",
    "HTTP request latency by method and route.",
    ["method", "route"],
    buckets=(0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5),
)
TXN_PROCESSED = Counter(
    "txn_processed", "Transactions processed by currency and result (ok, rejected, error).", ["currency", "result"]
)

CURRENCIES = {"USD", "EUR", "GBP", "USDC"}
LEVELS = {"debug": logging.DEBUG, "info": logging.INFO, "warn": logging.WARNING, "error": logging.ERROR}


class JSONFormatter(logging.Formatter):
    def format(self, record):
        out = {
            "time": datetime.now(timezone.utc).isoformat(),
            "level": "WARN" if record.levelname == "WARNING" else record.levelname,
            "msg": record.getMessage(),
        }
        out.update(getattr(record, "fields", {}))
        return json.dumps(out)


def make_logger(level):
    handler = logging.StreamHandler(sys.stdout)
    handler.setFormatter(JSONFormatter())
    log = logging.getLogger("txn-api")
    log.addHandler(handler)
    log.setLevel(level)
    return log


def load_config():
    missing = []
    region = os.environ.get("TXN_REGION", "")
    if not region:
        missing.append("TXN_REGION")
    try:
        fail_rate = float(os.environ.get("TXN_FAIL_RATE", "0"))
    except ValueError:
        fail_rate = 0.0
    return {
        "port": int(os.environ.get("PORT", "8000")),
        "region": region,
        "fail_rate": fail_rate,
        "log_level": LEVELS.get(os.environ.get("LOG_LEVEL", "info").lower(), logging.INFO),
    }, missing


def simulate_work():
    """Sleep for a plausible amount of time, with a slow tail."""
    ms = 5 + random.randrange(40)
    if random.random() < 0.05:
        ms += 200 + random.randrange(300)
    time.sleep(ms / 1000)


class State:
    def __init__(self, cfg, log):
        self.cfg = cfg
        self.log = log
        self.ready = False
        self.lock = threading.Lock()
        self.store = {}


class Handler(BaseHTTPRequestHandler):
    state: State = None
    protocol_version = "HTTP/1.1"

    def log_message(self, *args):
        pass  # we do our own structured logging

    def send_body(self, status, body, content_type="application/json"):
        data = body if isinstance(body, bytes) else body.encode()
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def send_json(self, status, obj):
        self.send_body(status, json.dumps(obj) + "\n")

    def do_GET(self):
        path = self.path.split("?", 1)[0]
        if path == "/healthz":
            return self.send_body(200, "ok\n", "text/plain")
        if path == "/readyz":
            if self.state.ready:
                return self.send_body(200, "ready\n", "text/plain")
            return self.send_body(503, "not ready\n", "text/plain")
        if path == "/metrics":
            return self.send_body(200, generate_latest(), CONTENT_TYPE_LATEST)
        prefix = "/api/transactions/"
        if path.startswith(prefix) and len(path) > len(prefix) and "/" not in path[len(prefix):]:
            return self.instrument("/api/transactions/{id}", self.handle_get, path[len(prefix):])
        self.send_body(404, "404 page not found\n", "text/plain")

    def do_POST(self):
        if self.path.split("?", 1)[0] == "/api/transactions":
            return self.instrument("/api/transactions", self.handle_create)
        self.send_body(404, "404 page not found\n", "text/plain")

    def instrument(self, route, fn, *args):
        start = time.monotonic()
        status, body = fn(*args)
        self.send_json(status, body)
        elapsed = time.monotonic() - start

        HTTP_REQUESTS.labels(self.command, route, str(status)).inc()
        HTTP_DURATION.labels(self.command, route).observe(elapsed)

        fields = {
            "method": self.command,
            "route": route,
            "status": status,
            "duration_ms": round(elapsed * 1000, 3),
            "region": self.state.cfg["region"],
        }
        if status >= 500:
            fields["error"] = body.get("error", "")
            self.state.log.error("request failed", extra={"fields": fields})
        elif status >= 400:
            fields["error"] = body.get("error", "")
            self.state.log.warning("request rejected", extra={"fields": fields})
        else:
            self.state.log.info("request handled", extra={"fields": fields})

    def handle_create(self):
        try:
            length = int(self.headers.get("Content-Length", "0"))
            req = json.loads(self.rfile.read(length) or b"null")
            if not isinstance(req, dict):
                raise ValueError
        except ValueError:
            TXN_PROCESSED.labels("unknown", "rejected").inc()
            return 400, {"error": "invalid JSON body"}

        cur = str(req.get("currency", "")).upper()
        if cur not in CURRENCIES:
            TXN_PROCESSED.labels("unknown", "rejected").inc()
            return 400, {"error": "unsupported currency"}
        amount = req.get("amount_cents")
        if not isinstance(amount, int) or amount <= 0:
            TXN_PROCESSED.labels(cur, "rejected").inc()
            return 400, {"error": "amount_cents must be positive"}

        simulate_work()
        if random.random() < self.state.cfg["fail_rate"]:
            TXN_PROCESSED.labels(cur, "error").inc()
            return 500, {"error": "ledger write timed out"}

        txn = {
            "id": "txn_" + secrets.token_hex(8),
            "amount_cents": amount,
            "currency": cur,
            "region": self.state.cfg["region"],
            "created_at": datetime.now(timezone.utc).isoformat(),
        }
        with self.state.lock:
            self.state.store[txn["id"]] = txn
        TXN_PROCESSED.labels(cur, "ok").inc()
        return 201, txn

    def handle_get(self, txn_id):
        simulate_work()
        if random.random() < self.state.cfg["fail_rate"] / 2:
            return 503, {"error": "ledger read replica unavailable"}
        with self.state.lock:
            txn = self.state.store.get(txn_id)
        if txn is None:
            return 404, {"error": "transaction not found"}
        return 200, txn


def main():
    cfg, missing = load_config()
    log = make_logger(cfg["log_level"])
    if missing:
        log.error("missing required environment variable", extra={"fields": {"vars": ",".join(missing)}})
        sys.exit(1)

    Handler.state = State(cfg, log)
    server = ThreadingHTTPServer(("0.0.0.0", cfg["port"]), Handler)
    Handler.state.ready = True
    log.info(
        "starting txn-api",
        extra={"fields": {"port": str(cfg["port"]), "region": cfg["region"], "fail_rate": cfg["fail_rate"]}},
    )
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass


if __name__ == "__main__":
    main()
