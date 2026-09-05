#!/usr/bin/env python3
"""Управляющий сервер для liveblocksscan.

Запускает сетевые watcher-скрипты как дочерние процессы, собирает их вывод
в кольцевой буфер и отдаёт статус/лог через маленький HTTP API с токеном.

Запуск:
    python3 server.py [--host 127.0.0.1] [--port 9299]

Конфигурация через env:
    LIVEBLOCKS_TOKEN    — токен авторизации (если пуст — генерируется и
                          печатается в stderr при старте);
    LIVEBLOCKS_HOST/ LIVEBLOCKS_PORT — адрес прослушивания;
    LIVEBLOCKS_SCRIPTS_DIR — каталог со скриптами (по умолчанию — рядом
                          с этим файлом);
    VAULN_API_URL, ADMIN_API_KEY — что передавать watcher-скриптам
                          (скрипты получают их и через свои env);
    LIVEBLOCKS_AUTO_START  — список имён скриптов через запятую, запустить
                          при старте сервера (например "ethereum,bitcoin");
    LIVEBLOCKS_NO_AUTH=1  — отключить авторизацию (только для отладки).

Endpoints (все требуют `Authorization: Bearer <token>`, кроме самой
страницы `/` — она открывает UI, который спрашивает токен):
    GET    /                          — веб-дашборд;
    GET    /api/health                — {ok, scripts_running, scripts_total};
    GET    /api/scripts              — статусы всех скриптов;

    POST   /api/scripts/<name>/start    — запустить (тело опционально:
                                          {"interval", "start", "once", "extra"});
    POST   /api/scripts/<name>/stop     — остановить SIGTERM (через ~6с — SIGKILL);
    POST   /api/scripts/<name>/restart  — перезапустить;
    GET    /api/scripts/<name>/logs?after=N — строки лога с seq > N.
"""

import argparse
import gc
import hmac
import json
import os
import re
import secrets
import signal
import sys
import threading
import time
from collections import deque
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from subprocess import DEVNULL, PIPE, STDOUT, Popen


def load_dotenv():
    """Загрузить liveblocksscan/.env в окружение (не перезаписывая его.


    Ищет .env рядом с server.py, затем в текущем каталоге;, адрес можно
    переопределить через LIVEBLOCKS_ENV_DIR. Комментарии: строки на "#";
    значения в кавычках — как в docker-compose.

    """
    root = Path(__file__).resolve().parent
    candidates = [root / ".env", Path.cwd() / ".env"]
    if os.environ.get("LIVEBLOCKS_ENV_DIR"):
        candidates.insert(0, Path(os.environ["LIVEBLOCKS_ENV_DIR"]) / ".env")
    seen = set()
    for path in candidates:
        try:
            with path.open("r", encoding="utf-8") as fh:
                lines = fh.readlines()
        except OSError:
            continue
        for raw in lines:
            line = raw.strip()
            if not line or line.startswith("#"):
                continue
            key, _, val = line.partition("=")
            key = key.strip()
            if not key or not re.match(r"^[A-Za-z_][A-Za-z0-9_]*$", key):
                continue
            if key in seen or key in os.environ:
                continue
            seen.add(key)
            val = val.strip()
            if len(val) >= 2 and val[0] == '"' and val[-1] == '"':
                val = val[1:-1].replace('""', '""')
            os.environ[key] = val


load_dotenv()  # до констант конфига ниже


# ---------------------------------------------------------------- конфиг

DEFAULT_API_URL = os.environ.get("VAULN_API_URL", "http://127.0.0.1:8080")
DEFAULT_ADMIN_KEY = os.environ.get("ADMIN_API_KEY", "")

DEFAULT_HOST = os.environ.get("LIVEBLOCKS_HOST", "127.0.0.1")
DEFAULT_PORT = int(os.environ.get("LIVEBLOCKS_PORT", "9299"))
SCRIPTS_DIR = Path(os.environ.get("LIVEBLOCKS_SCRIPTS_DIR",
                   os.path.dirname(os.path.abspath(__file__))))
AUTO_START = [s.strip() for s in
               os.environ.get("LIVEBLOCKS_AUTO_START", "").split(",") if s.strip()]
NO_AUTH = os.environ.get("LIVEBLOCKS_NO_AUTH", "").lower() in {"1", "true", "yes"}

TOKEN = os.environ.get("LIVEBLOCKS_TOKEN", "").strip()
if not TOKEN:
    TOKEN = secrets.token_urlsafe(24)

SCRIPT_RE = re.compile(r"^[a-z0-9_]+\.py$")
EXCLUDED = {"common.py", "server.py"}

MAX_LINES = 2000
TAIL_LINES = 20
KILL_GRACE = 6.0

CHAIN_LABELS = {
    "ethereum": "Ethereum",
    "bnb": "BNB Smart Chain",
    "base": "Base",
    "linea": "Linea",
    "arbitrum": "Arbitrum One",
    "polygon": "Polygon PoS",
    "optimism": "Optimism",
    "avalanche": "Avalanche C-Chain",
    "bitcoin": "Bitcoin",
    "solana": "Solana",
    "tron": "TRON",
    "sui": "Sui",
}


def discover_scripts():
    """Имена скриптов-наблюдателей в каталоге (кроме служебных)."""
    out = []
    if not SCRIPTS_DIR.is_dir():
        return out
    for p in sorted(SCRIPTS_DIR.iterdir()):
        if p.name in EXCLUDED or not SCRIPT_RE.match(p.name):
            continue
        out.append(p.name)
    return out


# ------------------------------------------------------------- менеджер

class ScriptState:
    __slots__ = ("name", "path", "proc", "running", "stopping",
                 "pid", "started_at", "exited_at", "exit_code",
                 "spawn_count", "lines", "seq", "last_activity", "extra_args")
    def __init__(self, name, path):
        self.name = name
        self.path = path
        self.proc = None
        self.running = False
        self.stopping = False
        self.pid = None
        self.started_at = None
        self.exited_at = None
        self.exit_code = None
        self.spawn_count = 0
        self.lines = deque(maxlen=MAX_LINES)
        self.seq = 0
        self.last_activity = None
        self.extra_args = None

    def uptime(self):
        if not self.running or self.started_at is None:
            return None
        return time.time() - self.started_at

    def append_line(self, text):
        self.lines.append({"seq": self.seq, "ts": time.time(),
                          "text": text.rstrip("\n").rstrip("\r")})
        self.seq += 1
        self.last_activity = time.time()

    def summary(self, tail=TAIL_LINES):
        return {
            "name": self.name,
            "label": CHAIN_LABELS.get(self.name[:-3], self.name[:-3].title()),
            "running": self.running,
            "stopping": self.stopping,
            "pid": self.pid,
            "started_at": self.started_at,
            "uptime": self.uptime(),
            "exited_at": self.exited_at,
            "exit_code": self.exit_code,
            "spawn_count": self.spawn_count,
            "last_activity": self.last_activity,
            "lines_total": len(self.lines),
            "tail": list(self.lines)[-tail:],
            "extra_args": self.extra_args,
        }


class Manager:
    def __init__(self):
        self.lock = threading.RLock()
        self.scripts = {}
        self.reaper_stop = threading.Event()
        for name in discover_scripts():
            self.scripts[name] = ScriptState(name, SCRIPTS_DIR / name)

    # ------------------------------------------------------------ helpers

    def _reader(self, st, name):
        try:
            while True:
                raw = st.readline()
                if not raw:
                    break
                if isinstance(raw, bytes):
                    raw = raw.decode("utf-8", "replace")
                with self.lock:
                    self.scripts[name].append_line(raw)
        finally:
            try:
                st.close()
            except OSError:
                pass

    def _spawn(self, st, extra_args):
        env = os.environ.copy()
        env["PYTHONUNBUFFERED"] = "1"
        env["VAULN_API_URL"] = DEFAULT_API_URL
        env["ADMIN_API_KEY"] = DEFAULT_ADMIN_KEY
        cmd = [sys.executable, str(st.path)]
        for a in extra_args or []:
            cmd.append(str(a))
        with self.lock:
            if st.running:
                raise RuntimeError("already_running")
            proc = Popen(cmd, cwd=str(SCRIPTS_DIR), env=env,
                        stdin=DEVNULL, stdout=PIPE, stderr=STDOUT)
            st.proc = proc
            st.running = True
            st.stopping = False
            st.pid = proc.pid
            st.started_at = time.time()
            st.exited_at = None
            st.exit_code = None
            st.spawn_count += 1
            st.extra_args = list(extra_args or [])
            st.append_line(f"--- запуск: {' '.join(cmd)} (pid {proc.pid})")
        threading.Thread(target=self._reader, args=(proc.stdout, st.name),
                        daemon=True).start()
        return True

    def _finalize_exit(self, st):
        with self.lock:
            if not st.running:
                return
            code = st.proc.poll()
            st.running = False
            st.exited_at = time.time()
            st.exit_code = code if code is not None else 0
            st.stopping = False
            st.append_line(
                f"--- процесс завершён (код {st.exit_code})"
                if st.exit_code == 0 else
                f"--- процесс завершён с ошибкой (код {st.exit_code})")

    def _stop(self, st):
        with self.lock:
            if not st.running or st.proc is None:
                return False
            st.stopping = True
            try:
                st.proc.terminate()
            except OSError:
                pass
        # ждём штатного выхода (watcher-поток зафиксирует его; если за
        # KILL_GRACE секунд процесс жив — прибьём.

        deadline = time.time() + KILL_GRACE
        while time.time() < deadline:
            with self.lock:
                if not st.running:
                    return True
            time.sleep(0.1)
        with self.lock:
            if st.running and st.proc is not None:
                try:
                    st.proc.kill()
                except OSError:
                    pass
        return True

    # ------------------------------------------------------------ API calls

    def list_scripts(self):
        with self.lock:
            return [st.summary() for st in self.scripts.values()]

    def health(self):
        with self.lock:
            total = len(self.scripts)
            running = sum(1 for st in self.scripts.values() if st.running)
        return {"ok": True, "total": total, "running": running,
                "token_required": not NO_AUTH}

    def start(self, name, body):
        st = self.scripts.get(name)
        if st is None:
            raise KeyError(name)
        body = body or {}
        if not isinstance(body, dict):
            raise ValueError("тело должно быть JSON-объектом")
        extra = []
        if body.get("interval") is not None:
            extra += ["--interval", str(float(body["interval"]))]
        if body.get("start") is not None:
            extra += ["--start", str(int(body["start"]))]
        if body.get("once"):
            extra.append("--once")
        for a in body.get("extra") or []:
            if not isinstance(a, str) or not a:
                continue
            extra.append(a)
        with self.lock:
            if st.running:
                raise RuntimeError("already_running")
        return self._spawn(st, extra)

    def stop(self, name):
        st = self.scripts.get(name)
        if st is None:
            raise KeyError(name)
        self._stop(st)
        return True

    def restart(self, name, body):
        self.stop(name)
        time.sleep(0.2)
        return self.start(name, body)

    def logs(self, name, after=0):
        st = self.scripts.get(name)
        if st is None:
            raise KeyError(name)
        with self.lock:
            return [l for l in st.lines if l["seq"] > after]

    def reap(self):
        """Фоновый поток: ловит завершения процессов и обновляет статус."""
        while True:
            if self.reaper_stop.wait(0.5):
                return
            with self.lock:
                snap = [st for st in self.scripts.values() if st.running]
            for st in snap:
                with self.lock:
                    if not st.running or st.proc is None:
                        continue
                    code = st.proc.poll()
                    if code is not None:
                        self._finalize_exit(st)
                    elif st.stopping and time.time() - st.started_at > KILL_GRACE:
                        try:
                            st.proc.kill()
                        except OSError:
                            pass

    def shutdown(self):
        """Остановить все дочерние процессы (SIGTERM, затем KILL)."""
        with self.lock:
            sts = [st for st in self.scripts.values() if st.running]
        for st in sts:
            self._stop(st)
        # reaper фиксирует завершения; останавливаем его после остановки всех.
        self.reaper_stop.set()


# ------------------------------------------------------- HTTP handler

class Handler(BaseHTTPRequestHandler):
    server_version = "liveblocksscan-server/1.0"
    protocol_version = "HTTP/1.1"

    @property
    def manager(self):
        return self.server.manager  # type: ignore[attr-defined]

    # ------------------------------------------------------------- helpers

    def _json(self, obj, status=200):
        data = json.dumps(obj, ensure_ascii=False).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(data)))
        self.send_header("Cache-Control", "no-store")
        self.end_headers()
        self.wfile.write(data)

    def _html(self, html):
        data = html.encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "text/html; charset=utf-8")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def _no_content(self):
        self.send_response(204)
        self.send_header("Content-Length", "0")
        self.end_headers()

    def _authorized(self):
        if NO_AUTH:
            return True
        header = self.headers.get("Authorization", "")
        if header.startswith("Bearer "):
            given = header[7:].strip()
        else:
            given = self.headers.get("X-Auth-Token", "")
        if not given:
            qs = self.path.split("?", 1)
            if len(qs) == 2:
                query = dict(x.split("=", 1) for x in qs[1].split("&") if "=" in x)
                given = query.get("token", "")
        return hmac.compare_digest(given.encode(), TOKEN.encode())

    def _not_found(self):
        self._json({"error": "not_found"}, 404)

    def _bad(self, msg):
        self._json({"error": msg}, 400)

    def _conflict(self, msg):
        self._json({"error": msg}, 409)

    def _script_from_path(self):
        parts = self.path.split("?", 1)[0].strip("/").split("/")
        # /api/scripts/<name>/action[/logs]
        if len(parts) < 3 or parts[:2] != ["api", "scripts"]:
            return None, parts
        return parts[2], parts

    def log_message(self, fmt, *args):
        if self.path.startswith("/api/health"):
            return
        sys.stderr.write("%s - - [%s] %s\n" %
                         (self.address_string(), self.log_date_time_string(),
                          fmt % args))

    # -------------------------------------------------------------- routes

    def do_GET(self):
        path = self.path.split("?", 1)[0]
        if path == "/":
            self._html(INDEX_HTML)
            return
        if path == "/favicon.ico":
            self._no_content()
            return
        if not self._authorized():
            self._json({"error": "unauthorized"}, 401)
            return
        if path == "/api/health":
            self._json(self.manager.health())
            return
        if path == "/api/scripts":
            self._json({"scripts": self.manager.list_scripts()})
            return
        name, parts = self._script_from_path()
        if name:
            if len(parts) == 4 and parts[3] == "logs":
                query = {}
                if "?" in self.path:
                    qs = self.path.split("?", 1)[1]
                    for kv in qs.split("&"):
                        if "=" in kv:
                            k, v = kv.split("=", 1)
                            query[k] = v
                try:
                    after = int(query.get("after", "0"))
                except ValueError:
                    after = 0
                try:
                    lines = self.manager.logs(name, after)
                except KeyError:
                    self._not_found()
                    return
                self._json({"name": name, "lines": lines})
                return
            self._not_found()
            return
        self._not_found()

    def do_POST(self):
        if not self._authorized():
            self._json({"error": "unauthorized"}, 401)
            return
        path = self.path.split("?", 1)[0]
        name, parts = self._script_from_path()
        if not name:
            self._not_found()
            return
        if len(parts) != 4:
            self._not_found()
            return
        action = parts[3]
        body = None
        try:
            length = int(self.headers.get("Content-Length", "0"))
            if length:
                raw = self.rfile.read(length)
                if raw:
                    body = json.loads(raw.decode("utf-8"))
        except (ValueError, json.JSONDecodeError):
            self._bad("invalid JSON body")
            return
        try:
            if action == "start":
                self.manager.start(name, body)
            elif action == "restart":
                self.manager.restart(name, body)
            elif action == "stop":
                self.manager.stop(name)
            else:
                self._not_found()
                return
        except KeyError:
            self._not_found()
            return
        except ValueError as e:
            self._bad(str(e))
            return
        except RuntimeError as e:
            if str(e) == "already_running":
                self._conflict("script is already running")
                return
            self._bad(str(e))
            return
        self._json({"ok": True})

    do_PUT = do_POST


# ------------------------------------------------------------- main

def main():
    ap = argparse.ArgumentParser(description="Управляющий сервер liveblocksscan")
    ap.add_argument("--host", default=DEFAULT_HOST)
    ap.add_argument("--port", type=int, default=DEFAULT_PORT)
    args = ap.parse_args()

    manager = Manager()
    httpd = ThreadingHTTPServer((args.host, args.port), Handler)
    httpd.manager = manager  # type: ignore[attr-defined]

    reaper = threading.Thread(target=manager.reap, daemon=True)
    reaper.start()

    print("=" * 60, flush=True)
    print(f"liveblocksscan управляющий сервер: http://{args.host}:{args.port}", flush=True)
    if NO_AUTH:
        print("АВТОРИЗАЦИЯ ОТКЛЮЧЕНА (LIVEBLOCKS_NO_AUTH)", flush=True)
    else:
        print(f"Токен авторизации: {TOKEN}", flush=True)
    print("Watcher'ы:", ", ".join(manager.scripts.keys()) or "нет",
          flush=True)
    if AUTO_START:
        print(f"Автостарт: {', '.join(AUTO_START)}", flush=True)
        for name in AUTO_START:
            if name not in manager.scripts:
                print(f"  !! неизвестный скрипт для автостарта: {name}",
                      file=sys.stderr, flush=True)
                continue
            try:
                manager.start(name, None)
            except Exception as e:  # noqa: BLE001
                print(f"  !! не удалось запустить {name}: {e}",
                      file=sys.stderr, flush=True)
    print("=" * 60, flush=True)

    def shutdown(signum, frame):  # noqa: ARG001
        # httpd.shutdown() блокирует до конца serve_forever в главном потоке;
        # cleanup запускаем в отдельном потоке, иначе сигнальный обработчик
        # (сам главный поток) дедлачил бы сам с собой。
        print("\nОстановка…", flush=True)
        threading.Thread(
            target=lambda: (manager.shutdown(), httpd.shutdown()), daemon=True
        ).start()

    signal.signal(signal.SIGTERM, shutdown)
    signal.signal(signal.SIGINT, shutdown)

    try:
        httpd.serve_forever(poll_interval=0.5)
    finally:
        manager.shutdown()
        httpd.server_close()
        sys.exit(0)


INDEX_HTML = r"""<!doctype html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>liveblocksscan — управление</title>
<style>
:root {
  --bg: #0c0f14; --panel: #131a24; --panel2: #0f1520;
  --border: #1f2937; --text: #e7ecf5; --muted: #8a96ad;
  --accent: #3b5fcf; --green: #27ae60; --red: #e05252; --yellow: #d4a017;
}
* { margin: 0; padding: 0; box-sizing: border-box; }
body {
  background: var(--bg);
  background-image: radial-gradient(circle at 30% 20%, #1a202a, #07090c 95%);
  color: var(--text);
  font: 14px/1.45 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, system-ui, sans-serif;
  min-height:100vh; padding: 24px;
}
.wrap { max-width: 1040px; margin: 0 auto; }
header {
  display:flex; align-items:center; justify-content:space-between; gap:16px; flex-wrap:wrap;
  margin-bottom: 20px;
}
h1 { font-size:20px; font-weight:700; }
h1 small { color: var(--muted); font-weight:400; font-size:13px; display:block; margin-top:2px; }
.header-actions { display:flex; align-items:center; gap:12px; }
.badge-reset { color:var(--muted); font-size:12px; }
.btn {
  background: #1a2331; color: var(--text); border:1px solid var(--border);
  border-radius:8px; padding:7px 14px; cursor:pointer; font-size:13px;
  transition: background .15s, border-color .15s, opacity .15s;
}
.btn:hover { background:#223047; border-color:#2d3e55; }
.btn:disabled { opacity:.45; cursor:not-allowed; }
.btn.primary { background: var(--accent); border-color:var(--accent); }
.btn.primary:hover { background:#4a6fe0; }
.btn.small { padding:4px 10px; font-size:12px; border-radius:6px; }
.btn.danger:hover { background:#3a1d1d; border-color:#5f3333; }
.btn: { border-color:transparent; background:transparent; }

.cards { display:grid; grid-template-columns:repeat(auto-fill, minmax(340px, 1fr)); gap:14px;; }
.card {
  background: var(--panel); border:1px solid var(--border); border-radius:14px;
  padding:14px 16px; box-shadow:0 8px 24px rgba(0,0,0,.25);
}
.card.head-row { display:flex; align-items:center; gap:10px; margin-bottom:10px; flex-wrap:wrap; }
.dot { width:9px; height:9px; border-radius:50%; background:var(--muted); flex:none; }
.dot.running { background:var(--green); box-shadow:0 0 0 3px rgba(39,174,96,.18); animation:pulse 2s infinite; }
.dot.stopping { background:var(--yellow); }
.dot.error { background:var(--red); box-shadow:0 0 0 3px rgba(224,82,82,.15); }
.dot.stopped { background:var(--muted); }
@keyframes pulse { 50% { box-shadow:0 0 0 5px rgba(39,174,96,.08); } }
.name { font-size:15px; font-weight:600; }
.chain { color:var(--muted); font-size:12px; margin-left:4px; }
.spacer { flex:1; }
.status { font-size:12px; color:var(--muted); }
.status b.running { color:var(--green); }
.status b.error { color:var(--red); }
.meta { display:flex; gap:16px; flex-wrap:wrap; font-size:12px; color:var(--muted); margin:6px 0 10px; }
.meta span b { color:var(--text); font-weight:600; }
.out { margin-top:14px; border-top:1px solid var(--border); padding-top:10px; font-size:12px; }
.out-head { display:flex; align-items:center; justify-content:space-between; margin-bottom:6px; color:var(--muted);; }
.out-head .btn { padding:2px 8px; font-size:11px; }
.logs {
  background: var(--panel2); border:1px solid var(--border); border-radius:8px;
  padding:8px 10px; max-height:260px; overflow-y:auto; overflow-x:hidden;
  font-family:'SF Mono', ui-monospace, Menlo, Consolas, monospace; font-size:11.5px;
  line-height:1.5; white-space:pre-wrap; word-break:break-all;
}
.logs .line { color:#c7d2e6; }
.logs .line .t { color:#5a6a8e; margin-right:8px; user-select:none; }
.logs .line.die { color:var(--red); }
.logs .line.run { color:var(--green); }
.empty { color:var(--muted); font-style:italic; padding:4px 2px; }

.login {
  max-width:380px; margin:18vh auto; text-align:center;
}
.login .panel { background:var(--panel); border:1px solid var(--border); border-radius:16px; padding:28px 24px; text-align:left;; }
.login h2 { margin-bottom:6px; font-size:18px; }
.login p { color:var(--muted); font-size:13px; margin-bottom:18px; }
.login .field { margin-bottom:14px; }
.login label { display:block; font-size:12px; color:var(--muted); margin-bottom:6px; }
.login input, .adv input {
  width:100%; background:var(--panel2); border:1px solid var(--border); border-radius:8px;
  color:var(--text); padding:9px 12px; font-size:13px; outline:none;
}
.login input:focus, .adv input:focus { border-color:var(--accent);; }
.login .err { color:var(--red); font-size:12px; margin-top:10px; min-height:16px; }
.foot { margin-top:22px; text-align:center; color:var(--muted); font-size:12px; }

.adv { margin-top:10px; padding:10px; border:1px dashed var(--border); border-radius:8px; display:grid; grid-template-columns:110px 1fr; gap:8px; }
..adv label { font-size:11px; color:var(--muted); grid-column:1 / 3; margin-top:4px; }
..adv input { padding:5px 8px; font-size:12px; }
..adv .ck { display:flex; align-items:center; gap:8px; font-size:12px; color:var(--muted);; }
..adv .ck input { width:auto; }
..adv .full { grid-column:1 / 3; }

#toast {
  position:fixed; top:16px; right:16px; z-index:50; display:flex; flex-direction:column; gap:8px;;
}
#toast div { background:#2a1216; border:1px solid #5f3333; color:#ffd9d9; padding:10px 16px; border-radius:10px; font-size:13px; box-shadow:0 8px 24px rgba(0,0,0,.4); animation:in .2s; }
@keyframes in { from { opacity:0; transform:translateY(-6px); } }
@media (max-width:640px) { .cards { grid-template-columns:1fr; } body { padding:14px; } }
</style>
</head>
<body>
<div class="wrap" id="app"></div>
<div id="toast"></div>
<script>
const TOKEN_KEY = 'lb_token';
const $ = (sel, el=document) => el.querySelector(sel);
let state = { scripts: [], token: localStorage.getItem(TOKEN_KEY) || '', expanded: new Set(), adv: new Set() };

function el(tag, attrs, ...kids) {
  const n = document.createElement(tag);
  for (const [k,v] of Object.entries(attrs || {})) {
    if (k === 'class') n.className = v;
    else if (k === 'text') n.textContent = v;
    else if (k === 'html') n.innerHTML = v;
    else n[k] = v;
  }
  for (const c of kids.flat()) if (c) n.append(c);
  return n;
}

function toast(msg) {
  const d = el('div', { text: msg });
  $('#toast').append(d);
  setTimeout(() => d.remove(), 4200);
}

async function api(path, opts = {}) {
  const headers = Object.assign({ 'Content-Type': 'application/json' }, opts.headers || {});
  if (state.token) headers['Authorization'] = 'Bearer ' + state.token;
  const r = await fetch(path, Object.assign({}, opts, { headers }));
  if (r.status === 401) {
    state.token = '';
    localStorage.removeItem(TOKEN_KEY);
    renderLogin();
    throw new Error('unauthorized');
  }
  const ct = r.headers.get('content-type') || '';
  const body = ct.includes('application/json') ? await r.json() : null;
  if (!r.ok) throw new Error((body && body.error) || ('HTTP ' + r.status));
  return body;
}

function fmtUptime(s) {
  if (s == null) return '—';
  s = Math.floor(s);
  const d = Math.floor(s / 86400), h = Math.floor(s % 86400 / 3600);
  const m = Math.floor(s % 3600 / 60), sec = s % 60;
  if (d) return `${d}д ${h}ч`;
  if (h) return `${h}ч ${m}м`;
  if (m) return `${m}м ${sec}с`;
  return `${sec}с`;
}

function fmtClock(ts) {
  if (!ts) return '—';
  const t = new Date(ts * 1000);
  return t.toLocaleTimeString('ru-RU', { hour:'2-digit', minute:'2-digit', second:'2-digit' });
}

function fmtAgo(ts) {
  if (!ts) return '—';
  const d = Math.floor((Date.now()/1000) - ts);
  if (d < 5) return 'только что';
  if (d < 60) return `${Math.floor(d)}с назад`;
  if (d < 3600) return `${Math.floor(d/60)}м назад`;
  return `${Math.floor(d/3600)}ч назад`;
}

function statusDot(s) {
  if (s.running) return s.stopping ? 'stopping' : 'running';
  if (s.exit_code == null) return 'stopped';
  return s.exit_code === 0 ? 'stopped' : 'error';
}
function statusText(s) {
  if (s.running) return s.stopping ? 'останавливается…' : 'работает';
  if (s.exit_code == null) return 'остановлен';
  return s.exit_code === 0 ? 'завершён' : `ошибка (код ${s.exit_code})`;
}

function logClass(l) {
  if (l.text.includes('процесс завершён') || l.text.includes('с ошибкой')) return 'die';
  if (l.text.startsWith('--- запуск')) return 'run';
  return '';
}

function renderLogin(err) {
  const app = $('#app');
  app.replaceChildren(el('div', { class:'login' }, el('div', { class:'panel' },
    el('h2', { text:'🔐 liveblocksscan' }),
    el('p', { text:'Введите токен авторизации (сервер показывает его при старте)' }),
    el('div', { class:'field' }, el('label', { text:'Токен' }), el('input', { id:'tkn', type:'password', placeholder:'…' })),
    el('button', { class:'btn primary', id:'go', text:'Войти' }),
    el('div', { class:'err', text:err || '' })
  )));
  $('#go').onclick = () => {
    const t = $('#tkn').value.trim();
    if (!t) return;
    state.token = t;
    localStorage.setItem(TOKEN_KEY, t);
    try { api('/api/scripts').then(init).catch(e => { state.token = ''; localStorage.removeItem(TOKEN_KEY); renderLogin(e.message); }); }
    catch (e) { renderLogin(e.message); }
  };
  $('#tkn').onkeydown = (e) => { if (e.key === 'Enter') $('#go').click(); };
}

function renderCard(s) {
  const dot = el('span', { class:'dot ' + statusDot(s) });
  const meta = el('div', { class:'meta' },
    el('span', { html:'<b>' + (s.pid || '—') + '</b> · pid' }),
    el('span', { html:'работает <b>' + fmtUptime(s.uptime) + '</b>' }),
    el('span', { html:'запусков <b>' + s.spawn_count + '</b>' }),
    el('span', { html:'строк <b>' + s.lines_total + '</b>' }),
    el('span', { html:'активность ' + fmtAgo(s.last_activity) })
  );
  const acts = el('div', { class:'header-actions' },
    el('button', { class:'btn small primary', disabled:s.running, text:'▶ Запустить' }),
    el('button', { class:'btn small', text:'⚙' }),
    el('button', { class:'btn small', disabled:!s.running, text:'⏹' }),
    el('button', { class:'btn small', disabled:!s.running, text:'↻' })
  );
  acts.children[0].onclick = () => startScript(s, actss.children[0]);
  acts.children[1].onclick = () => toggleAdv(s.name);
  acts.children[2].onclick = () => stopScript(s, true);
  acts.children[3].onclick = () => restartScript(s);
  const head = el('div', { class:'head-row' }, dot,
    el('span', { class:'name', text:s.name.replace(/\.py$/, '') }),
    el('span', { class:'chain', text:s.label }),
    el('span', { class:'spacer' }),
    el('span', { class:'status' }, el('b', { class:statusDot(s), text:statusText(s) })),
    acts
  );
  const outHead = el('div', { class:'out-head' },
    el('span', { text:'Вывод' }),
    el('button', { class:'btn small', text:state.expanded.has(s.name) ? 'Свернуть' : 'Развернуть' })
  );
  outHead.children[1].onclick = () => { state.expanded.has(s.name) ? state.expanded.delete(s.name) : state.expanded.add(s.name); render(); };
  const logs = el('div', { class:'logs', hidden:!state.expanded.has(s.name) });
  logs.dataset.name = s.name;

  const card = el('div', { class:'card' }, head, meta, outHead, logs);
  if (s.extra_args && s.extra_args.length) card.append(el('div', { class:'meta', html:'аргументы: <b>' + s.extra_args.join(' ') + '</b>' }));
  return card;
}

function render() {
  const app = $('#app');
  const running = state.scripts.filter(s => s.running).length;
  const header = el('header', {},
    el('h1', { html:'🛰 liveblocksscan <small>live-мониторинг блоков · ' + running + '/' + state.scripts.length + ' работают</small>' }),
    el('div', { class:'header-actions' },
      el('span', { class:'badge-reset', text:'обновление: 5с' }),
      el('button', { class:'btn', text:running ? 'Остановить все' : 'Запустить все' }),
      el('button', { class:'btn', text:'↻' }),
      el('button', { class:'btn danger', id:'logout', text:'Выйти' })
    )
  );
  header.children[1].children[1].onclick = () => { const s = running ? 'stop' : 'start'; state.scripts.forEach(x => (s === 'start' ? startScript(x) : stopScript(x, false))); };
  header.children[1].children[2].onclick = () => refresh(true);
  header.children[1].children[3].onclick = () => {
    state.token = '';
    localStorage.removeItem(TOKEN_KEY);
    renderLogin();
    clearInterval(window.__autoRefresh);
  };
  const cards = state.scripts.map(renderCard);
  app.replaceChildren(header, ...cards, el('div', { class:'foot', text:'liveblocksscan · сервер процессов' }));
  renderLogs();
}

function renderLogs() {
  state.scripts.forEach(s => {
    const logs = $('.logs[data-name="' + s.name + '"]');
    if (!logs || logs.hidden) return;
    const wasBottom = logs.scrollTop + logs.clientHeight >= logs.scrollHeight - 20;
    logs.replaceChildren(...s.tail.map(l =>
      el('div', { class:'line ' + logClass(l)},
        el('span', { class:'t', text:fmtClock(l.ts) }), document.createTextNode(l.text))
      )
    );
    if (wasBottom) logs.scrollTop = logs.scrollHeight;
  });
}

async function refresh(markActive) {
  try {
    const data = await api('/api/scripts?ts=' + Date.now());
    state.scripts = data.scripts;
    if (markActive) clearInterval(window.__autoRefresh);
    render();
    scheduleRefresh();
  } catch (e) {
    if (e.message !== 'unauthorized') { /* уже в логіне */ }
  }
}

function scheduleRefresh() {
  clearInterval(window.__autoRefresh);
  window.__autoRefresh = setInterval(() => refresh(false), 5000);
}

function startScript(s, btn) {
  const extra = collectAdv(s.name);
  api('/api/scripts/' + s.name + '/start', { method:'POST', body:JSON.stringify(extra)})
    .then(() => refresh(true)).catch(e => toast(e.message));
}

function stopScript(s, confirmStop) {
  if (confirmStop && !confirm('Остановить ' + s.name + '?')) return;
  api('/api/scripts/' + s.name + '/stop', { method:'POST' })
    .then(() => refresh(true)).catch(e => toast(e.message));
}

function restartScript(s) {
  if (!confirm('Перезапустить ' + s.name + '?')) return;
  api('/api/scripts/' + s.name + '/restart', { method:'POST' })
    .then(() => refresh(true)).catch(e => toast(e.message));
}

function toggleAdv(name) {
  state.adv.has(name) ? state.adv.delete(name) : state.adv.add(name);
  render();
}

function collectAdv(name) {
const adv = $('.adv[data-name="' + name + '"]');
  if (!adv) return {};
  const iv = $('#iv_' + name).value.trim();
  const st = $('#st_' + name).value.trim();
  const oc = $('#oc_' + name).checked;
  const ex = $('#ex_' + name).value.trim();
  const out = {};
  if (iv) out.interval = parseFloat(iv);
  if (st) out.start = parseFloat(st);
  if (oc) out.once = true;
  if (ex) out.extra = ex.split(/\s+/).filter(Boolean);
  return out;
}

function init(data) {
  state.scripts = data.scripts;
 render();
  scheduleRefresh();
}

(function boot() {
  if (!state.token) { renderLogin(); return; }
  api('/api/scripts').then(init).catch(e => { if (e.message !== 'unauthorized') toast(e.message); });
})();
</script>
</body>
</html>
"""


if __name__ == "__main__":
    main()
