#!/usr/bin/env python3
"""Токен VK Ads по схеме ТП ZaleyCash.

1) Секрет из настроек ZaleyCash → POST /api/v2/token
2) С этим Bearer → POST /api/v2/vk_advert/token
   В account_id нужно ИМЯ кабинета, не числовой id.
   Пример ТП: {"account_id": "Пример_Никита Пак Статуи"}
3) Полученным vk access_token логинимся в ads.vk.com
   (GET /api/v2/user.json) — без этого запросы в ВКР не пойдут.

Примеры:
  python zaley_vk_token.py --secret SECRET --list
  python zaley_vk_token.py --secret SECRET --account "Юлия тесты"
  python zaley_vk_token.py --secret SECRET --account "Юлия тесты" --check

Секрет можно положить в ZALEY_SECRET.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import urllib.error
import urllib.request
from typing import Any

ZALEY_BASE = "https://zaleycash.com"
VK_ADS_BASE = "https://ads.vk.com"


class ApiError(RuntimeError):
    def __init__(self, message: str, payload: dict[str, Any] | None = None) -> None:
        super().__init__(message)
        self.payload = payload or {}


def request_json(
    method: str,
    url: str,
    token: str,
    body: dict[str, Any] | None = None,
) -> dict[str, Any]:
    data = None if body is None else json.dumps(body).encode("utf-8")
    req = urllib.request.Request(
        url,
        data=data,
        method=method,
        headers={
            "Authorization": f"Bearer {token}",
            "Accept": "application/json",
            "Content-Type": "application/json",
        },
    )
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            raw = resp.read().decode("utf-8")
            status = resp.status
    except urllib.error.HTTPError as exc:
        raw = exc.read().decode("utf-8", errors="replace")
        try:
            payload = json.loads(raw) if raw else {}
        except json.JSONDecodeError:
            payload = {"raw": raw}
        raise ApiError(
            f"HTTP {exc.code} {url}: {payload.get('message') or raw or exc.reason}",
            payload,
        ) from exc
    except urllib.error.URLError as exc:
        raise ApiError(f"Сеть: {exc.reason}") from exc

    try:
        payload = json.loads(raw) if raw else {}
    except json.JSONDecodeError as exc:
        raise ApiError(f"Не JSON в ответе {url} (HTTP {status}): {raw[:300]}") from exc
    return payload


def zaley_request(
    method: str,
    path: str,
    token: str,
    body: dict[str, Any] | None = None,
) -> dict[str, Any]:
    payload = request_json(method, ZALEY_BASE + path, token, body)
    code = payload.get("code")
    if code not in (None, 200):
        raise ApiError(
            f"ZaleyCash code={code} {path}: {payload.get('message', payload)}",
            payload,
        )
    return payload


def token_from_response(payload: dict[str, Any]) -> str:
    body = payload.get("response") or {}
    if not isinstance(body, dict):
        return ""
    return str(body.get("access_token") or body.get("accessToken") or "").strip()


def get_zaley_token(secret: str) -> dict[str, Any]:
    payload = zaley_request("POST", "/api/v2/token", secret)
    token = token_from_response(payload)
    if not token:
        raise ApiError("В ответе /api/v2/token нет access_token", payload)
    body = payload["response"]
    if "access_token" not in body:
        body["access_token"] = token
    return body


def list_vk_accounts(zaley_token: str) -> list[dict[str, Any]]:
    payload = zaley_request("GET", "/api/v2/user/accounts/list?type=vk", zaley_token)
    accounts = payload.get("response") or []
    if not isinstance(accounts, list):
        raise ApiError("Ожидался список аккаунтов", payload)
    return accounts


def get_vk_ads_token(zaley_token: str, account_name: str) -> dict[str, Any]:
    # Как в письме ТП: в account_id именно название кабинета.
    payload = zaley_request(
        "POST",
        "/api/v2/vk_advert/token",
        zaley_token,
        {"account_id": account_name},
    )
    token = token_from_response(payload)
    if not token:
        raise ApiError("В ответе /api/v2/vk_advert/token нет access_token", payload)
    body = payload["response"]
    if isinstance(body, dict) and "access_token" not in body:
        body["access_token"] = token
    return body


def login_vk_ads(vk_token: str) -> dict[str, Any]:
    payload = request_json("GET", f"{VK_ADS_BASE}/api/v2/user.json", vk_token)
    if payload.get("error"):
        raise ApiError(f"VK Ads отклонил токен: {payload}", payload)
    return payload


def print_json(data: Any) -> None:
    json.dump(data, sys.stdout, ensure_ascii=False, indent=2)
    sys.stdout.write("\n")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Токен VK Ads через ZaleyCash: секрет → токен ЗК → токен ВКР по названию кабинета",
    )
    parser.add_argument(
        "--secret",
        default=os.environ.get("ZALEY_SECRET", ""),
        help="Секретный ключ из настроек ZaleyCash (или env ZALEY_SECRET)",
    )
    parser.add_argument(
        "--list",
        action="store_true",
        help="Список кабинетов. В --account нужно поле login",
    )
    parser.add_argument(
        "--account",
        help='Название кабинета, как в ZaleyCash. Пример: "Юлия тесты"',
    )
    parser.add_argument(
        "--check",
        action="store_true",
        help="После получения токена залогиниться в ads.vk.com (GET /api/v2/user.json)",
    )
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    if not args.secret:
        print("Нужен --secret или переменная ZALEY_SECRET", file=sys.stderr)
        return 2
    if not args.list and not args.account:
        print("Укажи --list или --account \"Название кабинета\"", file=sys.stderr)
        return 2

    try:
        zaley = get_zaley_token(args.secret.strip())
        zaley_token = zaley["access_token"]

        if args.list:
            accounts = list_vk_accounts(zaley_token)
            print_json(
                {
                    "zaley_expires_at": zaley.get("expires_at"),
                    "hint": "В --account передавай login активного кабинета",
                    "accounts": [
                        {
                            "login": item.get("login"),
                            "status": item.get("status"),
                            "id": item.get("id"),
                            "agency_id": item.get("agency_id"),
                            "balance": item.get("balance"),
                        }
                        for item in accounts
                    ],
                }
            )
            if not args.account:
                return 0

        vk = get_vk_ads_token(zaley_token, args.account.strip())
        result: dict[str, Any] = {
            "account_id": args.account.strip(),
            "zaley_expires_at": zaley.get("expires_at"),
            "vk_access_token": vk["access_token"],
            "vk_expires_at": vk.get("expires_at"),
        }

        if args.check:
            result["vk_user"] = login_vk_ads(vk["access_token"])

        print_json(result)
        return 0
    except ApiError as exc:
        print(str(exc), file=sys.stderr)
        if exc.payload:
            print_json(exc.payload)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
