#!/usr/bin/env python3
"""Sui: live-мониторинг чекпоинтов.

suix_getLatestCheckpointSequenceNumber -> sui_getCheckpoint (дайджесты
транзакций) -> suix_multiGetTransactionBlocks (showBalanceChanges):
отправитель — transaction.data.sender, получатели — положительные
balanceChanges в нативном SUI (0x2::sui::SUI) с AddressOwner.

Запуск: python3 sui.py [--api-url …] [--api-key …] [--interval 2]
Конфигурация также через env: VAULN_API_URL, ADMIN_API_KEY, SUI_RPC_URL
(список RPC через запятую).
"""

import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from common import (BlockUnavailable, JsonRpc, Transfer,  # noqa: E402
                    parse_args, run)

CHAIN = "sui"
POLL_INTERVAL = 2.0  # чекпоинт ~0.3-0.5 секунды, обрабатываем пачкой

RPC_ENDPOINTS = [
    "https://fullnode.mainnet.sui.io:443",
    "https://sui-rpc.publicnode.com",
    "https://rpc.ankr.com/sui",
]

SUI_COIN = "0x2::sui::SUI"
MIST = 1e9


def make_sui_watcher(endpoints):
    rpc = JsonRpc(endpoints, timeout=60)

    def latest():
        return int(rpc.call("suix_getLatestCheckpointSequenceNumber"))

    def transfers(height):
        cp = rpc.call("sui_getCheckpoint", [str(height)])
        if not cp:
            raise BlockUnavailable(f"чекпоинт {height} не найден")
        digests = cp.get("transactions") or []
        if not digests:
            return []
        txs = rpc.call("suix_multiGetTransactionBlocks", [
            digests,
            {"showInput": False, "showEffects": False,
             "showBalanceChanges": True},
        ]) or []
        out = []
        for tx in txs:
            if not tx:
                continue
            sender = ((tx.get("transaction") or {}).get("data") or {}
                      ).get("sender") or ""
            digest = tx.get("digest") or ""
            received = {}
            for bc in tx.get("balanceChanges") or []:
                if bc.get("coinType") != SUI_COIN:
                    continue
                owner = bc.get("owner") or {}
                addr = owner.get("AddressOwner")
                try:
                    amount = int(bc.get("amount") or 0)
                except ValueError:
                    continue
                if not addr or amount <= 0 or addr == sender:
                    continue
                received[addr] = received.get(addr, 0) + amount
            for addr, amount in received.items():
                out.append(Transfer(digest, sender, addr, amount / MIST))
        return out

    return latest, transfers


def main():
    args = parse_args("sui", POLL_INTERVAL)
    endpoints = list(RPC_ENDPOINTS)
    env_eps = os.environ.get("SUI_RPC_URL")
    if env_eps:
        endpoints = [e.strip() for e in env_eps.split(",") if e.strip()]
    latest, transfers = make_sui_watcher(endpoints)
    run("sui", CHAIN, args.interval, latest, transfers,
        api_url=args.api_url, api_key=args.api_key,
        once=args.once, start=args.start)


if __name__ == "__main__":
    main()
