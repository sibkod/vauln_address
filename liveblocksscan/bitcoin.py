#!/usr/bin/env python3
"""Bitcoin: live-мониторинг блоков.

Публичных JSON-RPC узлов Bitcoin без авторизации нет, поэтому используется
Esplora-совместимый REST API (blockstream.info, mempool.space):
  GET /blocks/tip/height      — высота головы
  GET /block-height/{h}       — хэш блока
  GET /block/{hash}/txs       — транзакции (vin[].prevout + vout[])

Из каждой транзакции извлекаются отправители (адреса prevout во vin) и
получатели (адреса vout) с суммами; сдача (vout обратно на адрес
отправителя) пропускается. Найденные в базе угроз адреса порождают находки.

Запуск: python3 bitcoin.py [--api-url …] [--api-key …] [--interval 60]
Конфигурация также через env: VAULN_API_URL, ADMIN_API_KEY, BTC_API_URL
(список Esplora-эндпоинтов через запятую).
"""

import os
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from common import (BlockUnavailable, Transfer, http_json,  # noqa: E402
                    parse_args, run)

CHAIN = "btc"
POLL_INTERVAL = 60.0  # блок ~10 минут

API_ENDPOINTS = [
    "https://blockstream.info/api",
    "https://mempool.space/api",
]


class Esplora:
    """REST-клиент Esplora с ротацией эндпоинтов."""

    def __init__(self, endpoints, timeout=60, retries=3):
        self.endpoints = list(endpoints)
        self.timeout = timeout
        self.retries = retries
        self.idx = 0

    def get(self, path):
        last_err = None
        for _ in range(self.retries * len(self.endpoints)):
            ep = self.endpoints[self.idx % len(self.endpoints)]
            self.idx += 1
            try:
                return http_json(ep + path, timeout=self.timeout)
            except Exception as e:  # noqa: BLE001
                last_err = e
                time.sleep(0.5)
        raise RuntimeError(f"все Esplora endpoints недоступны: {last_err}")


def make_btc_watcher(endpoints):
    api = Esplora(endpoints)

    def latest():
        return int(api.get("/blocks/tip/height"))

    def transfers(height):
        block_hash = api.get(f"/block-height/{height}")
        if not block_hash:
            raise BlockUnavailable(f"блок {height} не найден")
        txs = api.get(f"/block/{block_hash}/txs")
        if txs is None:
            raise BlockUnavailable(f"транзакции блока {height} недоступны")
        out = []
        for tx in txs:
            txid = tx.get("txid") or ""
            senders = set()
            for vin in tx.get("vin") or []:
                prev = vin.get("prevout") or {}
                addr = prev.get("scriptpubkey_address")
                if addr:
                    senders.add(addr)
            sender = sorted(senders)[0] if senders else ""
            for vout in tx.get("vout") or []:
                addr = vout.get("scriptpubkey_address")
                if not addr or addr in senders:
                    continue  # сдача / служебные выходы без адреса
                out.append(Transfer(
                    txid, sender, addr, (vout.get("value") or 0) / 1e8))
        return out

    return latest, transfers


def main():
    args = parse_args("bitcoin", POLL_INTERVAL)
    endpoints = list(API_ENDPOINTS)
    env_eps = os.environ.get("BTC_API_URL")
    if env_eps:
        endpoints = [e.strip().rstrip("/") for e in env_eps.split(",")
                     if e.strip()]
    latest, transfers = make_btc_watcher(endpoints)
    run("bitcoin", CHAIN, args.interval, latest, transfers,
        api_url=args.api_url, api_key=args.api_key,
        once=args.once, start=args.start)


if __name__ == "__main__":
    main()
