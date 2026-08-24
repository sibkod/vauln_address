#!/usr/bin/env python3
"""Ethereum mainnet: live-мониторинг блоков.

Извлекает переводы нативной монеты (eth_getBlockByNumber) и ERC20-токенов
(логи Transfer из eth_getBlockReceipts) из каждого нового блока, проверяет
адреса через POST /api/check/bulk (chain=ethereum -> evm) и постит находки
о движениях средств с участием адресов из базы угроз. Только реальные
переводы: вызовы контрактов без движения средств (аппрувы и т.п.) находок
не порождают.

Запуск: python3 ethereum.py [--api-url …] [--api-key …] [--interval 12.0]
Конфигурация также через env: VAULN_API_URL, ADMIN_API_KEY, ETH_RPC_URL
(список RPC через запятую).
"""

import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from common import make_evm_watcher, parse_args, run  # noqa: E402

CHAIN = "ethereum"
POLL_INTERVAL = 12.0  # блок ~12 секунд

RPC_ENDPOINTS = [
"https://eth.llamarpc.com",
    "https://ethereum-rpc.publicnode.com",
    "https://rpc.ankr.com/eth",
    "https://cloudflare-eth.com",
    "https://eth.drpc.org"
]


def main():
    args = parse_args("ethereum", POLL_INTERVAL)
    endpoints = list(RPC_ENDPOINTS)
    env_eps = os.environ.get("ETH_RPC_URL")
    if env_eps:
        endpoints = [e.strip() for e in env_eps.split(",") if e.strip()]
    latest, transfers = make_evm_watcher("ethereum", endpoints)
    run("ethereum", CHAIN, args.interval, latest, transfers,
        api_url=args.api_url, api_key=args.api_key,
        once=args.once, start=args.start)


if __name__ == "__main__":
    main()
