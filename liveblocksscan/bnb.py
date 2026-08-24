#!/usr/bin/env python3
"""BNB Smart Chain: live-мониторинг блоков.

Извлекает переводы нативной монеты (eth_getBlockByNumber) и ERC20-токенов
(логи Transfer из eth_getBlockReceipts) из каждого нового блока, проверяет
адреса через POST /api/check/bulk (chain=bnb -> evm) и постит находки
о движениях средств с участием адресов из базы угроз. Только реальные
переводы: вызовы контрактов без движения средств (аппрувы и т.п.) находок
не порождают.

Запуск: python3 bnb.py [--api-url …] [--api-key …] [--interval 3.0]
Конфигурация также через env: VAULN_API_URL, ADMIN_API_KEY, BNB_RPC_URL
(список RPC через запятую).
"""

import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from common import make_evm_watcher, parse_args, run  # noqa: E402

CHAIN = "bnb"
POLL_INTERVAL = 3.0  # блок ~3 секунды

RPC_ENDPOINTS = [
"https://bsc-dataseed.binance.org",
    "https://bsc-dataseed1.defibit.io",
    "https://bnb-rpc.publicnode.com",
    "https://rpc.ankr.com/bsc",
    "https://bsc.drpc.org"
]


def main():
    args = parse_args("bnb", POLL_INTERVAL)
    endpoints = list(RPC_ENDPOINTS)
    env_eps = os.environ.get("BNB_RPC_URL")
    if env_eps:
        endpoints = [e.strip() for e in env_eps.split(",") if e.strip()]
    latest, transfers = make_evm_watcher("bnb", endpoints)
    run("bnb", CHAIN, args.interval, latest, transfers,
        api_url=args.api_url, api_key=args.api_key,
        once=args.once, start=args.start)


if __name__ == "__main__":
    main()
