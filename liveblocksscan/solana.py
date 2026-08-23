#!/usr/bin/env python3
"""Solana: live-мониторинг блоков (слотов).

getSlot + getBlock (jsonParsed): из каждой транзакции извлекаются системные
переводы SOL (program=system, type=transfer) крупнее пыли; участники
проверяются через POST /api/check/bulk (chain=solana), движения средств с
участием адресов из базы угроз постятся как находки.

Дрейнер-паттерны Solana отслеживает основной solana_scan.py — этот скрипт
дополняет его отслеживанием движений по всем известным адресам базы.

Запуск: python3 solana.py [--api-url …] [--api-key …] [--interval 2]
Конфигурация также через env: VAULN_API_URL, ADMIN_API_KEY, SOLANA_RPC_URL
(список RPC через запятую).
"""

import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from common import (BlockUnavailable, JsonRpc, Transfer,  # noqa: E402
                    parse_args, run)

CHAIN = "solana"
POLL_INTERVAL = 2.0  # слот ~0.4-0.6 секунды, обрабатываем пачкой

RPC_ENDPOINTS = [
    "https://api.mainnet-beta.solana.com",
    "https://solana-rpc.publicnode.com",
    "https://rpc.ankr.com/solana",
]

# Пыль не интересна: отсекает ренту и служебные микропереводы.
MIN_LAMPORTS = 1_000_000  # 0.001 SOL


def make_sol_watcher(endpoints):
    rpc = JsonRpc(endpoints, timeout=60)

    def latest():
        return int(rpc.call("getSlot"))

    def transfers(slot):
        block = rpc.call("getBlock", [
            slot,
            {"encoding": "jsonParsed", "transactionDetails": "full",
             "rewards": False, "maxSupportedTransactionVersion": 0},
        ])
        if not block:
            # пропущенный слот или узел не хранит — идём дальше
            raise BlockUnavailable(f"слот {slot} пропущен")
        out = []
        for tx in block.get("transactions") or []:
            meta = tx.get("meta") or {}
            if meta.get("err"):
                continue
            sigs = (tx.get("transaction") or {}).get("signatures") or [""]
            sig = sigs[0]
            message = (tx.get("transaction") or {}).get("message") or {}
            ixs = list(message.get("instructions") or [])
            for inner in meta.get("innerInstructions") or []:
                ixs.extend(inner.get("instructions") or [])
            for ix in ixs:
                if ix.get("program") != "system":
                    continue
                parsed = ix.get("parsed") or {}
                if parsed.get("type") != "transfer":
                    continue
                info = parsed.get("info") or {}
                lamports = int(info.get("lamports") or 0)
                if lamports < MIN_LAMPORTS:
                    continue
                out.append(Transfer(sig, info.get("source") or "",
                                    info.get("destination") or "",
                                    lamports / 1e9))
        return out

    return latest, transfers


def main():
    args = parse_args("solana", POLL_INTERVAL)
    endpoints = list(RPC_ENDPOINTS)
    env_eps = os.environ.get("SOLANA_RPC_URL")
    if env_eps:
        endpoints = [e.strip() for e in env_eps.split(",") if e.strip()]
    latest, transfers = make_sol_watcher(endpoints)
    run("solana", CHAIN, args.interval, latest, transfers,
        api_url=args.api_url, api_key=args.api_key,
        once=args.once, start=args.start)


if __name__ == "__main__":
    main()
