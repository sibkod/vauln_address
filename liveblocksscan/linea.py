#!/usr/bin/env python3
"""Linea: live-мониторинг блоков.

Извлекает from/to/value из каждой транзакции нового блока
(eth_blockNumber + eth_getBlockByNumber с полными транзакциями), проверяет
адреса через POST /api/check/bulk (chain=linea -> evm) и постит находки
о движениях средств с участием адресов из базы угроз. Транзакции с value=0
тоже учитываются: активность известного адреса (аппрувы, вызовы контрактов
дрейнера) — сама по себе сигнал.

Запуск: python3 linea.py [--api-url …] [--api-key …] [--interval 2.0]
Конфигурация также через env: VAULN_API_URL, ADMIN_API_KEY, LINEA_RPC_URL
(список RPC через запятую).
"""

import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from common import (BlockUnavailable, JsonRpc, Transfer,  # noqa: E402
                    parse_args, run)

CHAIN = "linea"
POLL_INTERVAL = 2.0  # блок ~2 секунды

RPC_ENDPOINTS = [
"https://rpc.linea.build",
    "https://linea-rpc.publicnode.com",
    "https://rpc.ankr.com/linea",
    "https://linea.drpc.org",
]


def make_watcher(endpoints):
    """Вернуть (latest_fn, transfers_fn) для common.run()."""
    rpc = JsonRpc(endpoints)

    def latest():
        return int(rpc.call("eth_blockNumber"), 16)

    def transfers(height):
        block = rpc.call("eth_getBlockByNumber", [hex(height), True])
        if not block:
            raise BlockUnavailable(f"блок {height} не найден")
        out = []
        for tx in block.get("transactions") or []:
            frm = (tx.get("from") or "").lower()
            to = (tx.get("to") or "").lower()  # None при создании контракта
            value = int(tx.get("value") or "0x0", 16) / 1e18
            out.append(Transfer(tx.get("hash") or "", frm, to, value))
        return out

    return latest, transfers


def main():
    args = parse_args("linea", POLL_INTERVAL)
    endpoints = list(RPC_ENDPOINTS)
    env_eps = os.environ.get("LINEA_RPC_URL")
    if env_eps:
        endpoints = [e.strip() for e in env_eps.split(",") if e.strip()]
    latest, transfers = make_watcher(endpoints)
    run("linea", CHAIN, args.interval, latest, transfers,
        api_url=args.api_url, api_key=args.api_key,
        once=args.once, start=args.start)


if __name__ == "__main__":
    main()
