#!/usr/bin/env python3
"""TRON: live-мониторинг блоков.

HTTP API (TronGrid-совместимое): POST /wallet/getnowblock возвращает
последний блок; номер головы — block_header.raw_data.number. Из контрактов
TransferContract извлекаются owner/to/amount (hex-адреса конвертируются в
base58check, т.к. в базе адреса TRON хранятся в base58). Для вызовов смарт
контрактов учитываются только TRC20 transfer(...) — получатель и сумма
декодируются из ABI данные, адрес контракта (например USDT
TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t) никогда не становится «получателем».

Запуск: python3 tron.py [--api-url …] [--api-key …] [--interval 3]
Конфигурация также через env: VAULN_API_URL, ADMIN_API_KEY, TRON_API_URL
(список эндпоинтов через запятую).
"""

import hashlib
import os
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from common import Transfer, http_json, parse_args, run  # noqa: E402

CHAIN = "tron"
POLL_INTERVAL = 3.0  # блок ~3 секунды

API_ENDPOINTS = [
    "https://api.trongrid.io",
    "https://api.tronstack.io",
]

B58_ALPHABET = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

# TRC20 transfer(address,uint256) selector. The recipient is ABI-encoded as
# the first 32-byte argument (12 zero bytes + 21-byte TRON address, 0x41
# prefix). Anything other than a direct transfer call is not a plain
# transfer and is skipped entirely — otherwise the contract address gets
# mis-reported as the recipient (as happened for the USDT contract
# TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t, which ended up registered as
# "suspicious" and then every USDT transfer tripped an F1 finding).
T20_TRANSFER_SELECTOR = "a9059cbb"


def decode_trc20_transfer(data_hex):
    """TRC20 transfer(...) call data -> (recipient, raw amount)."""
    d = (data_hex or "").lower().replace("0x", "")
    if len(d) < 8 + 64 + 64 or d[:8] != T20_TRANSFER_SELECTOR:
        return "", 0
    word1, word2 = d[8:72], d[72:136]
    hex_addr = "41" + word1[-40:]  # last 20 bytes; tron prefix 0x41
    amount = 0
    try:
        amount = int(word2, 16)
    except ValueError:
        pass
    return hex_to_base58check(hex_addr), amount


def hex_to_base58check(hex_addr):
    """TRON hex-адрес (41 + 20 байт) -> base58check (T…)."""
    try:
        payload = bytes.fromhex(hex_addr)
    except ValueError:
        return ""
    checksum = hashlib.sha256(hashlib.sha256(payload).digest()).digest()[:4]
    num = int.from_bytes(payload + checksum, "big")
    out = ""
    while num:
        num, rem = divmod(num, 58)
        out = B58_ALPHABET[rem] + out
    # ведущие нулевые байты -> '1'
    for b in payload + checksum:
        if b == 0:
            out = "1" + out
        else:
            break
    return out


class TronApi:
    """TronGrid-совместимый клиент с ротацией эндпоинтов."""

    def __init__(self, endpoints, timeout=30, retries=3):
        self.endpoints = list(endpoints)
        self.timeout = timeout
        self.retries = retries
        self.idx = 0

    def now_block(self):
        last_err = None
        for _ in range(self.retries * len(self.endpoints)):
            ep = self.endpoints[self.idx % len(self.endpoints)]
            self.idx += 1
            try:
                return http_json(ep + "/wallet/getnowblock", {},
                                 timeout=self.timeout)
            except Exception as e:  # noqa: BLE001
                last_err = e
                time.sleep(0.3)
        raise RuntimeError(f"все TRON endpoints недоступны: {last_err}")


def make_tron_watcher(endpoints):
    api = TronApi(endpoints)
    state = {"height": None, "block": None}

    def fetch(height):
        """Блок по номеру: TronGrid отдаёт только голову, поэтому блоки
        запоминаются по мере появления (watcher идёт строго вперёд)."""
        if state["height"] != height or state["block"] is None:
            block = api.now_block()
            h = ((block.get("block_header") or {}).get("raw_data") or {}
                 ).get("number")
            if h is None:
                raise RuntimeError("getnowblock вернул пустой блок")
            state["height"], state["block"] = int(h), block
        return state["block"]

    def latest():
        block = api.now_block()
        h = ((block.get("block_header") or {}).get("raw_data") or {}
             ).get("number")
        if h is None:
            raise RuntimeError("getnowblock вернул пустой блок")
        state["height"], state["block"] = int(h), block
        return int(h)

    def transfers(height):
        block = fetch(height)
        out = []
        for tx in block.get("transactions") or []:
            txid = tx.get("txID") or ""
            contracts = ((tx.get("raw_data") or {}).get("contract")) or []
            for c in contracts:
                value = c.get("parameter") or {}
                value = value.get("value") or {}
                ctype = c.get("type")
                if ctype == "TransferContract":
                    out.append(Transfer(
                        txid,
                        hex_to_base58check(value.get("owner_address") or ""),
                        hex_to_base58check(value.get("to_address") or ""),
                        (value.get("amount") or 0) / 1e6))
                elif ctype == "TriggerSmartContract":
                    recipient, raw = decode_trc20_transfer(value.get("data"))
                    if not recipient:
                        continue  # вызовы контрактов — не переводы
                    # большинство TRC20 (USDT/USDD) — 6 десятичных
                    out.append(Transfer(
                        txid,
                        hex_to_base58check(value.get("owner_address") or ""),
                        recipient, raw / 1e6, is_token=True,
                        token_symbol="TRC20"))
        return out

    return latest, transfers


def main():
    args = parse_args("tron", POLL_INTERVAL)
    endpoints = list(API_ENDPOINTS)
    env_eps = os.environ.get("TRON_API_URL")
    if env_eps:
        endpoints = [e.strip().rstrip("/") for e in env_eps.split(",")
                     if e.strip()]
    latest, transfers = make_tron_watcher(endpoints)
    run("tron", CHAIN, args.interval, latest, transfers,
        api_url=args.api_url, api_key=args.api_key,
        once=args.once, start=args.start)


if __name__ == "__main__":
    main()
