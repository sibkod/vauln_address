# liveblocksscan — live-мониторинг блоков по всем сетям

Скрипты поллят последние блоки своей сети, извлекают из транзакций адреса,
**массово проверяют** их через `POST /api/check/bulk` и для движений средств
с участием адресов из базы угроз отправляют находки на
`POST /api/admin/scanner/findings`. Находки появляются:

- в ленте мониторинга (`/monitor`, `GET /api/monitor/findings`);
- в отчётах кошельков (`/api/report`) — дерево транзакций, денежные потоки,
  цепочка доказательств;
- в базе: вторичные получатели регистрируются как `suspicious`, отправители,
  финансирующие известный адрес, получают флаг `associated_hacker`.

Зависимостей нет — только стандартная библиотека Python 3.8+.

## Скрипты

| Скрипт        | Сеть              | Источник блоков                        |
|---------------|-------------------|----------------------------------------|
| `ethereum.py` | Ethereum          | JSON-RPC (список в скрипте)            |
| `bnb.py`      | BNB Smart Chain   | JSON-RPC                               |
| `base.py`     | Base              | JSON-RPC                               |
| `linea.py`    | Linea             | JSON-RPC                               |
| `arbitrum.py` | Arbitrum One      | JSON-RPC                               |
| `polygon.py`  | Polygon PoS       | JSON-RPC                               |
| `optimism.py` | Optimism          | JSON-RPC                               |
| `avalanche.py`| Avalanche C-Chain | JSON-RPC                               |
| `bitcoin.py`  | Bitcoin           | Esplora REST (blockstream/mempool)     |
| `solana.py`   | Solana            | JSON-RPC getSlot/getBlock              |
| `tron.py`     | TRON              | TronGrid-совместимый HTTP API (TRX + TRC20 transfer) |
| `sui.py`      | Sui               | JSON-RPC чекпоинты + balanceChanges    |

Каждая EVM-сеть — полностью самостоятельный скрипт со своим списком
публичных RPC (round-robin с ретраями); список можно заменить переменной
окружения `<NETWORK>_RPC_URL` (через запятую). Общее ядро (API-клиент,
цикл поллинга, постинг находок) — в `common.py`.

## Запуск

```bash
export VAULN_API_URL=http://127.0.0.1:8080
export ADMIN_API_KEY=...            # ключ админ-API бэкенда

python3 ethereum.py                 # вечный цикл от текущей головы
python3 bnb.py --interval 3
python3 bitcoin.py --once           # один проход по доступным блокам
python3 solana.py --start 250000000 --once
```

Флаги: `--api-url`, `--api-key`, `--interval` (пауза между опросами),
`--start` (стартовый блок вместо головы), `--once`.

## Семантика находок

- **Исходящий перевод с compromised адреса** (`hacked`, `drained`,
  `phishing` — куда идут украденные; `hacker` — flush) — `SUSPICIOUS` +
  `F1_DOWNSTREAM_TRANSFER`, `victim=""`, `hacker=получатель`,
  `exposed=[отправитель]`. Получатель регистрируется как `suspicious`,
  дерево отчёта связывает выплату с отправителем.
- **Входящий перевод на hacker** — пополнение от жертвы: `SUSPICIOUS` +
  `L1_WATCHED_INFLOW`, `victim=отправитель`, `hacker=известный адрес`.
  Отправитель получает флаг `associated_hacker`. Входящие на victim-адреса
  (`hacked`/`drained`/`phishing`) не мониторятся — по жертве средства уже
  ушли, её приём нового платежа не значит compromise.

Находки создаются только для адресов со статусами `hacked`, `hacker`,
`drained`, `phishing`, `suspicious` — движения всех остальных статусов
(`vulnerable`, `scam`, `mixer`, `sanctioned`, `frozen`, `safe`, `exchange`,
`unknown`) в ленту не попадают (она ограничена 10 последними находками).
`source` находок — `live-blocks`.

Дедупликация: внутри прогона по `tx|sender|recipient`, на бэкенде — по
`signature` (UNIQUE в `scan_findings`).

## Массовая проверка адресов

```
POST /api/check/bulk
{"chain": "btc", "addresses": ["bc1q...", "1A1z..."]}
→ {"chain": "btc", "checked": 2, "found": 1,
   "results": [{"address": "...", "status": "hacker", "has_pk": false,
                "has_seed": false, "associated_hacker": false,
                "reason": "...", "source": "..."}]}
```

До 500 адресов за запрос. Принимаются канонические сети (`evm`, `btc`,
`solana`, `sui`, `tron`) и конкретные EVM-сети (`ethereum`, `bnb`, `base`,
`linea`, `arbitrum`, `polygon`, `optimism`, `avalanche` — нормализуются в
`evm`, адреса сравниваются без учёта регистра). В ответе — только адреса,
найденные в базе и имеющие один из отслеживаемых статусов (`hacked`,
`hacker`, `drained`, `phishing`, `suspicious`); остальные записи базы
не раскрываются.
