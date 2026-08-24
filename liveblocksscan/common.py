"""Общая инфраструктура live-сканеров блоков (liveblocksscan).

Каждый сетевой скрипт (ethereum.py, bitcoin.py, solana.py, …) поллит
последние блоки своей сети, извлекает переводы (sender, recipient, amount),
массово проверяет адреса через POST /api/check/bulk и для движений средств
с участием адресов из базы угроз отправляет находки на
POST /api/admin/scanner/findings — они появляются в мониторинге (/monitor)
и в отчётах кошельков.

Семантика находок (совместима с solana_scan.py):
  * исходящий перевод С известного плохого адреса — движение украденного:
    verdict SUSPICIOUS, индикатор F1_DOWNSTREAM_TRANSFER, victim="",
    hacker=получатель, exposed=[отправитель] (получатель становится
    suspicious, дерево отчёта связывает выплату с отправителем);
  * входящий перевод НА известный плохой адрес — финансирование оператора:
    verdict SUSPICIOUS, индикатор L1_WATCHED_INFLOW, victim=отправитель,
    hacker=известный адрес, exposed=[отправитель] (отправитель получает
    флаг associated_hacker).

Конфигурация: переменные окружения VAULN_API_URL и ADMIN_API_KEY либо
флаги --api-url/--api-key (см. parse_args).
"""

import argparse
import json
import os
import sys
import time
import urllib.error
import urllib.request

DEFAULT_API_URL = os.environ.get("VAULN_API_URL", "http://127.0.0.1:8080")
ADMIN_API_KEY = os.environ.get("ADMIN_API_KEY", "")

# Массовая проверка ограничена 500 адресами на запрос на бэкенде.
BULK_BATCH = 400

# Статусы, движения которых попадают в мониторинг и отчёты. Только прямые
# угрозы: движения остальных статусов (vulnerable, scam, mixer, sanctioned,
# frozen, safe, exchange, unknown) находок не порождают — иначе лента
# мониторинга, ограниченная 10 последними находками, тонет в шуме.
BAD_STATUSES = {
    "hacked", "hacker", "drained", "phishing", "suspicious",
}

USER_AGENT = "vauln-liveblocksscan/1.0"


class BlockUnavailable(Exception):
    """Блок ещё не добыли / узел его не отдал — пропускаем без ретрая."""


# ------------------------------------------------------------- HTTP helpers

def http_json(url, payload=None, headers=None, timeout=30):
    """GET (payload=None) или POST с JSON-телом; ответ — распарсенный JSON."""
    hdrs = {"User-Agent": USER_AGENT, "Accept": "application/json"}
    if headers:
        hdrs.update(headers)
    data = None
    if payload is not None:
        data = json.dumps(payload).encode()
        hdrs["Content-Type"] = "application/json"
    req = urllib.request.Request(url, data=data, headers=hdrs)
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        return json.load(resp)


class JsonRpc:
    """JSON-RPC клиент с round-robin ротацией эндпоинтов и ретраями."""

    def __init__(self, endpoints, timeout=30, retries=3):
        if not endpoints:
            raise ValueError("нужен хотя бы один RPC endpoint")
        self.endpoints = list(endpoints)
        self.timeout = timeout
        self.retries = retries
        self.idx = 0
        self.req_id = 0

    def _next(self):
        ep = self.endpoints[self.idx % len(self.endpoints)]
        self.idx += 1
        return ep

    def call(self, method, params=None):
        self.req_id += 1
        body = {"jsonrpc": "2.0", "id": self.req_id,
                "method": method, "params": params or []}
        last_err = None
        for _ in range(self.retries * len(self.endpoints)):
            ep = self._next()
            try:
                data = http_json(ep, body, timeout=self.timeout)
                if data.get("error"):
                    raise RuntimeError(f"RPC error: {data['error']}")
                return data.get("result")
            except (urllib.error.URLError, OSError, RuntimeError,
                    json.JSONDecodeError) as e:
                last_err = e
                time.sleep(0.3)
        raise RuntimeError(f"все RPC endpoints недоступны: {last_err}")


# -------------------------------------------------------------- API клиент

class ApiClient:
    """Доступ к бэкенду vauln-address: массовая проверка и постинг находок."""

    def __init__(self, api_url, api_key):
        self.api_url = api_url.rstrip("/")
        self.api_key = api_key

    def bulk_check(self, chain, addresses):
        """Вернуть {address: result} для адресов, найденных в базе угроз."""
        found = {}
        for i in range(0, len(addresses), BULK_BATCH):
            batch = addresses[i:i + BULK_BATCH]
            try:
                data = http_json(
                    self.api_url + "/api/check/bulk",
                    {"chain": chain, "addresses": batch}, timeout=30)
            except Exception as e:  # noqa: BLE001 — пропуск батча не фатален
                print(f"  !! bulk check failed: {e}", file=sys.stderr)
                continue
            # бэкенд отвечает адресом в исходном регистре (checksummed);
            # для EVM ключи словаря приводим к нижнему регистру — RPC
            # отдаёт адреса lowercase
            evm = data.get("chain") == "evm"
            for res in data.get("results") or []:
                addr = res.get("address")
                if addr:
                    found[addr.lower() if evm else addr] = res
        return found

    def post_finding(self, payload):
        headers = {"X-Admin-Key": self.api_key} if self.api_key else {}
        try:
            return http_json(
                self.api_url + "/api/admin/scanner/findings",
                payload, headers=headers, timeout=15)
        except Exception as e:  # noqa: BLE001
            print(f"  !! не удалось отправить находку в API: {e}",
                  file=sys.stderr)
            return None


# ----------------------------------------------------------- ядро watcher'а

class Transfer:
    """Одно движение средств внутри блока."""

    __slots__ = ("tx", "sender", "recipient", "amount")

    def __init__(self, tx, sender, recipient, amount):
        self.tx = tx
        self.sender = sender or ""
        self.recipient = recipient or ""
        self.amount = amount or 0.0


def process_transfers(api, chain, height, transfers, posted):
    """Проверить участников переводов по базе и отправить находки.

    posted — set ключей "tx|sender|recipient" для дедупликации внутри прогона
    (бэкенд дополнительно дедупит по signature).
    """
    candidates = set()
    for t in transfers:
        if t.sender:
            candidates.add(t.sender)
        if t.recipient:
            candidates.add(t.recipient)
    if not candidates:
        return 0

    known = api.bulk_check(chain, sorted(candidates))
    known = {a: r for a, r in known.items()
             if r.get("status") in BAD_STATUSES}
    if not known:
        return 0

    def lookup(addr):
        # EVM-ключи уже lowercase (см. bulk_check); остальные сети — точное
        # совпадение, fallback на lower безопасен (ключи 0x-префиксные)
        return known.get(addr) or known.get(addr.lower())

    sent = 0
    for t in transfers:
        sender_hit = lookup(t.sender) if t.sender else None
        recipient_hit = lookup(t.recipient) if t.recipient else None
        sender_bad = sender_hit is not None
        recipient_bad = recipient_hit is not None
        if not (sender_bad or recipient_bad):
            continue
        if not t.recipient or t.recipient == t.sender:
            continue
        key = f"{t.tx}|{t.sender}|{t.recipient}"
        if key in posted:
            continue
        posted.add(key)

        if sender_bad:
            status = sender_hit["status"]
            # известную сторону отправляем в регистре из базы (checksummed),
            # чтобы отчёты и статусы сходились с записью реестра
            sender_db = sender_hit.get("address") or t.sender
            payload = {
                "chain": chain,
                "signature": t.tx,
                "slot": height,
                "verdict": "SUSPICIOUS",
                "indicators": ["F1_DOWNSTREAM_TRANSFER"],
                "victim_address": "",
                "hacker_address": t.recipient,
                "amount_sol": round(t.amount, 9),
                "programs": [],
                "exposed_addresses": [sender_db],
                "source": "live-blocks",
            }
            print(f"  >>> ДВИЖЕНИЕ блок {height}: {t.sender[:16]}… ({status})"
                  f" -> {t.recipient[:16]}…  {t.amount:.6f}"
                  f" [{t.tx[:20]}…]", flush=True)
        else:
            status = recipient_hit["status"]
            recipient_db = recipient_hit.get("address") or t.recipient
            payload = {
                "chain": chain,
                "signature": t.tx,
                "slot": height,
                "verdict": "SUSPICIOUS",
                "indicators": ["L1_WATCHED_INFLOW"],
                "victim_address": t.sender,
                "hacker_address": recipient_db,
                "amount_sol": round(t.amount, 9),
                "programs": [],
                "exposed_addresses": [t.sender],
                "source": "live-blocks",
            }
            print(f"  >>> ПОПОЛНЕНИЕ блок {height}: {t.sender[:16]}… ->"
                  f" {t.recipient[:16]}… ({status})  {t.amount:.6f}"
                  f" [{t.tx[:20]}…]", flush=True)
        resp = api.post_finding(payload)
        if resp is not None:
            sent += 1
    return sent


def run(name, chain, poll_interval, latest_fn, transfers_fn,
        api_url=None, api_key=None, once=False, start=None):
    """Главный цикл: latest_fn() -> номер головы, transfers_fn(h) -> [Transfer].

    Стартует от текущей головы (или от start) и идёт только вперёд.
    """
    api = ApiClient(api_url or DEFAULT_API_URL,
                    api_key if api_key is not None else ADMIN_API_KEY)
    posted = set()
    print(f"[i] {name}: API {api.api_url}, сеть {chain}, "
          f"интервал {poll_interval}s", flush=True)

    last = start - 1 if start is not None else None
    while True:
        try:
            head = latest_fn()
        except Exception as e:  # noqa: BLE001 — сеть моргнула, ждём
            print(f"  !! {name}: ошибка получения головы: {e}",
                  file=sys.stderr, flush=True)
            time.sleep(poll_interval)
            continue
        if last is None:
            last = head - 1
        while last < head:
            nxt = last + 1
            try:
                transfers = transfers_fn(nxt)
            except BlockUnavailable:
                last = nxt
                continue
            except Exception as e:  # noqa: BLE001
                print(f"  !! {name}: блок {nxt} недоступен: {e}",
                      file=sys.stderr, flush=True)
                break  # повторим тот же блок на следующем круге
            hits = process_transfers(api, chain, nxt, transfers, posted)
            if hits:
                print(f"[+] {name}: блок {nxt}: находок отправлено {hits}",
                      flush=True)
            last = nxt
            if len(posted) > 200_000:  # не раздуваем память на длинных прогонах
                posted.clear()
        if once:
            return
        time.sleep(poll_interval)


def parse_args(name, default_interval):
    ap = argparse.ArgumentParser(description=f"{name}: live-мониторинг блоков")
    ap.add_argument("--api-url", default=DEFAULT_API_URL,
                    help="URL бэкенда (env VAULN_API_URL)")
    ap.add_argument("--api-key", default=ADMIN_API_KEY,
                    help="админ-ключ (env ADMIN_API_KEY)")
    ap.add_argument("--interval", type=float, default=default_interval,
                    help="пауза между опросами головы, сек")
    ap.add_argument("--start", type=int, default=None,
                    help="стартовать с конкретного блока (по умолчанию — с головы)")
    ap.add_argument("--once", action="store_true",
                    help="обработать доступные блоки один раз и выйти")
    return ap.parse_args()
