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

Белый список известных программ (~300 programId: DEX, лендинг, NFT,
инфраструктура) подгружается из solana_programs.json рядом со скриптом
(флаг --programs-file / env SOLANA_PROGRAMS_FILE) — без него P3 шумит.

Режимы watch и scan-wallet с флагом --api-url (env VAULN_API_URL) отправляют
каждую находку (victim + hacker) в БД через API бэкенда
(POST /api/admin/scanner/findings, заголовок X-Admin-Key = --api-key /
env ADMIN_API_KEY). Находки видны на странице живого мониторинга.

Только stdlib. Кэширует транзакции в <cache_dir>/<address>.json
"""

import argparse
import datetime
import json
import os
import sys
import time
import urllib.request
from collections import Counter, defaultdict

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
}
# EtrnLzgbS7nMMy5fbD42kXiUzGg8XQzJ972Xtk1cjWih убрана: это активный легитимный
# сервис/бот (не содержит takeover/transfer, балансы не меняются), попавший
# в watchlist по ошибке. P5 теперь требует подтверждения другим индикатором.


# Дрейнерный стиль: ровно 2 инструкции ComputeBudget + неизвестная программа.
# На миллионах реальных транзакций это редкая комбинация, а у дрейнера — постоянная.
COMPUTE_BUDGET = "ComputeBudget111111111111111111111111111111"


DEFAULT_PROGRAMS_FILE = os.path.join(
    os.path.dirname(os.path.abspath(__file__)), "solana_programs.json")


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
        sweep = details["drain_transfers"][0]
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
        "programs": details["unknown_programs"],
        "source": source,
    }


class Rpc:
    """RPC-клиент с ротацией эндпоинтов и ретраями."""

    def __init__(self, timeout=60):
        self.ep_idx = 0
        self.timeout = timeout

    def call(self, method, params, retries=6):
        last_err = None
        for attempt in range(retries):
            ep = RPC_ENDPOINTS[self.ep_idx % len(RPC_ENDPOINTS)]
            try:
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
                self.ep_idx += 1
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
        """Все подписи адреса (или до limit), новые первыми."""
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
            time.sleep(0.4)
        return out

    def get_transactions(self, signatures, progress=False):
        """Выкачивает транзакции по одной — публичные узлы режут батчи."""
        result = {}
        missing = list(signatures)
        for i, s in enumerate(missing):
            try:
                r = self.call(
                    "getTransaction",
                    [s, {"encoding": "jsonParsed",
                         "maxSupportedTransactionVersion": 0}])
                result[s] = r.get("result")
            except Exception:  # noqa: BLE001
                result[s] = None
            time.sleep(0.8)
            if progress and (i + 1) % 40 == 0:
                print(f"  txs: {i + 1}/{len(missing)}", flush=True)
        # вторая попытка для null'ов — первый прогон мог упасть по rate limit
        nulls = [s for s, v in result.items() if not v]
        if nulls:
            time.sleep(5)
            for s in nulls:
                try:
                    r = self.call(
                        "getTransaction",
                        [s, {"encoding": "jsonParsed",
                             "maxSupportedTransactionVersion": 0}])
                    result[s] = r.get("result")
                except Exception:  # noqa: BLE001
                    pass
                time.sleep(0.8)
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

            # P2: перевод >=90% баланса, остаток <0.01 SOL.
            # Off-curve участник (PDA: пул, эскроу, служебный аккаунт
            # программы) — это маршрутизация ликвидности, не кража.
            if itype == "transfer":
                src = info.get("source")
                dst = info.get("destination")
                lamports = int(info.get("lamports") or 0)
                if src in key_list and is_on_curve(src) and is_on_curve(dst):
                    idx = key_list.index(src)
                    if idx < len(pre) and idx < len(post) and pre[idx] > 0:
                        ratio = lamports / pre[idx]
                        if ratio >= 0.9 and post[idx] < 10_000_000 and lamports > 1_000_000:
                            details["drain_transfers"].append({
                                "from": src, "to": dst,
                                "amount_sol": lamports / 1e9,
                                "left_sol": post[idx] / 1e9,
                                "ratio": round(ratio, 4)})

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
               stats_every=20, start_slot=None, api_url=None, api_key=None):
    """Живой мониторинг блоков Solana на паттерн дрейнера.

    Стратегия по умолчанию (лёгкая):
      1. getBlock(signatures) — дешёвый блок (~150 КБ), prefilter транзакций
      2. getTransaction — полный разбор только подозрительных
    С --full-blocks: getBlock(full) — анализ всех транзакций сразу
      (нужно ~0.5 блока/сек на эндпоинт, публичные RPC на пределе).
    С --api-url: каждая находка отправляется в БД через API бэкенда.
    """
    watch = dict(KNOWN_BAD_PROGRAMS)
    for p in (watch_programs or []):
        watch.setdefault(p, "user-watchlist")

    print("LIVE-режим. Мониторим блоки на паттерн дрейнера.")
    print(f"  watchlist программ: {len(watch)} ({', '.join(list(watch)[:3])}...)")
    print(f"  стратегия: {'FULL blocks (все транзакции)' if full_blocks else 'prefilter (signatures->tx)'}")
    if out_file:
        print(f"  находки пишем в: {out_file}")
    if api_url:
        print(f"  находки отправляем в API: {api_url}")
    print("  Ctrl+C для остановки.\n")

    slot = start_slot or (rpc.get_slot() - 100)  # RPC не отдаёт свежие блоки сразу
    lag_notice_at = 0
    stats = {"blocks": 0, "txs_scanned": 0, "prefilter_hits": 0,
             "drainer": 0, "suspicious": 0, "candidates": 0}
    log_fp = open(out_file, "a") if out_file else None
    t_start = time.time()
    print(f"  стартовый слот: {slot} (RPC-лаг ~100 слотов)", flush=True)

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
        if api_url:
            hacker = entry.get("hacker", "")
            exposed = sorted({d["from"] for d in entry.get("sweeps", [])
                              if hacker and d.get("to") == hacker} |
                             {t["account"] for t in entry.get("takeovers", [])})
            resp = post_finding(api_url, api_key, {
                "chain": "solana",
                "signature": entry["signature"],
                "slot": slot_,
                "verdict": entry["verdict"],
                "indicators": entry["indicators"],
                "victim_address": entry.get("victim", ""),
                "hacker_address": hacker,
                "amount_sol": round(sum(d.get("amount_sol") or 0
                                        for d in entry.get("sweeps", [])), 9),
                "programs": entry.get("unknown_programs", []),
                "exposed_addresses": exposed,
                "source": "watch",
            })
            if resp is not None:
                print(f"      API: id={resp.get('id')} victim={entry.get('victim', '')[:12]}..."
                      f" hacker={entry.get('hacker', '')[:12]}...")

    try:
        while True:
            head = rpc.get_slot()
            if slot > head:
                time.sleep(0.4)
                continue
            # не отставать безнадёжно: если лаг > 200 блоков — прыгаем к голове
            if head - slot > 200:
                if slot > lag_notice_at:
                    print(f"  .. отставание {head - slot} блоков, прыгаем к слоту {head - 5}")
                    lag_notice_at = head
                slot = head - 5

            while slot <= head:
                blk = rpc.get_block(slot, details="full")
                if blk is None:
                    print(f"  block {slot}: недоступен/пустой, пропуск", flush=True)
                    slot += 1  # пропущенный/пустой слот
                    continue
                txs = blk.get("transactions") or []
                stats["txs_scanned"] += len(txs)
                stats["blocks"] += 1
                elapsed = time.time() - t_start
                bps = stats["blocks"] / elapsed if elapsed > 0 else 0
                print(f"  block {slot}: {len(txs)} tx "
                      f"(всего {stats['blocks']} блок., {bps:.2f} блок/с, "
                      f"chain ~2.5 блок/с)", flush=True)
                for tx in txs:
                    if (tx.get("meta") or {}).get("err"):
                        continue
                    if not full_blocks and not prefilter_block_tx(tx, watch):
                        continue
                    stats["candidates"] += 1
                    sig = (tx.get("transaction", {}).get("signatures") or [""])[0]
                    verdict, indicators, details = detect_patterns(tx, watch_programs)
                    # В живом мониторинге показываем только DRAINER (P1 —
                    # полный захват аккаунта). SUSPICIOUS (sweeps без
                    # захвата) — скипаем: это обычные свапы/переводы.
                    if verdict == "DRAINER":
                        stats["drainer"] += 1
                        victim, hacker = extract_parties(details, signers(tx))
                        record_finding(slot, {
                            "slot": slot, "signature": sig, "verdict": verdict,
                            "indicators": indicators,
                            "victim": victim, "hacker": hacker,
                            "takeovers": details["takeovers"],
                            "sweeps": details["drain_transfers"],
                            "unknown_programs": details["unknown_programs"]})
                slot += 1
            time.sleep(0.3)
    except KeyboardInterrupt:
        print(f"\nОстановлено. Итог: {stats}")
    finally:
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


def mode_scan_wallet(rpc, address, cache_dir, watch_programs=None, max_txs=None,
                     api_url=None, api_key=None):
    os.makedirs(cache_dir, exist_ok=True)
    cache_path = os.path.join(cache_dir, f"{address}.json")
    cache = json.load(open(cache_path)) if os.path.exists(cache_path) else {}

    print(f"[1/3] Список транзакций {address} ...")
    sigs = rpc.get_signatures(address, limit=max_txs)
    print(f"      всего подписей: {len(sigs)}")
    if sigs:
        print(f"      период: {fmt_time(sigs[-1]['blockTime'])}"
              f" -> {fmt_time(sigs[0]['blockTime'])}")

    print("[2/3] Загрузка транзакций (с кэшем) ...")
    # старые прогоны кэшировали null-ы из-за сброшенных RPC-запросов — чистим
    stale = [k for k, v in cache.items() if not v]
    for k in stale:
        del cache[k]
    if stale:
        print(f"      выброшено пустых записей из кэша: {len(stale)}")
    missing = [s["signature"] for s in sigs if not cache.get(s["signature"])]
    print(f"      в кэше: {len(sigs) - len(missing)}, к загрузке: {len(missing)}")
    if missing:
        fetched = rpc.get_transactions(missing, progress=True)
        cache.update(fetched)
        json.dump(cache, open(cache_path, "w"))

    print("[3/3] Анализ потоков и паттернов ...")
    in_flows, out_flows = Counter(), Counter()
    in_cnt = Counter()
    monthly = defaultdict(float)
    takeover_victims, takeover_details = set(), {}
    drainer_sources = {}  # signature -> set of senders funding the operator
    operators = Counter()  # operator (hacker) wallets: sweep destination / takeover new owner
    operator_sol = defaultdict(float)  # swept SOL received per operator
    unknown_progs = Counter()
    drainer_tx_count = 0

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
        for t in details["takeovers"]:
            takeover_victims.add(t["account"])
            takeover_details[t["account"]] = {"tx": s["signature"],
                                              "time": tx.get("blockTime")}
            if t.get("new_owner"):
                operators[t["new_owner"]] += 1
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
    print(f"\n  Полный отчёт: {out_path}")
    print(f"  Кэш транзакций: {cache_path}")

    # Отправляем находки в БД: жертвы — захваченные аккаунты,
    # хакер — реальный оператор транзакции (назначение sweep / новый владелец
    # аккаунта); сканируемый кошелёк считается оператором только если он сам
    # получил sweep-перевод в этой транзакции.
    if api_url:
        sent = 0
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
            resp = post_finding(api_url, api_key, {
                "chain": "solana",
                "signature": item["tx"],
                "slot": tx.get("slot") or 0,
                "verdict": verdict,
                "indicators": indicators,
                "victim_address": victim,
                "hacker_address": hacker,
                "amount_sol": round(sum(d.get("amount_sol") or 0
                                        for d in details["drain_transfers"]), 9),
                "programs": details["unknown_programs"],
                "exposed_addresses": drainer_sources.get(item["tx"], []),
                "source": "scan-wallet",
            })
            if resp is not None:
                sent += 1
        print(f"  Отправлено находок в API: {sent}/{len(report['hijacked_accounts'])}")
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

    p_w = sub.add_parser("watch", help="LIVE-мониторинг блоков на паттерн дрейнера")
    p_w.add_argument("--full-blocks", action="store_true",
                     help="анализировать все транзакции (по умолчанию prefilter)")
    p_w.add_argument("--out", default=None,
                     help="файл для лога находок (JSONL)")
    p_w.add_argument("--stats-every", type=int, default=20,
                     help="печатать статистику каждые N блоков")
    p_w.add_argument("--start-slot", type=int, default=None,
                     help="начать с конкретного слота (по умолчанию — текущий)")

    for p in (p_tx, p_qs, p_sw, p_w):
        p.add_argument("--watch-program", action="append", default=[],
                       help="добавить programId в список вредоносных")
        p.add_argument("--programs-file",
                       default=os.environ.get("SOLANA_PROGRAMS_FILE",
                                              DEFAULT_PROGRAMS_FILE),
                       help="JSON со списком известных программ "
                            "(по умолчанию solana_programs.json рядом со скриптом)")

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
    rpc = Rpc()

    if args.mode == "check-tx":
        sys.exit(mode_check_tx(rpc, args.signature, args.watch_program))
    elif args.mode == "quick-scan":
        mode_quick_scan(rpc, args.address, args.limit, args.watch_program)
    elif args.mode == "scan-wallet":
        mode_scan_wallet(rpc, args.address, args.cache_dir,
                         args.watch_program, args.max_txs,
                         api_url=args.api_url, api_key=args.api_key)
    elif args.mode == "watch":
        mode_watch(rpc, args.watch_program, full_blocks=args.full_blocks,
                   out_file=args.out, stats_every=args.stats_every,
                   start_slot=args.start_slot,
                   api_url=args.api_url, api_key=args.api_key)


if __name__ == "__main__":
    main()