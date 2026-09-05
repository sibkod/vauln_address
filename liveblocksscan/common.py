"""Общая инфраструктура live-сканеров блоков (liveblocksscan).

Каждый сетевой скрипт (ethereum.py, bitcoin.py, solana.py, …) поллит
последние блоки своей сети, извлекает переводы (sender, recipient, amount),
массово проверяет адреса через POST /api/check/bulk и для движений средств
с участием адресов из базы угроз отправляет находки на
POST /api/admin/scanner/findings — они появляются в мониторинге (/monitor)
и в отчётах кошельков.

Семантика находок (совместима с solana_scan.py):
  * исходящий перевод С известного плохого адреса — движение украденного:
    verdict SUSPICIOUS, индикатор F1_DOWNSTREAM_TRANSFER, victim="",
    hacker=получатель, exposed=[отправитель] (получатель становится
    suspicious, дерево отчёта связывает выплату с отправителем);
  * входящий перевод НА известный плохой адрес — финансирование оператора:
    verdict SUSPICIOUS, индикатор L1_WATCHED_INFLOW, victim=отправитель,
    hacker=известный адрес, exposed=[отправитель] (отправитель получает
    флаг associated_hacker).

Учитываются ТОЛЬКО реальные переводы нативной монеты или токенов: вызовы
контрактов без движения средств (аппрувы, свапы, взаимодействие с дрейнером)
находок не порождают. Из одной транзакции рождается максимум одна находка —
переводы отсортированы по сумме (нативная монета раньше токенов), берётся
первый, задействующий отслеживаемый адрес; остальные переводы той же
транзакции пропускаются (все стороны уже известны через bulk check).

Конфигурация: переменные окружения VAULN_API_URL и ADMIN_API_KEY либо
флаги --api-url/--api-key (см. parse_args).
"""

import argparse
import json
import os
import re
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path


def load_dotenv():
    """Загрузить liveblocksscan/.env в окружение (не перезаписывая его).


    Если переменная уже задана (например, из шелла или системного unit),
    значение из .env не перекрывает её. Разделители: "=" и ";"  — то же,
    что поддерживает docker-compose. Поддержка простых комментариев "##".
    """
    root = Path(__file__).resolve().parent
    candidates = [root / ".env", Path.cwd() / ".env"]
    if os.environ.get("LIVEBLOCKS_ENV_DIR"):
        candidates.insert(0, Path(os.environ["LIVEBLOCKS_ENV_DIR"]) / ".env")
    seen = set()
    for path in candidates:
        try:
            with path.open("r", encoding="utf-8") as fh:
                lines = fh.readlines()
        except OSError:
            continue
        for raw in lines:
            line = raw.strip()
            if not line or line.startswith("#"):
                continue
            key, _, val = line.partition("=")
            key = key.strip()
            if not key or not re.match(r"^[A-Za-z_][A-Za-z0-9_]*$", key):
                continue
            if key in seen or key in os.environ:
                continue
            seen.add(key)
            val = val.strip()
            if len(val) >= 2 and val[0] == '"' and val[-1] == '"':
                val = val[1:-1].replace('""', '""')
            os.environ[key] = val


# Загружаем .env ДО вычисления констант ниже (watcher-скрипты импортируют
# common.py и уже на импорте получают правильные VAULN_API_URL/ADMIN_API_KEY).
load_dotenv()


DEFAULT_API_URL = os.environ.get("VAULN_API_URL", "http://127.0.0.1:8080")
ADMIN_API_KEY = os.environ.get("ADMIN_API_KEY", "")

# Массовая проверка ограничена 500 адресами на запрос на бэкенде.
BULK_BATCH = 400

# Статусы, движения которых попадают в мониторинг и отчёты. Только прямые
# угрозы: движения остальных статусов (vulnerable, scam, mixer, sanctioned,
# frozen, safe, exchange, unknown) находок не порождают — иначе лента
# мониторинга, ограниченная 10 последними находками, тонет в шуме.
BAD_STATUSES = {
    "hacked", "hacker", "drained", "phishing", "suspicious",
}

# Кого мониторить и с какой стороны:
#
#   OUTGOING_STATUSES — где получать сигнал "монеты уходят": жертвы
#     (drained / hacked / phishing) + hacker-хакер. Victim как отправитель —
#     отметка о том, куда ушли stolen; hacker как отправитель — dls отмывки.
#   INCOMING_STATUSES — кто финансирует адрес из базы: ТОЛЬКО hacker.
#     Входящие на victim-адрес (drained и др.) не мониторятся — у жертвы
#     уже ушли средства, её поток приема не значит compromise.
#
# suspicious один-off-F1-payout вышел — это шум и не нужен ни в одной
# стороне (ни outgoing, ни incoming); при повторной выплате он
# переопределяется в hacker и начинает покрываться по обоим направлениям.
OUTGOING_STATUSES = {
    "hacked", "drained", "phishing", "hacker",
}
INCOMING_STATUSES = {
    "hacker",
}

# «Нулевые» адреса mint/burn токенов. У такого перевода нет реального
# контрагента: находка по нему назвала бы victim/hacker нулевой адрес
# (и MarkAssociatedHacker записал бы его в базу), а заодно заняла бы
# слот единственной находки на транзакцию, затенив настоящее движение
# средств (например, оператор -> получатель в той же tx, что и mint).
ZERO_ADDRESSES = {
    "0x0000000000000000000000000000000000000000",  # EVM
    "T9yD14Nj9j7xAB4dbGeiX9h8unkKHxuWwb",           # Tron (base58)
}

USER_AGENT = "vauln-liveblocksscan/1.0"


class BlockUnavailable(Exception):
    """Блок ещё не добыли / узел его не отдал — пропускаем без ретрая."""


# ------------------------------------------------------------- HTTP helpers

def http_json(url, payload=None, headers=None, timeout=30):
    """GET (payload=None) или POST с JSON-телом; ответ — распарсенный JSON."""
    hdrs = {"User-Agent": USER_AGENT, "Accept": "application/json"}
    if headers:
        hdrs.update(headers)
    data = None
    if payload is not None:
        data = json.dumps(payload).encode()
        hdrs["Content-Type"] = "application/json"
    req = urllib.request.Request(url, data=data, headers=hdrs)
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        return json.load(resp)


class JsonRpc:
    """JSON-RPC клиент с round-robin ротацией эндпоинтов и ретраями."""

    def __init__(self, endpoints, timeout=30, retries=3):
        if not endpoints:
            raise ValueError("нужен хотя бы один RPC endpoint")
        self.endpoints = list(endpoints)
        self.timeout = timeout
        self.retries = retries
        self.idx = 0
        self.req_id = 0

    def _next(self):
        ep = self.endpoints[self.idx % len(self.endpoints)]
        self.idx += 1
        return ep

    def call(self, method, params=None):
        self.req_id += 1
        body = {"jsonrpc": "2.0", "id": self.req_id,
                "method": method, "params": params or []}
        last_err = None
        for _ in range(self.retries * len(self.endpoints)):
            ep = self._next()
            try:
                data = http_json(ep, body, timeout=self.timeout)
                if data.get("error"):
                    raise RuntimeError(f"RPC error: {data['error']}")
                return data.get("result")
            except (urllib.error.URLError, OSError, RuntimeError,
                    json.JSONDecodeError) as e:
                last_err = e
                time.sleep(0.3)
        raise RuntimeError(f"все RPC endpoints недоступны: {last_err}")


# -------------------------------------------------------------- API клиент

class ApiClient:
    """Доступ к бэкенду vauln-address: массовая проверка и постинг находок."""

    def __init__(self, api_url, api_key):
        self.api_url = api_url.rstrip("/")
        self.api_key = api_key

    def bulk_check(self, chain, addresses):
        """Вернуть {address: result} для адресов, найденных в базе угроз."""
        found = {}
        for i in range(0, len(addresses), BULK_BATCH):
            batch = addresses[i:i + BULK_BATCH]
            try:
                data = http_json(
                    self.api_url + "/api/check/bulk",
                    {"chain": chain, "addresses": batch}, timeout=30)
            except Exception as e:  # noqa: BLE001 — пропуск батча не фатален
                print(f"  !! bulk check failed: {e}", file=sys.stderr)
                continue
            # бэкенд отвечает адресом в исходном регистре (checksummed);
            # для EVM ключи словаря приводим к нижнему регистру — RPC
            # отдаёт адреса lowercase
            evm = data.get("chain") == "evm"
            for res in data.get("results") or []:
                addr = res.get("address")
                if addr:
                    found[addr.lower() if evm else addr] = res
        return found

    def post_finding(self, payload):
        headers = {"X-Admin-Key": self.api_key} if self.api_key else {}
        try:
            return http_json(
                self.api_url + "/api/admin/scanner/findings",
                payload, headers=headers, timeout=15)
        except Exception as e:  # noqa: BLE001
            print(f"  !! не удалось отправить находку в API: {e}",
                  file=sys.stderr)
            return None


# ----------------------------------------------------------- ядро watcher'а

class Transfer:
    """Одно движение средств внутри блока.

    is_token=True — перевод токена (ERC20/TRC20 и т.п.): amount в единицах
    токена, а не нативной монеты. Такие переводы внутри одной транзакции
    идут после нативных при выборе главной находки.
    is_nft=True — перевод NFT (ERC721): amount всегда 1, token_symbol —
    символ коллекции (может быть пустым).
    """

    __slots__ = ("tx", "sender", "recipient", "amount", "is_token",
                 "token_symbol", "is_nft")

    def __init__(self, tx, sender, recipient, amount, is_token=False,
                 token_symbol="", is_nft=False):
        self.tx = tx
        self.sender = sender or ""
        self.recipient = recipient or ""
        self.amount = amount or 0.0
        self.is_token = is_token
        self.token_symbol = token_symbol
        self.is_nft = is_nft


# ----------------------------------------------------- ERC20 Transfer логи

# keccak256("Transfer(address,address,uint256)") — topic0 события ERC20.
ERC20_TRANSFER_TOPIC = (
    "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef")

# Десятичные и символы популярных токенов — чтобы суммы в находках были
# читаемыми без лишних RPC-вызовов. Неизвестные токены разрешаются он-чейн
# через eth_call decimals()/symbol() (см. make_evm_watcher).
KNOWN_TOKENS = {
    # USDT
    "0xdac17f958d2ee523a2206206994597c13d831ec7": (6, "USDT"),
    "0x9702230a8ea53601f5cd2dc00fdbc13d4df4a8c7": (6, "USDT"),   # Avalanche
    "0xc7198437980c041c805a1edcba50c1ce5db95118": (6, "USDT.e"),  # Avalanche
    "0x55d398326f99059ff775485246999027b3197955": (18, "USDT"),  # BNB
    "0x8ac76a51cc950d9822d68b83fe1ad97b32cd580d": (18, "USDC"),  # BNB
    "0xde1e63dae0bf3b056cb0f86af5e01a53a90dd823": (6, "USDT"),   # Linea
    # USDC
    "0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48": (6, "USDC"),
    "0xb97ef9ef8734c71904d8002f8b6bc66dd9c48a6e": (6, "USDC"),   # Avalanche
    "0x833589fcd6edb6e08f4c7c32d4f71b54bda02913": (6, "USDC"),   # Base
    "0x0b2c639c533813f4aa9d7837caf62653d097ff85": (6, "USDC"),   # Optimism
    "0x7f5c764cbc14f9669b88837ca1490cca17c31607": (6, "USDC.e"),  # Optimism
    "0xaf88d065e77c8cc2239327c5edb3a432268e5831": (6, "USDC"),   # Arbitrum
    "0x3c499c542cef5e3811e1192ce70d8cc03d5c3359": (6, "USDC"),   # Polygon
    "0x2791bca1f2de4661ed88a30c99a7a9449aa84174": (6, "USDC.e"),  # Polygon
    "0x176211869ca2b568f2a7d4ee941e073a821ee1ff": (6, "USDC"),   # Linea
    # DAI
    "0x6b175474e89094c44da98b954eedeac495271d0f": (18, "DAI"),
    "0xd586e7f844cea2f87f50152665bcbc2c279d8d70": (18, "DAI.e"),  # Avalanche
    "0x50c5725949a6f0c72e6c4a641f24049a917db0cb": (18, "DAI"),   # Base
    "0xda10009cbd5d07dd0cecc66161fc93d7c9000da1": (18, "DAI"),   # OP/Arb
    "0x4af15ec2a0bd43db75dd04e62faa3b8ef36b00d5": (18, "DAI"),   # Linea
    "0x8f3cf7ad23cd3cadbd9735aff958023239c6a063": (18, "DAI"),   # Polygon
    # wrapped natives
    "0xc02aaa39b223fe8d0a0e5c4f27ead9083c756cc2": (18, "WETH"),
    "0xb31f66aa3c1e785363f0875a1b74e27b85fd66c7": (18, "WAVAX"),
    "0xbb4cdb9cbd36b01bd1cbaebf2de08d9173bc095c": (18, "WBNB"),
    "0x4200000000000000000000000000000000000006": (18, "WETH"),  # L2
    "0x82af49447d8a07e3bd95bd0d56f35241523fbab1": (18, "WETH"),  # Arbitrum
    "0x0d500b1d8e8ef31e21c99d1db9a6444d3adf1270": (18, "WPOL"),
    "0xe5d7c2a44ffddf6b295a15c148167daaaf5cf34f": (18, "WETH"),  # Linea
    # WBTC
    "0x2260fac5e5542a773aa44fbcfedf7c193bc2c599": (8, "WBTC"),
    "0x50b7545627a5162f82a992c33b87adc75187b218": (8, "WBTC.e"),  # Avalanche
}


def topic_to_address(topic):
    """Адрес из 32-байтовой темы лога (последние 20 байт)."""
    if not topic or len(topic) < 42:
        return ""
    return "0x" + topic[-40:].lower()


def decode_abi_string(hexdata):
    """Раскодировать string из результата eth_call (ABI или bytes32)."""
    try:
        raw = bytes.fromhex((hexdata or "0x")[2:])
    except ValueError:
        return ""
    if not raw:
        return ""
    # bytes32 (старые токены вроде MKR): нулевой «offset» и короткая строка
    if len(raw) == 32:
        raw = raw.rstrip(b"\x00")
    elif len(raw) >= 64:
        strlen = int.from_bytes(raw[32:64], "big")
        raw = raw[64:64 + strlen]
    else:
        return ""
    try:
        return raw.decode("utf-8", errors="replace").strip("\x00").strip()
    except UnicodeDecodeError:
        return ""


def extract_erc20_transfers(receipts, resolve=None):
    """Достать (tx_hash, from, to, amount) из логов Transfer рецептов блока.

    receipts — ответ eth_getBlockReceipts (список рецептов с transactionHash
    и logs). resolve(token_addr) -> (decimals, symbol) — обязателен для
    корректных сумм: без него берутся 18 десятичных (стандарт ERC20).
    Логи с 4 темами и пустым data — переводы NFT (ERC721: tokenId лежит
    в topics[3], а не в data): сумма всегда 1, Transfer помечается is_nft.
    """
    out = []
    for rc in receipts or []:
        tx_hash = rc.get("transactionHash") or ""
        for log in rc.get("logs") or []:
            topics = log.get("topics") or []
            if len(topics) < 3 or topics[0] != ERC20_TRANSFER_TOPIC:
                continue
            token = (log.get("address") or "").lower()
            try:
                raw = int(log.get("data") or "0x0", 16)
            except ValueError:
                raw = 0
            is_nft = False
            if raw <= 0:
                if len(topics) < 4:
                    continue
                is_nft = True  # ERC721: суммы нет, tokenId в topics[3]
            decimals, symbol = KNOWN_TOKENS.get(token, (18, ""))
            if resolve is not None and (token not in KNOWN_TOKENS):
                decimals, symbol = resolve(token)
            amount = 1.0 if is_nft else raw / (10 ** decimals)
            out.append(Transfer(tx_hash,
                                topic_to_address(topics[1]),
                                topic_to_address(topics[2]),
                                amount,
                                is_token=True, token_symbol=symbol,
                                is_nft=is_nft))
    return out


def make_evm_watcher(chain_label, endpoints):
    """(latest_fn, transfers_fn) для EVM-сети: нативные + ERC20 переводы.

    Реальные движения средств только: транзакции с value=0 без логов
    Transfer (аппрувы и прочие вызовы контрактов) не попадают в выдачу.
    ERC20-переводы читаются из eth_getBlockReceipts; если узел его не
    поддерживает — только нативные переводы (предупреждение в лог разово).
    """
    rpc = JsonRpc(endpoints)
    state = {"receipts_ok": None}  # None — неизвестно, True/False — проверено
    token_meta = dict(KNOWN_TOKENS)  # address -> (decimals, symbol), кэш

    def resolve_token(addr):
        """(decimals, symbol) токена; неизвестные — он-чейн decimals/symbol."""
        meta = token_meta.get(addr)
        if meta is None:
            decimals, symbol = 18, ""
            try:
                r = rpc.call("eth_call",
                             [{"to": addr, "data": "0x313ce567"}, "latest"])
                d = int(r, 16)
                if 0 <= d <= 36:
                    decimals = d
            except (RuntimeError, ValueError):
                pass
            try:
                r = rpc.call("eth_call",
                             [{"to": addr, "data": "0x95d89b41"}, "latest"])
                symbol = decode_abi_string(r)[:32]
            except RuntimeError:
                pass
            meta = (decimals, symbol)
            token_meta[addr] = meta
        return meta

    def latest():
        return int(rpc.call("eth_blockNumber"), 16)

    def transfers(height):
        block = rpc.call("eth_getBlockByNumber", [hex(height), True])
        if not block:
            raise BlockUnavailable(f"блок {height} не найден")
        out = []
        for tx in block.get("transactions") or []:
            tx_hash = tx.get("hash") or ""
            value = int(tx.get("value") or "0x0", 16)
            if value <= 0:
                continue  # чистый вызов контракта — не перевод
            out.append(Transfer(tx_hash,
                                (tx.get("from") or "").lower(),
                                (tx.get("to") or "").lower(),
                                value / 1e18))
        if state["receipts_ok"] is not False:
            try:
                receipts = rpc.call("eth_getBlockReceipts", [hex(height)])
                if receipts is None:
                    raise RuntimeError("eth_getBlockReceipts не поддерживается")
                state["receipts_ok"] = True
                out.extend(extract_erc20_transfers(receipts, resolve_token))
            except (RuntimeError, BlockUnavailable) as e:
                state["receipts_ok"] = False
                print(f"  !! {chain_label}: без ERC20-логов ({e}); "
                      f"только нативные переводы", file=sys.stderr, flush=True)
        return out

    return latest, transfers


def process_transfers(api, chain, height, transfers, posted):
    """Проверить участников переводов по базе и отправить находки.

    posted — set ключей "tx|sender|recipient" для дедупликации внутри прогона
    (бэкенд дополнительно дедупит по signature).
    """
    candidates = set()
    for t in transfers:
        if t.sender:
            candidates.add(t.sender)
        if t.recipient:
            candidates.add(t.recipient)
    if not candidates:
        return 0

    known = api.bulk_check(chain, sorted(candidates))
    known = {a: r for a, r in known.items()
             if r.get("status") in BAD_STATUSES}
    if not known:
        return 0

    def lookup(addr):
        # EVM-ключи уже lowercase (см. bulk_check); остальные сети — точное
        # совпадение, fallback на lower безопасен (ключи 0x-префиксные)
        return known.get(addr) or known.get(addr.lower())

    sent = 0
    seen_tx = set()  # из одной транзакции — максимум одна находка
    # главный перевод первым: самый крупный, нативная монета раньше токенов
    ordered = sorted(transfers, key=lambda t: (t.tx, t.is_token, -t.amount))
    for t in ordered:
        if t.tx in seen_tx:
            continue
        if t.sender in ZERO_ADDRESSES or t.recipient in ZERO_ADDRESSES:
            continue  # mint/burn: нулевой адрес — не контрагент
        sender_hit = lookup(t.sender) if t.sender else None
        recipient_hit = lookup(t.recipient) if t.recipient else None
        sender_tracked = (sender_hit is not None and
                          sender_hit.get("status") in OUTGOING_STATUSES)
        recipient_tracked = (recipient_hit is not None and
                             recipient_hit.get("status") in INCOMING_STATUSES)
        if not (sender_tracked or recipient_tracked):
            continue
        if not t.recipient or t.recipient == t.sender:
            continue
        key = f"{t.tx}|{t.sender}|{t.recipient}"
        if key in posted:
            continue
        posted.add(key)
        seen_tx.add(t.tx)
        if t.is_nft:
            token_ind = f"ERC721:{t.token_symbol or 'NFT'}"
        elif t.is_token:
            fallback = "TRC20" if chain == "tron" else "ERC20"
            token_ind = f"ERC20:{t.token_symbol or fallback}"
        else:
            token_ind = None

        if sender_tracked:
            status = sender_hit["status"]
            # известную сторону отправляем в регистре из базы (checksummed),
            # чтобы отчёты и статусы сходились с записью реестра
            sender_db = sender_hit.get("address") or t.sender
            indicators = ["F1_DOWNSTREAM_TRANSFER"]
            if token_ind:
                indicators.append(token_ind)
            payload = {
                "chain": chain,
                "signature": t.tx,
                "slot": height,
                "verdict": "SUSPICIOUS",
                "indicators": indicators,
                "victim_address": "",
                "hacker_address": t.recipient,
                "amount_sol": round(t.amount, 9),
                "programs": [],
                "exposed_addresses": [sender_db],
                "source": "live-blocks",
            }
            unit = (f"NFT {t.token_symbol}".strip() if t.is_nft
                    else t.token_symbol if t.is_token else "")
            print(f"  >>> ДВИЖЕНИЕ блок {height}: {t.sender[:16]}… ({status})"
                  f" -> {t.recipient[:16]}…  {t.amount:.6f} {unit}"
                  f" [{t.tx[:20]}…]", flush=True)
        else:
            status = recipient_hit["status"]
            recipient_db = recipient_hit.get("address") or t.recipient
            indicators = ["L1_WATCHED_INFLOW"]
            if token_ind:
                indicators.append(token_ind)
            payload = {
                "chain": chain,
                "signature": t.tx,
                "slot": height,
                "verdict": "SUSPICIOUS",
                "indicators": indicators,
                "victim_address": t.sender,
                "hacker_address": recipient_db,
                "amount_sol": round(t.amount, 9),
                "programs": [],
                "exposed_addresses": [t.sender],
                "source": "live-blocks",
            }
            unit = (f"NFT {t.token_symbol}".strip() if t.is_nft
                    else t.token_symbol if t.is_token else "")
            print(f"  >>> ПОПОЛНЕНИЕ блок {height}: {t.sender[:16]}… ->"
                  f" {t.recipient[:16]}… ({status})  {t.amount:.6f} {unit}"
                  f" [{t.tx[:20]}…]", flush=True)
        resp = api.post_finding(payload)
        if resp is not None:
            sent += 1
    return sent


def run(name, chain, poll_interval, latest_fn, transfers_fn,
        api_url=None, api_key=None, once=False, start=None):
    """Главный цикл: latest_fn() -> номер головы, transfers_fn(h) -> [Transfer].

    Стартует от текущей головы (или от start) и идёт только вперёд.
    """
    api = ApiClient(api_url or DEFAULT_API_URL,
                    api_key if api_key is not None else ADMIN_API_KEY)
    posted = set()
    print(f"[i] {name}: API {api.api_url}, сеть {chain}, "
          f"интервал {poll_interval}s", flush=True)

    last = start - 1 if start is not None else None
    while True:
        try:
            head = latest_fn()
        except Exception as e:  # noqa: BLE001 — сеть моргнула, ждём
            print(f"  !! {name}: ошибка получения головы: {e}",
                  file=sys.stderr, flush=True)
            time.sleep(poll_interval)
            continue
        if last is None:
            last = head - 1
        while last < head:
            nxt = last + 1
            try:
                transfers = transfers_fn(nxt)
            except BlockUnavailable:
                last = nxt
                continue
            except Exception as e:  # noqa: BLE001
                print(f"  !! {name}: блок {nxt} недоступен: {e}",
                      file=sys.stderr, flush=True)
                break  # повторим тот же блок на следующем круге
            hits = process_transfers(api, chain, nxt, transfers, posted)
            if hits:
                print(f"[+] {name}: блок {nxt}: находок отправлено {hits}",
                      flush=True)
            last = nxt
            if len(posted) > 200_000:  # не раздуваем память на длинных прогонах
                posted.clear()
        if once:
            return
        time.sleep(poll_interval)


def parse_args(name, default_interval):
    ap = argparse.ArgumentParser(description=f"{name}: live-мониторинг блоков")
    ap.add_argument("--api-url", default=DEFAULT_API_URL,
                    help="URL бэкенда (env VAULN_API_URL)")
    ap.add_argument("--api-key", default=ADMIN_API_KEY,
                    help="админ-ключ (env ADMIN_API_KEY)")
    ap.add_argument("--interval", type=float, default=default_interval,
                    help="пауза между опросами головы, сек")
    ap.add_argument("--start", type=int, default=None,
                    help="стартовать с конкретного блока (по умолчанию — с головы)")
    ap.add_argument("--once", action="store_true",
                    help="обработать доступные блоки один раз и выйти")
    return ap.parse_args()
