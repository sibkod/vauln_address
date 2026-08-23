#!/usr/bin/env python3
"""
drainer_analyzer.py — детектор Solana-дрейнеров и анализатор их сетей.

Режимы:
  check-tx    <signature>  — проверить одну транзакцию на паттерн дрейнера
  quick-scan  <address>    — проверить последние N транзакций адреса на паттерн
  scan-wallet <address>    — полный анализ сети (входы/выходы/жертвы/операторы)

Детектируемые паттерны (работает для ЛЮБОГО дрейнера, не только известного):
  P1  system.assign аккаунта жертвы на не-system программу (перехват владения)
  P2  вывод почти всего баланса (>=90%) с остатком под rent (<0.01 SOL)
  P3  вызов неизвестной программы (не в белом списке) в той же транзакции
  P4  создание "контрольного" аккаунта (createAccount, owner=программа, space=0)
  P5  пересечение с базой известных дрейнер-программ
  P6  снос ВСЕХ SPL-токенов подписанта без компенсации (>=2 mint'ов в ноль,
      SOL не вырос) — отсекает легитимные полные свапы одного токена

Вердикты: CLEAN (нет индикаторов), SUSPICIOUS (P2..P6 без захвата — контекст,
может быть легитимным переводом/депозитом/свапом), DRAINER (только P1 —
захват on-curve кошелька неизвестной программой).

Трейсинг пути средств (scan-wallet): после выявления кошельков операторов
их исходящие SOL-переводы считаются распределением украденного. Получатели
классифицируются:
  F1  1 перевод от оператора    — SUSPICIOUS (возможный сообщник)
  F2  2+ перевода от операторов — hacker (соучастник/другой хакер);
      такие кошельки раскрываются рекурсивно (--trace-depth)
Кошельки бирж/обменников (solana_exchanges.json, --exchanges-file / env
SOLANA_EXCHANGES_FILE) — точки кэшаута: цепочка на них заканчивается.
Программы и PDA (off-curve) — не кошельки, в метки не идут.

Белый список известных программ (~300 programId: DEX, лендинг, NFT,
инфраструктура) подгружается из solana_programs.json рядом со скриптом
(флаг --programs-file / env SOLANA_PROGRAMS_FILE) — без него P3 шумит.

Режимы watch и scan-wallet с флагом --api-url (env VAULN_API_URL) отправляют
каждую находку (victim + hacker) в БД через API бэкенда
(POST /api/admin/scanner/findings, заголовок X-Admin-Key = --api-key /
env ADMIN_API_KEY). Находки видны на странице живого мониторинга. Дубли
отсекаются дважды: локально по signature за прогон и на бэкенде
(scan_findings.signature UNIQUE).

Watch отслеживает не только кражи, но и ДВИЖЕНИЕ украденного: исходящие
переводы с хакерских кошельков. Множество сеется из БД бэкенда
(GET /api/admin/wallets?status=hacker) и --hacker-file, пополняется на лету
(оператор DRAINER-находки и F2-получатель сразу под наблюдением) — цепочка
увода средств раскручивается рекурсивно в реальном времени.

Многопоточность: загрузка транзакций, трейсинг потоков, отправка находок
в API и обработка блоков в watch (скользящее окно слотов, --window)
выполняются пулом потоков (--threads, по умолчанию 8) — рассчитано на
платные RPC с высоким rate limit (Helius и т.п., --rpc-url / env
SOLANA_RPC_URL, можно указать несколько эндпоинтов — round-robin). Для
публичных RPC: --threads 1 --rpc-delay 0.8.

Только stdlib. Кэширует транзакции в <cache_dir>/<address>.json
"""

import argparse
import datetime
import json
import os
import sys
import threading
import time
import urllib.request
from collections import Counter, defaultdict
from concurrent.futures import ThreadPoolExecutor

# PyNaCl — проверка Ed25519 (on-curve). libsodium crypto_core_ed25519_is_valid_point
# отвергает точки в малой подгруппе (y<8), тогда как Solana findProgramAddress их
# пропускает. Поэтому используем libsodium, но для y<8 (малая подгруппа) считаем
# адрес on-curve — в точности как on-curve в Solana.
try:
    import nacl.bindings as _nb
except ImportError:
    _nb = None


def _b58decode(s):
    """Base58 -> bytes. Для Solana-адресов (32 байта) результат всегда 32 байта."""
    alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
    n = 0
    for c in s:
        n = n * 58 + alphabet.index(c)
    body = n.to_bytes(max((n.bit_length() + 7) // 8, 1), "big") if s else b""
    # body может быть короче 32 (лидирующие нули) или длиннее (не 32-байтный адрес)
    pad = len(s) - len(s.lstrip("1"))
    out = b"\x00" * pad + body
    # Solana адрес — ровно 32 байта; обрезаем/дополняем
    return out[-32:].rjust(32, b"\x00")


def is_on_curve(addr):
    """True, если base58-адрес — точка на Ed25519 (реальный кошелёк с ключом).

    PDA-аккаунты программ лежат ВНЕ кривой — у них нет приватного ключа,
    это служебные адреса (пулы, эскроу). Участие off-curve адреса в sweep
    означает маршрутизацию ликвидности, а не кражу у кошелька.
    """
    if _nb is None:
        return True  # без PyNaCl фильтр отключён
    try:
        p = _b58decode(addr)
    except (ValueError, IndexError):
        return False
    if len(p) != 32:
        return False
    y = int.from_bytes(p, "little") & ((1 << 255) - 1)
    if y >= 2 ** 255 - 19:
        return False
    if y < 8:
        return True  # малая подгруппа — Solana считает on-curve
    try:
        return bool(_nb.crypto_core_ed25519_is_valid_point(p))
    except Exception:
        return False

RPC_ENDPOINTS = [
    "https://api.mainnet-beta.solana.com",
    "https://solana-rpc.publicnode.com",
]
HEADERS = {"Content-Type": "application/json", "User-Agent": "Mozilla/5.0"}

SYSTEM_PROGRAM = "11111111111111111111111111111111"
TOKEN_PROGRAMS = {
    "TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA",
    "TokenzQdBNbLqP5VEhdkAS6EPFLC1PHnBqCXEpPxuEb",
    "ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL",
}

# Легитимные программы — всё остальное считается "неизвестным кодом"
KNOWN_PROGRAMS = {
    SYSTEM_PROGRAM,
    "TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA",
    "TokenzQdBNbLqP5VEhdkAS6EPFLC1PHnBqCXEpPxuEb",
    "ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL",
    "ComputeBudget111111111111111111111111111111",
    "MemoSq4gqABAXKb96qnH8TysNcWxMyWCqXgDLGmfcHr",
    "Memo1UhkJRfHyvLMcVucJwxXeuD728EqVDDwQDxFMNo",
    "JUP6LkbZbjS1jKKwapdHNy74zcZ3tLUZoi5QNyVTaV4",
    "JUP2jxvXaqu7NQY1GmNF4m1vodw12LVXYxbFL2uJvfo",
    "JUP4Fb2cqiRUcaTHdrPC8h2gNsA2ETXiPDD33WcGuJB",
    "goonuddtQRrWqqn5nFyczVKaie28f3kDkHWkHtURSLE",  # Jupiter event authority
    "675kPX9MHTjS2zt1qfr1NYHuzeLXfQM9H24wFSUt1Mp8",
    "whirLbMiicVdio4qvUfM5KAg6Ct8VwpYzGff3uctyCc",
    "metaqbxxUerdq28cj1RbAWkYQm3ybzjb6a8bt518x1s",
    "BPFLoaderUpgradeab1e11111111111111111111111",
    "BPFLoader2111111111111111111111111111111111",
    "Vote111111111111111111111111111111111111111",
    "Stake11111111111111111111111111111111111111",
    "AddressLookupTab1e1111111111111111111111111",
    "6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P",  # pump.fun
}

# База известных дрейнер-программ (пополняется флагом --watch-program)
KNOWN_BAD_PROGRAMS = {
    "3wRre6bqgqBFfNUbdTaUGQSJ4vWFnbwe6QiWJgqzHiu7": "drainer-core (case 9ML9o4nY)",
    "syspv6Qe5BbK8GPEjLbMnmF9hZy7juAuePbuspciCFP": "account-takeover (case 9ML9o4nY)",
    # Cases из scanner-review 22.08.2026: вызываются рядом с каждым takeover
    # через syspv6… в связке с sweep-переводом на оператора 4QFiKg8e…
    "4PG6e97DLCn2PRN4ZMmTLg83jsetrDkvamr3JiXoiffa": "drainer-side program (takeover tx companion)",
    "J9kkSCMTwuXtANwDrTdbKCW6vey8pWySUMc7mGooTzTo": "drainer-side program (takeover tx companion)",
}
# EtrnLzgbS7nMMy5fbD42kXiUzGg8XQzJ972Xtk1cjWih убрана: это активный легитимный
# сервис/бот (не содержит takeover/transfer, балансы не меняются), попавший
# в watchlist по ошибке. P5 теперь требует подтверждения другим индикатором.


# Дрейнерный стиль: ровно 2 инструкции ComputeBudget + неизвестная программа.
# На миллионах реальных транзакций это редкая комбинация, а у дрейнера — постоянная.
COMPUTE_BUDGET = "ComputeBudget111111111111111111111111111111"

# Известные кошельки бирж и обменных сервисов (горячие/холодные) — точки
# кэшаута, куда операторы выводят украденное. Перевод на такой адрес — конец
# отслеживаемой цепочки, а не сообщник. Пополняется из solana_exchanges.json
# (--exchanges-file / env SOLANA_EXCHANGES_FILE).
KNOWN_EXCHANGES = {
    "5tzFkiKscXHK5ZXCGbXZxdw7gTjjD1mBwuoFbhUvuAi9": "Binance (hot wallet)",
    "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM": "Binance (cold wallet)",
    "AC5RDfQFmDS1deWZos921JfqscXdByf8BKHs5ACWjtW2": "Bybit",
    "H8sMJSCQxfKiFTCfDR3DUMLPwcRbM61LGFJ8N4dK3WjS": "Coinbase",
    "2AQdpHJ2JpcEgPiATUXjQxA8QmafFegfQwSLWSprPicn": "Coinbase 2",
    "6FEVkH17P9y8Q9aCkDdPcMDjvj7SVxrTETaYEm8f51Jy": "Crypto.com",
    "AobVSwdW9BbpMdJvTqeCN4hPAmh4rHm7vwLnQ5ATSyrS": "Crypto.com 2",
    "u6PJ8DtQuPFnfmwHbGFULQ4u4EgjDiyYKjVEsynXq2w": "Gate.io",
    "5VCwKtCXgCJ6kit5FybXjvriW3xELsFDhYrPSqtJNmcD": "OKX",
    "88xTWZMeKfiTgbfEmPLdsUCQcZinwUfk25EBQZ21XMAZ": "HTX (Huobi)",
    "FWznbcNXWQuHTawe9RxvQ2LdCENssh12dsznf4RiouN5": "Kraken",
    "ASTyfSima4LLAdDgoFGkgqoKowG1LZFDr9fAQrg7iaJZ": "MEXC",
}

# Пыль ниже этого порога в исходящих потоках операторов игнорируется.
TRACE_MIN_SOL = 0.001


DEFAULT_PROGRAMS_FILE = os.path.join(
    os.path.dirname(os.path.abspath(__file__)), "solana_programs.json")
DEFAULT_EXCHANGES_FILE = os.path.join(
    os.path.dirname(os.path.abspath(__file__)), "solana_exchanges.json")


def load_known_programs(path):
    """Догружает белый список известных программ из JSON (address -> name).

    solana_programs.json содержит ~300 programId известных сервисов
    (DEX, лендинг, NFT-маркетплейсы, инфраструктура) — без него
    P3_UNKNOWN_PROGRAM даёт много ложных срабатываний.
    """
    try:
        with open(path) as f:
            data = json.load(f)
        programs = data.get("programs", data)
        added = 0
        for addr in programs:
            addr = addr.strip()
            if addr and addr not in KNOWN_PROGRAMS:
                KNOWN_PROGRAMS.add(addr)
                added += 1
        print(f"[i] белый список программ: {len(KNOWN_PROGRAMS)} "
              f"(+{added} из {path})")
    except FileNotFoundError:
        print(f"[!] файл белого списка не найден: {path} "
              f"(используется встроенный, {len(KNOWN_PROGRAMS)} программ)",
              file=sys.stderr)
    except Exception as e:
        print(f"[!] ошибка загрузки {path}: {e}", file=sys.stderr)


def load_known_exchanges(path):
    """Догружает кошельки бирж/обменников из JSON (address -> name).

    Файл опционален: без него работает встроенный список KNOWN_EXCHANGES.
    """
    try:
        with open(path) as f:
            data = json.load(f)
        exchanges = data.get("exchanges", data)
        added = 0
        for addr, name in exchanges.items():
            addr = addr.strip()
            if addr and addr not in KNOWN_EXCHANGES:
                KNOWN_EXCHANGES[addr] = str(name)
                added += 1
        print(f"[i] известные биржи/обменники: {len(KNOWN_EXCHANGES)} "
              f"(+{added} из {path})")
    except FileNotFoundError:
        pass  # файл опционален — встроенного списка достаточно
    except Exception as e:
        print(f"[!] ошибка загрузки {path}: {e}", file=sys.stderr)


# ---------------------------------------------------------- API integration

def extract_parties(details, signer_list):
    """Извлекает (victim, hacker) из результата detect_patterns.

    victim  — аккаунт, захваченный assign, либо источник sweep-перевода.
    hacker  — назначение sweep-перевода, новый владелец аккаунта (программа),
              либо подписант транзакции, не совпадающий с жертвой.
    """
    victim, hacker = "", ""
    if details["takeovers"]:
        victim = details["takeovers"][0].get("account") or ""
    if details["drain_transfers"]:
        # при дробном выводе оператор — получатель крупнейшего перевода
        sweep = max(details["drain_transfers"],
                    key=lambda d: d.get("amount_sol") or 0)
        if not victim:
            victim = sweep.get("from") or ""
        hacker = sweep.get("to") or ""
    if not hacker and details["takeovers"]:
        hacker = details["takeovers"][0].get("new_owner") or ""
    if not hacker:
        for s in signer_list:
            if s and s != victim:
                hacker = s
                break
    return victim, hacker


def post_finding(api_url, api_key, payload):
    """Отправляет находку в API бэкенда (/api/admin/scanner/findings)."""
    url = api_url.rstrip("/") + "/api/admin/scanner/findings"
    headers = dict(HEADERS)
    if api_key:
        headers["X-Admin-Key"] = api_key
    try:
        req = urllib.request.Request(
            url, data=json.dumps(payload).encode(), headers=headers)
        with urllib.request.urlopen(req, timeout=15) as resp:
            return json.load(resp)
    except Exception as e:  # noqa: BLE001
        print(f"  !! не удалось отправить находку в API: {e}", flush=True)
        return None


DEFAULT_HACKERS_FILE = os.path.join(
    os.path.dirname(os.path.abspath(__file__)), "solana_hackers.json")


def fetch_hacker_wallets(api_url, api_key, chain="solana", limit=50000):
    """Известные хакерские кошельки из БД бэкенда (GET /api/admin/wallets).

    Сид watch-множества: перезапущенный watch продолжает следить за всеми
    операторами, найденными за всё время (scan-wallet, watch, flow-trace),
    а не только за теми, кого он увидит в текущей сессии.
    """
    url = (api_url.rstrip("/") + "/api/admin/wallets?chain=" + chain
           + "&status=hacker&limit=" + str(limit))
    headers = dict(HEADERS)
    if api_key:
        headers["X-Admin-Key"] = api_key
    try:
        req = urllib.request.Request(url, headers=headers)
        with urllib.request.urlopen(req, timeout=15) as resp:
            data = json.load(resp)
        return [a for a in data.get("addresses") or [] if a]
    except Exception as e:  # noqa: BLE001
        print(f"[!] не удалось загрузить хакерские кошельки из API: {e}",
              file=sys.stderr)
        return []


def load_hacker_file(path):
    """Локальный список хакерских кошельков (JSON: [...] или {"hackers": [...]})."""
    try:
        with open(path) as f:
            data = json.load(f)
        hackers = data.get("hackers", data) if isinstance(data, dict) else data
        out = [a.strip() for a in hackers if isinstance(a, str) and a.strip()]
        if out:
            print(f"[i] хакерских кошельков из файла: {len(out)} ({path})")
        return out
    except FileNotFoundError:
        return []  # файл опционален — сидит основной список из БД
    except Exception as e:  # noqa: BLE001
        print(f"[!] ошибка загрузки {path}: {e}", file=sys.stderr)
        return []


class FindingPoster:
    """Параллельная отправка находок в API с дедупликацией по signature.

    Одна и та же транзакция может всплыть несколько раз за прогон
    (несколько жертв в одной tx в scan-wallet, совпадающие последние
    подписи у downstream-находок, повторный блок в watch) — повторные
    POST'ы бессмысленны и под параллельной отправкой гоняются за UNIQUE
    constraint на бэкенде. Дубли режем локально; бэкенд дополнительно
    дедупит по scan_findings.signature UNIQUE между прогонами.
    """

    def __init__(self, api_url, api_key, threads=8):
        self.api_url = api_url
        self.api_key = api_key
        self.threads = max(1, threads)
        self._seen = set()
        self._lock = threading.Lock()

    def submit_all(self, payloads):
        """Отправляет payload'ы пулом потоков; дубли по signature скипаются.

        Возвращает (responses, пропущено_дублей): responses — список ответов
        API (None при ошибке отправки) в порядке уникальных payload'ов.
        """
        unique = []
        skipped = 0
        for p in payloads:
            sig = p.get("signature") or ""
            with self._lock:
                if sig in self._seen:
                    skipped += 1
                    continue
                self._seen.add(sig)
            unique.append(p)
        if not unique:
            return [], skipped
        with ThreadPoolExecutor(max_workers=self.threads) as pool:
            responses = list(pool.map(
                lambda p: post_finding(self.api_url, self.api_key, p),
                unique))
        return responses, skipped


def finding_programs(details, takeovers=None):
    """Program id'ы, которые бэкенд обязан считать программами, а не
    кошельками: вызванные неизвестные программы + программы, ставшие новым
    владельцем захваченного аккаунта (assign не вызывает программу-владельца,
    поэтому в unknown_programs её может не быть, а extract_parties ставит её
    как hacker)."""
    progs = set(details.get("unknown_programs") or [])
    for t in (takeovers if takeovers is not None
              else details.get("takeovers") or []):
        owner = t.get("new_owner")
        if owner:
            progs.add(owner)
    return sorted(progs)


def sweep_breakdown(drain_transfers):
    """Разбивка sweep-переводов по получателям (поле sweeps находки).

    Дрейнеры дробят вывод на несколько получателей в одной транзакции;
    hacker_address — только крупнейший из них. Суммируем переводы по
    адресу назначения, крупнейший получатель идёт первым.
    """
    by_to = {}
    for d in drain_transfers or []:
        to = d.get("to") or ""
        if not to:
            continue
        by_to[to] = by_to.get(to, 0.0) + (d.get("amount_sol") or 0)
    return [{"address": to, "amount_sol": round(amt, 9)}
            for to, amt in sorted(by_to.items(), key=lambda kv: -kv[1])]


def make_finding_payload(signature, slot, verdict, indicators, details,
                         signer_list, source):
    victim, hacker = extract_parties(details, signer_list)
    return {
        "chain": "solana",
        "signature": signature,
        "slot": slot,
        "verdict": verdict,
        "indicators": indicators,
        "victim_address": victim,
        "hacker_address": hacker,
        "amount_sol": round(sum(d.get("amount_sol") or 0
                                for d in details["drain_transfers"]), 9),
        "sweeps": sweep_breakdown(details["drain_transfers"]),
        "programs": finding_programs(details),
        "source": source,
    }


class Rpc:
    """RPC-клиент с ротацией эндпоинтов, ретраями и пулом потоков.

    Потокобезопасен: ротация эндпоинтов под локом, счётчики прогресса —
    под отдельным локом. Параллелизм рассчитан на платные RPC (Helius и
    т.п. с высоким rate limit); для публичных узлов threads=1 + delay 0.8
    воспроизводит старое последовательное поведение.
    """

    def __init__(self, endpoints=None, timeout=60, threads=8, delay=0.0):
        self.endpoints = list(endpoints or RPC_ENDPOINTS)
        self.ep_idx = 0
        self.timeout = timeout
        self.threads = max(1, threads)
        self.delay = max(0.0, delay)
        self._ep_lock = threading.Lock()
        self._print_lock = threading.Lock()

    def _next_endpoint(self):
        with self._ep_lock:
            ep = self.endpoints[self.ep_idx % len(self.endpoints)]
            self.ep_idx += 1
            return ep

    def call(self, method, params, retries=6):
        last_err = None
        for attempt in range(retries):
            ep = self._next_endpoint()
            try:
                if self.delay:
                    time.sleep(self.delay)
                payload = {"jsonrpc": "2.0", "id": 1, "method": method, "params": params}
                req = urllib.request.Request(ep, data=json.dumps(payload).encode(), headers=HEADERS)
                r = json.load(urllib.request.urlopen(req, timeout=self.timeout))
                # часть публичных узлов не держит историю и отдаёт result: null
                # вместо ошибки — считаем это отказом и переключаемся
                if r.get("error") or (method == "getTransaction" and r.get("result") is None):
                    raise RuntimeError(f"RPC {method} on {ep}: {r.get('error') or 'null result'}")
                return r
            except Exception as e:  # noqa: BLE001
                last_err = e
                time.sleep(1.5 + attempt * 1.5)
        raise RuntimeError(f"RPC {method} failed after {retries} tries: {last_err}")

    def get_slot(self):
        return self.call("getSlot", [{"commitment": "confirmed"}])["result"]

    def get_block(self, slot, details="signatures", encoding="jsonParsed"):
        r = self.call("getBlock", [
            slot,
            {"encoding": encoding, "transactionDetails": details,
             "rewards": False, "maxSupportedTransactionVersion": 0},
        ])
        err = r.get("error") or {}
        if err.get("code") == -32004:  # блок недоступен — не ошибка, просто лаг
            return None
        return r.get("result")

    def get_signatures(self, address, limit=None):
        """Все подписи адреса (или до limit), новые первыми.

        Пагинация по курсору `before` по своей природе последовательна —
        параллелится только загрузка самих транзакций.
        """
        out, before = [], None
        while True:
            page = 1000 if limit is None else min(1000, limit - len(out))
            if page <= 0:
                break
            params = [address, {"limit": page}]
            if before:
                params[1]["before"] = before
            r = self.call("getSignaturesForAddress", params).get("result") or []
            out.extend(r)
            if len(r) < page:
                break
            before = r[-1]["signature"]
        return out

    def _fetch_tx(self, s):
        try:
            r = self.call(
                "getTransaction",
                [s, {"encoding": "jsonParsed",
                     "maxSupportedTransactionVersion": 0}])
            return s, r.get("result")
        except Exception:  # noqa: BLE001
            return s, None

    def get_transactions(self, signatures, progress=False):
        """Выкачивает транзакции пулом потоков (по одной на запрос — батчи
        не используем, чтобы не зависеть от лимитов конкретного узла)."""
        result = {}
        missing = list(dict.fromkeys(signatures))  # дубли подписей не качаем
        total = len(missing)
        done = [0]

        def tracked(s):
            sig, tx = self._fetch_tx(s)
            if progress:
                with self._print_lock:
                    done[0] += 1
                    if done[0] % 40 == 0 or done[0] == total:
                        print(f"  txs: {done[0]}/{total}", flush=True)
            return sig, tx

        with ThreadPoolExecutor(max_workers=self.threads) as pool:
            for sig, tx in pool.map(tracked, missing):
                result[sig] = tx
        # вторая попытка для null'ов — первый прогон мог упасть по rate limit
        nulls = [s for s, v in result.items() if not v]
        if nulls:
            time.sleep(5)
            with ThreadPoolExecutor(max_workers=self.threads) as pool:
                for sig, tx in pool.map(self._fetch_tx, nulls):
                    if tx:
                        result[sig] = tx
        return result


# ------------------------------------------------------- pattern detection

def iter_instructions(tx):
    """Все инструкции (верхний уровень + inner)."""
    groups = list(tx["transaction"]["message"].get("instructions") or [])
    for inner in (tx["meta"].get("innerInstructions") or []):
        if inner:
            groups.extend(inner.get("instructions") or [])
    return [ix for ix in groups if ix]


def account_keys(tx):
    keys = tx["transaction"]["message"].get("accountKeys") or []
    return [k["pubkey"] if isinstance(k, dict) else k for k in keys]


def signers(tx):
    return [k["pubkey"] for k in tx["transaction"]["message"].get("accountKeys") or []
            if isinstance(k, dict) and k.get("signer")]


def detect_patterns(tx, watch_programs=None):
    """
    Возвращает (verdict, indicators, details).
    verdict: DRAINER | SUSPICIOUS | CLEAN.
    """
    watch = dict(KNOWN_BAD_PROGRAMS)
    for p in (watch_programs or []):
        watch.setdefault(p, "user-watchlist")

    indicators = []
    details = {"takeovers": [], "drain_transfers": [], "unknown_programs": [],
               "created_control_accounts": [], "bad_program_hits": [],
               "token_sweeps": []}

    meta = tx.get("meta") or {}
    pre = meta.get("preBalances") or []
    post = meta.get("postBalances") or []
    key_list = account_keys(tx)

    # P6: снос ВСЕХ SPL-токенов подписанта без компенсации — классика дрейнера.
    # Один токен, ушедший в ноль (продали через свап), — норма, не флагаем:
    # требуем >=2 разных mint'ов и что подписант не получил SOL взамен.
    pre_tok_by_owner, post_tok_by_owner, pre_mints_by_owner = {}, {}, {}
    for b in (meta.get("preTokenBalances") or []):
        owner = b.get("owner")
        ui = (b.get("uiTokenAmount") or {}).get("uiAmount") or 0
        if owner and ui:
            pre_tok_by_owner[owner] = pre_tok_by_owner.get(owner, 0) + ui
            pre_mints_by_owner.setdefault(owner, set()).add(b.get("mint"))
    for b in (meta.get("postTokenBalances") or []):
        owner = b.get("owner")
        ui = (b.get("uiTokenAmount") or {}).get("uiAmount") or 0
        if owner:
            post_tok_by_owner[owner] = post_tok_by_owner.get(owner, 0) + ui

    signer_set = set(signers(tx))
    sweep_by_src = {}
    for owner, pre_ui in pre_tok_by_owner.items():
        if owner not in signer_set or post_tok_by_owner.get(owner, 0) != 0:
            continue
        mints = pre_mints_by_owner.get(owner) or set()
        if len(mints) < 2:
            continue  # полный свап одного токена — легитимная операция
        sol_delta = None
        if owner in key_list:
            i = key_list.index(owner)
            if i < len(pre) and i < len(post):
                sol_delta = post[i] - pre[i]
        if sol_delta is not None and sol_delta > 20_000_000:  # получил >0.02 SOL — это свап
            continue
        details["token_sweeps"].append({
            "owner": owner,
            "tokens_before": round(pre_ui, 6),
            "mints_swept": len(mints)})

    for ix in iter_instructions(tx):
        if ix.get("program") == "system":
            p = ix.get("parsed") or {}
            info = p.get("info") or {}
            itype = p.get("type")

            # P1: захват кошелька — assign РЕАЛЬНОГО (on-curve) аккаунта на
            # НЕИЗВЕСТНУЮ программу. PDA (off-curve) назначают программам
            # постоянно (Helium TukTuk и т.п.) — это не захват кошелька.
            if (itype == "assign"
                    and info.get("owner") != SYSTEM_PROGRAM
                    and info.get("owner") not in KNOWN_PROGRAMS
                    and is_on_curve(info.get("account") or "")):
                details["takeovers"].append(
                    {"account": info.get("account"), "new_owner": info.get("owner")})

            # P4: контрольный аккаунт, принадлежащий программе
            if (itype == "createAccount"
                    and info.get("owner") not in ({SYSTEM_PROGRAM} | TOKEN_PROGRAMS)
                    and int(info.get("space") or 0) == 0):
                details["created_control_accounts"].append(
                    {"account": info.get("newAccount"), "owner": info.get("owner")})

            # P2: исходящие переводы >=90% баланса суммарно, остаток <0.01 SOL.
            # Дрейнеры дробят вывод на несколько переводов (в т.ч. через CPI),
            # чтобы каждый по отдельности не достигал порога — поэтому суммируем
            # по источнику. Off-curve участник (PDA: пул, эскроу, служебный
            # аккаунт программы) — это маршрутизация ликвидности, не кража.
            if itype == "transfer":
                src = info.get("source")
                dst = info.get("destination")
                lamports = int(info.get("lamports") or 0)
                if (src in key_list and is_on_curve(src) and is_on_curve(dst)
                        and lamports > 1_000_000):
                    st = sweep_by_src.setdefault(src, {"lamports": 0, "transfers": []})
                    st["lamports"] += lamports
                    st["transfers"].append(
                        {"from": src, "to": dst, "amount_sol": lamports / 1e9})

    # агрегированный P2: сумма переводов источника против его баланса
    for src, st in sweep_by_src.items():
        idx = key_list.index(src)
        if idx >= len(pre) or idx >= len(post) or pre[idx] <= 0:
            continue
        ratio = st["lamports"] / pre[idx]
        if ratio >= 0.9 and post[idx] < 10_000_000:
            for t in st["transfers"]:
                t["left_sol"] = post[idx] / 1e9
                t["ratio"] = round(ratio, 4)
                details["drain_transfers"].append(t)

    # P3: вызванные программы вне белого списка
    unknown = set()
    for ix in iter_instructions(tx):
        pid = ix.get("programId")
        if pid and pid not in KNOWN_PROGRAMS:
            unknown.add(pid)
    details["unknown_programs"] = sorted(unknown)
    details["bad_program_hits"] = sorted(
        {p: watch[p] for p in unknown if p in watch}.items())

    if details["takeovers"]:
        indicators.append("P1_ACCOUNT_TAKEOVER")
    if details["drain_transfers"]:
        indicators.append("P2_FULL_BALANCE_SWEEP")
    if unknown:
        indicators.append("P3_UNKNOWN_PROGRAM")
    if details["created_control_accounts"]:
        indicators.append("P4_CONTROL_ACCOUNT")
    if details["bad_program_hits"]:
        indicators.append("P5_KNOWN_DRAINER_PROGRAM")
    if details["token_sweeps"]:
        indicators.append("P6_TOKEN_SWEEP")

    if "P1_ACCOUNT_TAKEOVER" in indicators:
        verdict = "DRAINER"
    elif any(i in indicators for i in
             ("P2_FULL_BALANCE_SWEEP", "P4_CONTROL_ACCOUNT",
              "P5_KNOWN_DRAINER_PROGRAM", "P6_TOKEN_SWEEP")):
        # sweep/контрольный аккаунт/watchlist без захвата — контекст,
        # не доказательство; одиночный P3 (неизвестная программа) — шум
        verdict = "SUSPICIOUS"
    else:
        verdict = "CLEAN"
    return verdict, indicators, details


# ------------------------------------------------------------- report utils

def fmt_time(ts):
    if not ts:
        return "?"
    return datetime.datetime.fromtimestamp(ts, datetime.timezone.utc).strftime(
        "%Y-%m-%d %H:%M:%S UTC")


def print_tx_verdict(sig, tx, watch_programs=None):
    verdict, indicators, details = detect_patterns(tx, watch_programs)
    print(f"\n{'=' * 90}")
    print(f"TX: {sig}")
    print(f"    time    : {fmt_time(tx.get('blockTime'))}")
    print(f"    signers : {', '.join(signers(tx))}")
    print(f"    VERDICT : {verdict}  [{', '.join(indicators) or 'no indicators'}]")
    for t in details["takeovers"]:
        print(f"    !! TAKEOVER: {t['account']}  -> owner={t['new_owner']}")
    for d in details["drain_transfers"]:
        print(f"    !! SWEEP  : {d['from']} -> {d['to']}  {d['amount_sol']:.6f} SOL"
              f" ({d['ratio'] * 100:.1f}% баланса, остаток {d['left_sol']:.6f})")
    for p, name in details["bad_program_hits"]:
        print(f"    !! KNOWN DRAINER PROGRAM: {p} ({name})")
    unk = [p for p in details["unknown_programs"] if p not in KNOWN_BAD_PROGRAMS]
    if unk:
        print(f"    ?? unknown programs: {', '.join(unk)}")
    return verdict


# ------------------------------------------------------------------- modes

def prefilter_block_tx(tx, watch_programs):
    """Быстрый фильтр по транзакции внутри блока (без полного разбора).

    Возвращает True, если транзакция потенциально дрейнерная:
      - вызывает программу из watchlist, ИЛИ
      - ровно 2 инструкции ComputeBudget + хотя бы одна неизвестная программа.
    """
    ixs = tx.get("transaction", {}).get("message", {}).get("instructions") or []
    progs = [ix.get("programId") for ix in ixs if ix.get("programId")]
    if any(p in watch_programs for p in progs):
        return True
    cb = sum(1 for p in progs if p == COMPUTE_BUDGET)
    if cb == 2:
        unknown = [p for p in progs if p not in KNOWN_PROGRAMS and p != COMPUTE_BUDGET]
        if unknown:
            return True
    return False


def mode_watch(rpc, watch_programs, full_blocks=False, out_file=None,
               stats_every=20, start_slot=None, api_url=None, api_key=None,
               window=None, hacker_file=None):
    """Живой мониторинг блоков Solana на паттерн дрейнера.

    Стратегия по умолчанию (лёгкая):
      1. getBlock(full) по слоту, prefilter транзакций
      2. detect_patterns — полный разбор только подозрительных
    С --full-blocks: анализ всех транзакций блока (без prefilter).
    С --api-url: каждая находка отправляется в БД через API бэкенда.

    Кроме краж (DRAINER-паттерн) watch отслеживает ДВИЖЕНИЕ украденного:
    исходящие переводы с известных хакерских кошельков. Множество сеется
    из БД бэкенда (GET /api/admin/wallets?status=hacker — все операторы,
    найденные scan-wallet/watch/flow-trace за всё время) и из
    --hacker-file, и пополняется на лету: оператор каждой DRAINER-находки
    и получатель F2-перевода сразу попадают под наблюдение, поэтому цепочка
    раскручивается рекурсивно в реальном времени. Классификация получателя
    как в flow-trace: 1 перевод — SUSPICIOUS (F1), 2+ — hacker (F2).

    Блоки обрабатываются скользящим окном: --threads воркеров параллельно
    забирают следующие слоты (round-robin по эндпоинтам --rpc-url), анализ
    и отправка находок идут по мере готовности блока — на нескольких
    эндпоинтах (Helius + ещё 2) сканер держит темп сети ~2.5 блока/с.
    Окно ограничено (--window, по умолчанию 4*threads): дальние слоты не
    стартуют, пока не завершены ближние — серия пропущенных/недоступных
    блоков не разгоняет курсор в тысячи слотов вперёд.
    """
    watch = dict(KNOWN_BAD_PROGRAMS)
    for p in (watch_programs or []):
        watch.setdefault(p, "user-watchlist")

    window = window or max(4, 4 * rpc.threads)
    print("LIVE-режим. Мониторим блоки на паттерн дрейнера.")
    print(f"  watchlist программ: {len(watch)} ({', '.join(list(watch)[:3])}...)")
    print(f"  стратегия: {'FULL blocks (все транзакции)' if full_blocks else 'prefilter (signatures->tx)'}")
    if out_file:
        print(f"  находки пишем в: {out_file}")
    if api_url:
        print(f"  находки отправляем в API: {api_url}")
    print("  Ctrl+C для остановки.\n")

    slot = start_slot or (rpc.get_slot() - 100)  # RPC не отдаёт свежие блоки сразу
    stats = Counter()
    log_fp = open(out_file, "a") if out_file else None
    poster = (FindingPoster(api_url, api_key, threads=rpc.threads)
              if api_url else None)
    t_start = time.time()

    # Множество хакерских кошельков под наблюдением (движение украденного):
    # сид из БД (все найденные ранее операторы) + локальный файл, далее
    # пополняется находками этой сессии. Под общим локом — воркеры
    # читают/пополняют его параллельно.
    hackers_lock = threading.Lock()
    hacker_wallets = set(load_hacker_file(hacker_file)) if hacker_file else set()
    if api_url:
        hacker_wallets.update(fetch_hacker_wallets(api_url, api_key))
    hacker_wallets = {a for a in hacker_wallets
                      if a and a not in KNOWN_PROGRAMS
                      and a not in KNOWN_EXCHANGES and is_on_curve(a)}
    movement_state = {}  # получатель -> {"txs": n, "sources": set}
    print(f"  хакерских кошельков под наблюдением: {len(hacker_wallets)}",
          flush=True)

    # Общее состояние окна. Курсор next выдаёт слоты воркерам; слот
    # считается обработанным после завершения process_slot. Стартовать
    # слоты дальше window от минимального незавершённого нельзя — иначе
    # серия пропущенных блоков уносит курсор далеко вперёд.
    state = {"next": slot, "min_inflight": None, "lag_notice_at": 0}
    inflight = set()
    state_lock = threading.Lock()
    slot_done = threading.Condition(state_lock)

    print(f"  стартовый слот: {slot} (RPC-лаг ~100 слотов)", flush=True)
    print(f"  параллелизм: {rpc.threads} воркеров, окно {window} слотов, "
          f"{len(rpc.endpoints)} RPC-эндпоинт(а)", flush=True)

    def record_finding(slot_, entry):
        line = json.dumps(entry, ensure_ascii=False)
        if log_fp:
            log_fp.write(line + "\n")
            log_fp.flush()
        print(f"\n  >>> НАХОДКА slot {slot_}: {entry['verdict']} {entry['signature'][:20]}..."
              f" [{', '.join(entry['indicators'])}]")
        for t in entry.get("takeovers", []):
            print(f"      TAKEOVER: {t['account']} -> {t['new_owner']}")
        for d in entry.get("sweeps", []):
            print(f"      SWEEP: {d['from']} -> {d['to']} {d['amount_sol']:.6f} SOL")
        if poster:
            hacker = entry.get("hacker", "")
            exposed = sorted({d["from"] for d in entry.get("sweeps", [])
                              if hacker and d.get("to") == hacker} |
                             {t["account"] for t in entry.get("takeovers", [])})
            responses, skipped = poster.submit_all([{
                "chain": "solana",
                "signature": entry["signature"],
                "slot": slot_,
                "verdict": entry["verdict"],
                "indicators": entry["indicators"],
                "victim_address": entry.get("victim", ""),
                "hacker_address": hacker,
                "amount_sol": round(sum(d.get("amount_sol") or 0
                                        for d in entry.get("sweeps", [])), 9),
                "sweeps": sweep_breakdown(entry.get("sweeps", [])),
                "programs": finding_programs(
                    {"unknown_programs": entry.get("unknown_programs", [])},
                    takeovers=entry.get("takeovers", [])),
                "exposed_addresses": exposed,
                "source": "watch",
            }])
            if skipped:
                print("      API: дубль signature за прогон, пропущен")
            elif responses and responses[0] is not None:
                resp = responses[0]
                print(f"      API: id={resp.get('id')} victim={entry.get('victim', '')[:12]}..."
                      f" hacker={entry.get('hacker', '')[:12]}...")

    def watch_hacker(addr):
        """Поставить кошелёк под наблюдение (движение украденного)."""
        if (addr and addr not in KNOWN_PROGRAMS
                and addr not in KNOWN_EXCHANGES and is_on_curve(addr)):
            with hackers_lock:
                hacker_wallets.add(addr)

    def record_movement(slot_, sig, moves):
        """Движение украденного с хакерского кошелька: классификация
        получателя (1 перевод — SUSPICIOUS/F1, 2+ — hacker/F2) и отправка
        находки. F2-получатель сразу попадает под наблюдение — цепочка
        раскручивается рекурсивно."""
        for m in moves:
            with hackers_lock:
                st = movement_state.setdefault(
                    m["to"], {"txs": 0, "sources": set()})
                st["txs"] += 1
                st["sources"].add(m["from"])
                repeat = st["txs"] >= 2 or len(st["sources"]) >= 2
                if repeat:
                    hacker_wallets.add(m["to"])
            stats["movements"] += 1
            label = "F2_REPEAT_DOWNSTREAM" if repeat else "F1_DOWNSTREAM_TRANSFER"
            print(f"\n  >>> ДВИЖЕНИЕ С ХАКЕРСКОГО slot {slot_}: "
                  f"{m['from'][:12]}... -> {m['to'][:12]}... "
                  f"{m['amount_sol']:.6f} SOL [{label}]", flush=True)
            if poster:
                poster.submit_all([{
                    "chain": "solana",
                    "signature": sig,
                    "slot": slot_,
                    "verdict": "DRAINER" if repeat else "SUSPICIOUS",
                    "indicators": [label],
                    # поток, а не дрейн: жертвы нет, бэкенд регистрирует
                    # только сторону хакера (получателя украденного)
                    "victim_address": "",
                    "hacker_address": m["to"],
                    "amount_sol": round(m["amount_sol"], 9),
                    "programs": [],
                    "exposed_addresses": [m["from"]],
                    "source": "watch",
                }])

    def process_slot(slot_):
        """Загрузка и анализ одного блока (выполняется в воркере пула)."""
        try:
            blk = rpc.get_block(slot_, details="full")
        except Exception as e:  # noqa: BLE001 — один слот не роняет монитор
            print(f"  block {slot_}: RPC-ошибка ({e}), пропуск", flush=True)
            stats["rpc_errors"] += 1
            return
        if blk is None:
            print(f"  block {slot_}: недоступен/пустой, пропуск", flush=True)
            stats["skipped"] += 1
            return
        txs = blk.get("transactions") or []
        stats["txs_scanned"] += len(txs)
        stats["blocks"] += 1
        elapsed = time.time() - t_start
        bps = stats["blocks"] / elapsed if elapsed > 0 else 0
        print(f"  block {slot_}: {len(txs)} tx "
              f"(всего {stats['blocks']} блок., {bps:.2f} блок/с, "
              f"chain ~2.5 блок/с)", flush=True)
        # снимок множества хакеров на блок: пополнение внутри блока
        # подхватывается со следующего
        with hackers_lock:
            hacker_snapshot = set(hacker_wallets)
        for tx in txs:
            if (tx.get("meta") or {}).get("err"):
                continue
            sig = (tx.get("transaction", {}).get("signatures") or [""])[0]
            # движение украденного: исходящие переводы с хакерских кошельков
            # видны независимо от prefilter — смотрим каждую транзакцию
            if hacker_snapshot:
                moves = hacker_movements(tx, hacker_snapshot)
                if moves:
                    record_movement(slot_, sig, moves)
            if not full_blocks and not prefilter_block_tx(tx, watch):
                continue
            stats["candidates"] += 1
            verdict, indicators, details = detect_patterns(tx, watch_programs)
            # В живом мониторинге показываем только DRAINER (P1 — полный
            # захват аккаунта). SUSPICIOUS (sweeps без захвата) — скипаем:
            # это обычные свапы/переводы.
            if verdict == "DRAINER":
                stats["drainer"] += 1
                victim, hacker = extract_parties(details, signers(tx))
                # оператор находки сразу под наблюдением: его дальнейшие
                # переводы — движение украденного
                watch_hacker(hacker)
                record_finding(slot_, {
                    "slot": slot_, "signature": sig, "verdict": verdict,
                    "indicators": indicators,
                    "victim": victim, "hacker": hacker,
                    "takeovers": details["takeovers"],
                    "sweeps": details["drain_transfers"],
                    "unknown_programs": details["unknown_programs"]})

    def claim_slot(head_):
        """Выдать воркеру следующий слот либо None (ждать голову/окно)."""
        with state_lock:
            nxt = state["next"]
            if nxt > head_:
                return None
            floor = state["min_inflight"]
            if floor is not None and nxt >= floor + window:
                return None
            # не отставать безнадёжно: лаг > 200 блоков — прыжок к голове
            if head_ - nxt > 200:
                if nxt > state["lag_notice_at"]:
                    print(f"  .. отставание {head_ - nxt} блоков, "
                          f"прыгаем к слоту {head_ - 5}", flush=True)
                    state["lag_notice_at"] = head_
                nxt = head_ - 5
                state["next"] = nxt
            state["next"] = nxt + 1
            inflight.add(nxt)
            if state["min_inflight"] is None or nxt < state["min_inflight"]:
                state["min_inflight"] = nxt
            return nxt

    def finish_slot(slot_):
        with slot_done:
            inflight.discard(slot_)
            if state["min_inflight"] == slot_:
                state["min_inflight"] = min(inflight) if inflight else None
            slot_done.notify_all()

    def worker():
        head = rpc.get_slot()
        while True:
            s = claim_slot(head)
            if s is None:
                with slot_done:
                    slot_done.wait(timeout=0.4)
                head = rpc.get_slot()
                continue
            try:
                process_slot(s)
            finally:
                finish_slot(s)
            if s >= head:
                head = rpc.get_slot()

    pool = ThreadPoolExecutor(max_workers=rpc.threads)
    try:
        futures = [pool.submit(worker) for _ in range(rpc.threads)]
        # любой необработанный exception в воркере — фатален для монитора
        for f in futures:
            f.result()
    except KeyboardInterrupt:
        print(f"\nОстановлено. Итог: {dict(stats)}")
    finally:
        pool.shutdown(wait=False, cancel_futures=True)
        if log_fp:
            log_fp.close()




def mode_check_tx(rpc, sig, watch_programs=None):
    tx = rpc.call("getTransaction",
                  [sig, {"encoding": "jsonParsed",
                         "maxSupportedTransactionVersion": 0}]).get("result")
    if not tx:
        print("Транзакция не найдена:", sig)
        return 1
    v = print_tx_verdict(sig, tx, watch_programs)
    return 2 if v == "DRAINER" else (1 if v == "SUSPICIOUS" else 0)


def mode_quick_scan(rpc, address, limit, watch_programs=None):
    print(f"Сканирую последние {limit} транзакций {address} ...")
    sigs = [s["signature"] for s in rpc.get_signatures(address, limit=limit)]
    txs = rpc.get_transactions(sigs, progress=True)
    stats = Counter()
    flagged = []
    for s in sigs:
        tx = txs.get(s)
        if not tx:
            continue
        verdict, indicators, details = detect_patterns(tx, watch_programs)
        stats[verdict] += 1
        if verdict != "CLEAN":
            flagged.append((s, tx, verdict, indicators, details))
    print(f"\nИтог: CLEAN={stats['CLEAN']}  SUSPICIOUS={stats['SUSPICIOUS']}"
          f"  DRAINER={stats['DRAINER']}")
    for s, tx, verdict, indicators, details in flagged:
        print_tx_verdict(s, tx, watch_programs)
    return flagged


def outgoing_transfers(tx, address, signature=""):
    """SOL-переводы, уходящие С адреса внутри транзакции (system transfer).

    Пыль (< TRACE_MIN_SOL) и self-transfer игнорируются.
    """
    outs = []
    for ix in iter_instructions(tx):
        if ix.get("program") != "system":
            continue
        p = ix.get("parsed") or {}
        if p.get("type") != "transfer":
            continue
        info = p.get("info") or {}
        if info.get("source") != address:
            continue
        dst = info.get("destination")
        amt = int(info.get("lamports") or 0) / 1e9
        if not dst or dst == address or amt < TRACE_MIN_SOL:
            continue
        outs.append({"to": dst, "amount_sol": amt, "tx": signature})
    return outs


def hacker_movements(tx, hackers):
    """Исходящие SOL-переводы с известных хакерских кошельков внутри tx.

    Увод украденного дальше по цепочке: получатель — новый узел сети.
    Биржи/программы/PDA-получатели пропускаются — как и в flow-trace, они
    не регистрируются как злоумышленники (кэшаут — конец цепочки).
    """
    outs = []
    for ix in iter_instructions(tx):
        if ix.get("program") != "system":
            continue
        p = ix.get("parsed") or {}
        if p.get("type") != "transfer":
            continue
        info = p.get("info") or {}
        src, dst = info.get("source"), info.get("destination")
        if src not in hackers or not dst or dst == src:
            continue
        amt = int(info.get("lamports") or 0) / 1e9
        if amt < TRACE_MIN_SOL:
            continue
        if (dst in KNOWN_EXCHANGES or dst in KNOWN_PROGRAMS
                or not is_on_curve(dst)):
            continue
        outs.append({"from": src, "to": dst, "amount_sol": amt})
    return outs


def trace_fund_flows(rpc, seeds, cache_dir, depth=1, max_txs=100, max_wallets=10):
    """BFS по исходящим SOL-потокам от кошельков операторов (полный путь средств).

    seeds — адреса операторов (назначения sweep-переводов / новые владельцы
    захваченных аккаунтов). Каждый получатель их исходящих переводов —
    соучастник или точка кэшаута; получатели с 2+ переводами (сообщники)
    раскрываются рекурсивно до глубины depth. Биржи (KNOWN_EXCHANGES),
    программы и PDA (off-curve) — терминальные узлы, дальше не идём.

    Возвращает список рёбер {from,to,level,sol,txs,signatures}.
    """
    visited = set()
    edges = []
    frontier = [a for a in dict.fromkeys(seeds)
                if a and a not in KNOWN_EXCHANGES][:max_wallets]
    level = 1  # уровень 0 — ребро victim -> operator из основного анализа

    def trace_one(src, lvl):
        """Исходящие потоки одного кошелька: (рёбра, кандидаты на раскрытие)."""
        print(f"  [trace] уровень {lvl}: исходящие потоки {src[:12]}...",
              flush=True)
        try:
            sigs, cache = load_wallet_txs(rpc, src, cache_dir, max_txs,
                                          verbose=False)
        except Exception as e:  # noqa: BLE001 — один узел не роняет трейс
            print(f"  [trace] !! не удалось загрузить {src[:12]}...: {e}")
            return [], []
        agg = {}
        for s in sigs:
            tx = cache.get(s["signature"])
            if not tx:
                continue
            for o in outgoing_transfers(tx, src, s["signature"]):
                a = agg.setdefault(o["to"], {"sol": 0.0, "txs": 0, "sigs": []})
                a["sol"] += o["amount_sol"]
                a["txs"] += 1
                a["sigs"].append(o["tx"])
        src_edges, expand = [], []
        for dst, a in agg.items():
            src_edges.append({"from": src, "to": dst, "level": lvl,
                              "sol": round(a["sol"], 6), "txs": a["txs"],
                              "signatures": a["sigs"]})
            # сообщник с 2+ переводами — раскрываем его исходящие дальше
            if (a["txs"] >= 2 and dst not in KNOWN_EXCHANGES
                    and dst not in KNOWN_PROGRAMS and is_on_curve(dst)):
                expand.append(dst)
        return src_edges, expand

    while frontier and level <= depth + 1:
        wave = [s for s in frontier
                if s not in visited and s not in KNOWN_EXCHANGES]
        visited.update(wave)
        nxt = []
        # кошельки одного уровня независимы — трейсим параллельно
        with ThreadPoolExecutor(max_workers=rpc.threads) as pool:
            for src_edges, expand in pool.map(
                    lambda s: trace_one(s, level), wave):
                edges.extend(src_edges)
                nxt.extend(expand)
        frontier = [d for d in dict.fromkeys(nxt)
                    if d not in visited][:max_wallets]
        level += 1
    return edges


def classify_downstream(edges):
    """Сводит рёбра потоков по получателю и выставляет метку.

    exchange   — кошелёк биржи/обменника (точка кэшаута, конец цепочки)
    program    — известная программа или off-curve адрес (PDA), не кошелёк
    hacker     — 2+ перевода от операторов сети: соучастник/другой хакер
    suspicious — 1 перевод от оператора: возможный сообщник
    """
    by_dst = {}
    for e in edges:
        d = by_dst.setdefault(e["to"], {"sol": 0.0, "txs": 0, "sources": set(),
                                        "sigs": []})
        d["sol"] += e["sol"]
        d["txs"] += e["txs"]
        d["sources"].add(e["from"])
        d["sigs"].extend(e["signatures"])
    out = []
    for dst, d in by_dst.items():
        entry = {"address": dst, "sol_received": round(d["sol"], 6),
                 "txs": d["txs"], "sources": sorted(d["sources"]),
                 "signatures": d["sigs"]}
        if dst in KNOWN_EXCHANGES:
            entry["label"] = "exchange"
            entry["service"] = KNOWN_EXCHANGES[dst]
        elif dst in KNOWN_PROGRAMS or not is_on_curve(dst):
            entry["label"] = "program"
        elif d["txs"] >= 2 or len(d["sources"]) >= 2:
            entry["label"] = "hacker"
        else:
            entry["label"] = "suspicious"
        out.append(entry)
    out.sort(key=lambda e: -e["sol_received"])
    return out


def load_wallet_txs(rpc, address, cache_dir, max_txs=None, verbose=True):
    """Подписи и транзакции адреса с дисковым кэшем <cache_dir>/<address>.json.

    Возвращает (sigs, cache): sigs — ответ getSignaturesForAddress (новые
    первыми), cache — {signature: tx}. Пустые записи кэша (null из-за
    сброшенных RPC-запросов прошлых прогонов) выбрасываются.
    """
    os.makedirs(cache_dir, exist_ok=True)
    cache_path = os.path.join(cache_dir, f"{address}.json")
    cache = {}
    if os.path.exists(cache_path):
        try:
            with open(cache_path) as f:
                cache = json.load(f)
        except Exception:  # noqa: BLE001 — битый кэш перекачиваем
            cache = {}
    stale = [k for k, v in cache.items() if not v]
    for k in stale:
        del cache[k]
    if stale and verbose:
        print(f"      выброшено пустых записей из кэша: {len(stale)}")
    sigs = rpc.get_signatures(address, limit=max_txs)
    missing = [s["signature"] for s in sigs if not cache.get(s["signature"])]
    if verbose:
        print(f"      в кэше: {len(sigs) - len(missing)}, к загрузке: {len(missing)}")
    if missing:
        fetched = rpc.get_transactions(missing, progress=verbose)
        cache.update(fetched)
        with open(cache_path, "w") as f:
            json.dump(cache, f)
    return sigs, cache


def mode_scan_wallet(rpc, address, cache_dir, watch_programs=None, max_txs=None,
                     api_url=None, api_key=None, trace_depth=1,
                     trace_wallets=10, trace_txs=100):
    cache_path = os.path.join(cache_dir, f"{address}.json")
    print(f"[1/3] Список транзакций {address} ...")
    print("[2/3] Загрузка транзакций (с кэшем) ...")
    sigs, cache = load_wallet_txs(rpc, address, cache_dir, max_txs)
    print(f"      всего подписей: {len(sigs)}")
    if sigs:
        print(f"      период: {fmt_time(sigs[-1]['blockTime'])}"
              f" -> {fmt_time(sigs[0]['blockTime'])}")

    print("[3/3] Анализ потоков и паттернов ...")
    in_flows, out_flows = Counter(), Counter()
    in_cnt = Counter()
    monthly = defaultdict(float)
    takeover_victims, takeover_details = set(), {}
    drainer_sources = {}  # signature -> set of senders funding the operator
    operators = Counter()  # operator (hacker) wallets: sweep destination / takeover new owner
    operator_sol = defaultdict(float)  # swept SOL received per operator
    takeover_owners = set()  # программы, ставшие владельцами захваченных аккаунтов
    unknown_progs = Counter()
    drainer_tx_count = 0
    theft_edges = {}  # (victim, operator) -> {sol, txs, sigs}: ребра уровня 0

    for s in sigs:
        tx = cache.get(s["signature"])
        if not tx:
            continue
        verdict, indicators, details = detect_patterns(tx, watch_programs)
        if verdict == "DRAINER":
            drainer_tx_count += 1
            sources = {d["from"] for d in details["drain_transfers"]
                       if d.get("to") == address}
            sources |= {t["account"] for t in details["takeovers"]}
            if sources:
                drainer_sources[s["signature"]] = sorted(sources)
        for d in details["drain_transfers"]:
            if d.get("to") and d["to"] != d.get("from"):
                operators[d["to"]] += 1
                operator_sol[d["to"]] += d.get("amount_sol") or 0
                key = (d["from"], d["to"])
                e = theft_edges.setdefault(key, {"sol": 0.0, "txs": 0, "sigs": []})
                e["sol"] += d.get("amount_sol") or 0
                e["txs"] += 1
                e["sigs"].append(s["signature"])
        for t in details["takeovers"]:
            takeover_victims.add(t["account"])
            takeover_details[t["account"]] = {"tx": s["signature"],
                                              "time": tx.get("blockTime")}
            if t.get("new_owner"):
                operators[t["new_owner"]] += 1
                takeover_owners.add(t["new_owner"])
        for p in details["unknown_programs"]:
            unknown_progs[p] += 1
        for ix in iter_instructions(tx):
            if ix.get("program") != "system":
                continue
            p = ix.get("parsed") or {}
            info = p.get("info") or {}
            if p.get("type") != "transfer":
                continue
            amt = int(info.get("lamports") or 0) / 1e9
            if info.get("destination") == address:
                in_flows[info["source"]] += amt
                in_cnt[info["source"]] += 1
                ts = tx.get("blockTime")
                if ts:
                    monthly[datetime.datetime.fromtimestamp(
                        ts, datetime.timezone.utc).strftime("%Y-%m")] += amt
            elif info.get("source") == address:
                out_flows[info["destination"]] += amt

    # Трейсинг исходящих потоков операторов: все исходящие с выявленных
    # кошельков операторов — это соучастники, другие хакеры или точки
    # кэшаута (биржи/обменники). 1 перевод получателю — SUSPICIOUS,
    # 2+ перевода — hacker; hacker-получатели раскрываются рекурсивно
    # до trace_depth уровней (полный путь движения средств).
    downstream, flow_edges = [], []
    # Программы (в т.ч. программа-владелец захваченных аккаунтов) не являются
    # кошельками операторов: их история — это чужие вызовы, а не потоки
    # средств. Сканируемый кошелёк трейсим, только если он сам получил sweep
    # (является оператором) — иначе его легитимные переводы попали бы в
    # монитор как "downstream".
    seeds = [a for a, _ in operators.most_common(trace_wallets)
             if a not in takeover_owners
             and a not in KNOWN_PROGRAMS and is_on_curve(a)]
    if operators.get(address) and address not in seeds:
        seeds.insert(0, address)
    if trace_depth >= 0 and seeds:
        print(f"[trace] трейсинг исходящих потоков: {len(seeds)} операторов, "
              f"глубина {trace_depth}, до {trace_txs} tx на кошелёк ...")
        flow_edges = trace_fund_flows(rpc, seeds, cache_dir, depth=trace_depth,
                                      max_txs=trace_txs,
                                      max_wallets=trace_wallets)
        downstream = classify_downstream(flow_edges)
    fund_flow = [{"from": v, "to": op, "level": 0,
                  "sol": round(e["sol"], 6), "txs": e["txs"],
                  "signatures": e["sigs"]}
                 for (v, op), e in sorted(theft_edges.items(),
                                          key=lambda kv: -kv[1]["sol"])]
    fund_flow.extend(flow_edges)

    report = {
        "target": address,
        "generated_utc": datetime.datetime.now(datetime.timezone.utc).isoformat(),
        "txs_analyzed": sum(1 for s in sigs if cache.get(s["signature"])),
        "period": {"first": fmt_time(sigs[-1]["blockTime"]) if sigs else None,
                   "last": fmt_time(sigs[0]["blockTime"]) if sigs else None},
        "totals": {"sol_in": round(sum(in_flows.values()), 6),
                   "sol_out": round(sum(out_flows.values()), 6),
                   "unique_sources": len(in_flows),
                   "unique_destinations": len(out_flows),
                   "drainer_pattern_txs": drainer_tx_count,
                   "hijacked_accounts": len(takeover_victims)},
        "monthly_inflow_sol": dict(sorted(monthly.items())),
        "top_sources": [{"address": a, "sol": round(v, 6), "txs": in_cnt[a],
                         "hijacked": a in takeover_victims}
                        for a, v in in_flows.most_common(50)],
        "top_destinations": [{"address": a, "sol": round(v, 6)}
                             for a, v in out_flows.most_common(50)],
        "hijacked_accounts": [
            {"address": a, "tx": takeover_details[a]["tx"],
             "time": fmt_time(takeover_details[a]["time"])}
            for a in sorted(takeover_victims)],
        "operator_wallets": [{"address": a, "sweep_txs": c,
                              "sol_received": round(operator_sol.get(a, 0.0), 6)}
                             for a, c in operators.most_common(50)],
        "unknown_programs_seen": [{"program": p, "tx_count": c}
                                  for p, c in unknown_progs.most_common(30)],
        "fund_flow": fund_flow,
        "downstream_wallets": [{k: v for k, v in e.items() if k != "signatures"}
                               for e in downstream],
        "cashout_points": [{"address": e["address"], "service": e["service"],
                            "sol_received": e["sol_received"], "txs": e["txs"],
                            "sources": e["sources"]}
                           for e in downstream if e["label"] == "exchange"],
    }

    out_path = os.path.join(cache_dir, f"{address}.report.json")
    json.dump(report, open(out_path, "w"), indent=2, ensure_ascii=False)

    t = report["totals"]
    print(f"\n{'=' * 90}")
    print(f"ОТЧЁТ ПО {address}")
    print(f"  проанализировано транзакций : {report['txs_analyzed']} / {len(sigs)}")
    print(f"  период                      : {report['period']['first']}"
          f" -> {report['period']['last']}")
    print(f"  SOL вошло                   : {t['sol_in']}")
    print(f"  SOL вышло                   : {t['sol_out']}")
    print(f"  уникальных источников       : {t['unique_sources']}")
    print(f"  уникальных получателей      : {t['unique_destinations']}")
    print(f"  транзакций с паттерном      : {t['drainer_pattern_txs']}")
    print(f"  ЗАХВАЧЕНО аккаунтов жертв   : {t['hijacked_accounts']}")
    print("\n  По месяцам (вход SOL):")
    for m, v in report["monthly_inflow_sol"].items():
        print(f"    {m}: {v:.4f}")
    print("\n  Топ-10 получателей (операторы/консолидация):")
    for d in report["top_destinations"][:10]:
        print(f"    {d['address']}  {d['sol']:.4f} SOL")
    print("\n  Кошельки операторов (назначение sweep-переводов и захватов):")
    for o in report["operator_wallets"][:10]:
        print(f"    {o['address']}  {o['sol_received']:.4f} SOL ({o['sweep_txs']} sweep tx)")
    print("\n  Топ-10 источников (жертвы):")
    for d_ in report["top_sources"][:10]:
        mark = " [HIJACKED]" if d_["hijacked"] else ""
        print(f"    {d_['address']}  {d_['sol']:.6f} SOL ({d_['txs']} tx){mark}")
    if downstream:
        hackers = [e for e in downstream if e["label"] == "hacker"]
        suspects = [e for e in downstream if e["label"] == "suspicious"]
        print("\n  ПУТЬ ДВИЖЕНИЯ СРЕДСТВ (исходящие потоки операторов):")
        if report["cashout_points"]:
            print("    Кэшаут (биржи/обменники):")
            for e in report["cashout_points"][:10]:
                print(f"      {e['address']}  {e['sol_received']:.4f} SOL"
                      f" ({e['txs']} tx)  -> {e['service']}")
        if hackers:
            print("    Соучастники/другие хакеры (2+ перевода):")
            for e in hackers[:10]:
                print(f"      {e['address']}  {e['sol_received']:.4f} SOL"
                      f" ({e['txs']} tx)")
        if suspects:
            print("    Подозрительные получатели (1 перевод):")
            for e in suspects[:10]:
                print(f"      {e['address']}  {e['sol_received']:.4f} SOL")
    print(f"\n  Полный отчёт: {out_path}")
    print(f"  Кэш транзакций: {cache_path}")

    # Отправляем находки в БД: жертвы — захваченные аккаунты,
    # хакер — реальный оператор транзакции (назначение sweep / новый владелец
    # аккаунта); сканируемый кошелёк считается оператором только если он сам
    # получил sweep-перевод в этой транзакции.
    if api_url:
        poster = FindingPoster(api_url, api_key, threads=rpc.threads)
        payloads = []
        for item in report["hijacked_accounts"]:
            tx = cache.get(item["tx"])
            if not tx:
                continue
            verdict, indicators, details = detect_patterns(tx, watch_programs)
            victim, hacker = extract_parties(details, signers(tx))
            if not victim:
                victim = item["address"]
            if not hacker:
                sweep_dests = {d.get("to") for d in details["drain_transfers"]}
                if address in sweep_dests:
                    hacker = address
            if hacker == victim:
                hacker = ""
            payloads.append({
                "chain": "solana",
                "signature": item["tx"],
                "slot": tx.get("slot") or 0,
                "verdict": verdict,
                "indicators": indicators,
                "victim_address": victim,
                "hacker_address": hacker,
                "amount_sol": round(sum(d.get("amount_sol") or 0
                                        for d in details["drain_transfers"]), 9),
                "sweeps": sweep_breakdown(details["drain_transfers"]),
                "programs": finding_programs(details),
                "exposed_addresses": drainer_sources.get(item["tx"], []),
                "source": "scan-wallet",
            })
        responses, skipped = poster.submit_all(payloads)
        sent = sum(1 for r in responses if r is not None)
        total = len(report['hijacked_accounts'])
        print(f"  Отправлено находок в API: {sent}/{total}"
              + (f" (дублей пропущено: {skipped})" if skipped else ""))

        # Downstream-находки: получатель одного перевода от оператора —
        # SUSPICIOUS, 2+ переводов — hacker (verdict DRAINER с пустой жертвой:
        # поток, а не дрейн — бэкенд регистрирует только сторону хакера).
        # Биржи/обменники и программы не отправляем — это не злоумышленники.
        flow_payloads = []
        for e in downstream:
            if e["label"] == "suspicious":
                f_verdict, f_ind = "SUSPICIOUS", ["F1_DOWNSTREAM_TRANSFER"]
            elif e["label"] == "hacker":
                f_verdict, f_ind = "DRAINER", ["F2_REPEAT_DOWNSTREAM"]
            else:
                continue
            flow_payloads.append({
                "chain": "solana",
                "signature": e["signatures"][-1],
                "slot": 0,
                "verdict": f_verdict,
                "indicators": f_ind,
                "victim_address": "",
                "hacker_address": e["address"],
                "amount_sol": e["sol_received"],
                "programs": [],
                "exposed_addresses": e["sources"],
                "source": "flow-trace",
            })
        flow_total = len(flow_payloads)
        if flow_total:
            responses, skipped = poster.submit_all(flow_payloads)
            flow_sent = sum(1 for r in responses if r is not None)
            print(f"  Отправлено downstream-находок в API: "
                  f"{flow_sent}/{flow_total}"
                  + (f" (дублей пропущено: {skipped})" if skipped else ""))
    return report


# -------------------------------------------------------------------- main

def main():
    ap = argparse.ArgumentParser(
        description="Детектор Solana-дрейнеров и анализатор их сетей",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__)
    sub = ap.add_subparsers(dest="mode", required=True)

    p_tx = sub.add_parser("check-tx", help="проверить транзакцию на паттерн")
    p_tx.add_argument("signature")

    p_qs = sub.add_parser("quick-scan", help="проверить последние N транзакций адреса")
    p_qs.add_argument("address")
    p_qs.add_argument("--limit", type=int, default=50)

    p_sw = sub.add_parser("scan-wallet", help="полный анализ сети кошелька")
    p_sw.add_argument("address")
    p_sw.add_argument("--cache-dir", default="./drainer_cache")
    p_sw.add_argument("--max-txs", type=int, default=None,
                      help="ограничить число транзакций (для теста)")
    p_sw.add_argument("--trace-depth", type=int, default=1,
                      help="глубина трейсинга исходящих потоков операторов "
                           "(0 — только исходящие операторов, -1 — выключить)")
    p_sw.add_argument("--trace-wallets", type=int, default=10,
                      help="макс. кошельков операторов на уровень трейсинга")
    p_sw.add_argument("--trace-txs", type=int, default=100,
                      help="макс. транзакций на кошелёк при трейсинге")
    p_sw.add_argument("--no-trace", action="store_true",
                      help="не трейсить исходящие потоки операторов")

    p_w = sub.add_parser("watch", help="LIVE-мониторинг блоков на паттерн дрейнера")
    p_w.add_argument("--full-blocks", action="store_true",
                     help="анализировать все транзакции (по умолчанию prefilter)")
    p_w.add_argument("--out", default=None,
                     help="файл для лога находок (JSONL)")
    p_w.add_argument("--stats-every", type=int, default=20,
                     help="печатать статистику каждые N блоков")
    p_w.add_argument("--start-slot", type=int, default=None,
                     help="начать с конкретного слота (по умолчанию — текущий)")
    p_w.add_argument("--window", type=int, default=None,
                     help="скользящее окно слотов в watch (по умолчанию "
                          "4*threads): дальше окна от минимального "
                          "необработанного слота воркеры не стартуют")
    p_w.add_argument("--hacker-file",
                     default=os.environ.get("SOLANA_HACKERS_FILE",
                                            DEFAULT_HACKERS_FILE),
                     help="JSON с хакерскими кошельками для отслеживания "
                          "движения украденного (дополняет список из БД "
                          "бэкенда; по умолчанию solana_hackers.json рядом "
                          "со скриптом)")

    for p in (p_tx, p_qs, p_sw, p_w):
        p.add_argument("--watch-program", action="append", default=[],
                       help="добавить programId в список вредоносных")
        p.add_argument("--rpc-url", action="append",
                       default=([u] if (u := os.environ.get("SOLANA_RPC_URL"))
                                else None),
                       help="RPC-эндпоинт Solana (можно несколько раз; env "
                            "SOLANA_RPC_URL). Например Helius: "
                            "https://mainnet.helius-rpc.com/?api-key=KEY. "
                            "По умолчанию — публичные узлы")
        p.add_argument("--threads", type=int,
                       default=int(os.environ.get("SOLANA_SCAN_THREADS", "8")),
                       help="потоков для загрузки tx/трейсинга/отправки в API "
                            "(env SOLANA_SCAN_THREADS, по умолчанию 8; "
                            "для публичных RPC ставьте 1)")
        p.add_argument("--rpc-delay", type=float, default=0.0,
                       help="пауза перед каждым RPC-вызовом, сек "
                            "(для публичных RPC: --threads 1 --rpc-delay 0.8)")
        p.add_argument("--programs-file",
                       default=os.environ.get("SOLANA_PROGRAMS_FILE",
                                              DEFAULT_PROGRAMS_FILE),
                       help="JSON со списком известных программ "
                            "(по умолчанию solana_programs.json рядом со скриптом)")
        p.add_argument("--exchanges-file",
                       default=os.environ.get("SOLANA_EXCHANGES_FILE",
                                              DEFAULT_EXCHANGES_FILE),
                       help="JSON с кошельками бирж/обменников (по умолчанию "
                            "solana_exchanges.json рядом со скриптом)")

    # отправка находок в БД через API бэкенда (watch и scan-wallet)
    for p in (p_sw, p_w):
        p.add_argument("--api-url", default=os.environ.get("VAULN_API_URL"),
                       help="URL бэкенда для записи находок в БД "
                            "(env VAULN_API_URL)")
        p.add_argument("--api-key", default=os.environ.get("ADMIN_API_KEY"),
                       help="admin API key бэкенда (env ADMIN_API_KEY)")

    args = ap.parse_args()
    if args.programs_file:
        load_known_programs(args.programs_file)
    if args.exchanges_file:
        load_known_exchanges(args.exchanges_file)
    rpc = Rpc(endpoints=args.rpc_url, threads=args.threads,
              delay=args.rpc_delay)

    if args.mode == "check-tx":
        sys.exit(mode_check_tx(rpc, args.signature, args.watch_program))
    elif args.mode == "quick-scan":
        mode_quick_scan(rpc, args.address, args.limit, args.watch_program)
    elif args.mode == "scan-wallet":
        trace_depth = -1 if args.no_trace else args.trace_depth
        mode_scan_wallet(rpc, args.address, args.cache_dir,
                         args.watch_program, args.max_txs,
                         api_url=args.api_url, api_key=args.api_key,
                         trace_depth=trace_depth,
                         trace_wallets=args.trace_wallets,
                         trace_txs=args.trace_txs)
    elif args.mode == "watch":
        mode_watch(rpc, args.watch_program, full_blocks=args.full_blocks,
                   out_file=args.out, stats_every=args.stats_every,
                   start_slot=args.start_slot, window=args.window,
                   hacker_file=args.hacker_file,
                   api_url=args.api_url, api_key=args.api_key)


if __name__ == "__main__":
    main()