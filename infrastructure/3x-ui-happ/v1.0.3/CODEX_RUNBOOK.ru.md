# Runbook для Codex: 3x-ui, Happ и Telegram

Перед следующим обслуживанием передать Codex этот файл целиком.

## Неприкосновенные условия

1. Не менять SSH-пользователей, пароли и способ входа без прямого запроса.
2. Не менять существующие URL подписок, UUID, `sub_id`, клиентов, inbounds и порты без согласования.
3. Перед production-изменением делать полный portable backup и отдельную копию SQLite.
4. Не выполнять широкий restart, cleanup, миграцию или пересоздание сервисов без доказанной необходимости.
5. Сначала читать фактическое состояние обоих серверов; документация может устареть.
6. Не считать HTTP `200` полной проверкой: нужны TLS, Base64, routing, Xray config и реальный тест Happ.
7. Не публиковать raw backup, пока репозиторий не подтверждён как `PRIVATE`.

## Зафиксированная точка `v1.0.3`

- существующие ссылки и клиенты сохранены;
- `subUpdates=1`;
- Xray `Configuration OK`;
- публичный server-side sniffing выключен;
- автоматические healthcheck-таймеры выключены;
- routing доставляется одной строкой внутри тела подписки;
- межсерверный OpenAI/WireGuard-обход не активен;
- Telegram явно покрыт доменами и IPv4/IPv6;
- Happ Android имеет известную задержку первого Telegram-пакета после reconnect.

## Доставка routing

```text
subEnableRouting=true
subRoutingRules=<пустая строка>
subIncyEnableRouting=true
subIncyRoutingRules=happ://routing/add/<base64 canonical JSON>
```

Не возвращать большой routing в HTTP-заголовок: подтверждённый вариант доставляет его в Base64-теле подписки.

## Контракт `v1.0.3`

- 22 уникальных ключа;
- `DirectSites=469`, `DirectIp=12`;
- `ProxySites=44`, `ProxyIp=12`;
- `BlockSites=431`, `BlockIp=0`;
- `geoip:ru` разрешён;
- `Geoipurl`/`Geositeurl` пустые;
- отсутствуют проблемные `TORRENT`, `TWITCH-ADS`, `WHITELIST`, `DIRECT`;
- compact SHA: `239cf1eebc297157ff13fa06b665045562d9629c3146469bfec2b0c9fac962c4`;
- deeplink SHA: `09bd9a430450e4e7f3c439876a9a05c82000b90d4cb2a47550f8526a55909283`.

Проверка:

```bash
python3 scripts/build_happ_link.py routing/RoscomVPN.routing.json \
  --output /tmp/RoscomVPN.deeplink.txt --print-hashes
python3 scripts/verify_release.py
```

## Что нельзя повторять

- не включать sniffing на публичных inbounds ради Telegram или torrent;
- не включать прежние пятиминутные healthcheck-таймеры;
- не добавлять отсутствующие GEO-секции по одной;
- не возвращать межсерверный OpenAI/WireGuard-обход без отдельного backup и доказанного теста Telegram;
- не объявлять Telegram исправленным только потому, что сервер отвечает быстро.

## Backup-first

На соответствующем узле:

```bash
bash scripts/portable-backup.sh main
bash scripts/portable-backup.sh node2
```

Проверить tar paths, внутренние SHA, SQLite integrity, роль архива, локальный SHA после скачивания и `audit_portable_backup.sh` → `AUDIT_OK`.

## Минимальная production-проверка

- PID и состояние x-ui/Xray;
- SQLite integrity;
- число клиентов, inbounds, назначений и `sub_id` до/после;
- Xray `run -test`;
- все подписки через публичный TLS endpoint;
- отсутствие большого `Routing` header;
- один deeplink в расшифрованном теле;
- routing SHA и JSON-контракт;
- Happ refresh без ручных настроек;
- Telegram: сообщения, истории, файлы, статус и несколько reconnect;
- отдельные тесты Wi-Fi и мобильной сети;
- российский direct и зарубежный proxy.

При Android-диагностике отдельно измерять время до первого пакета в Xray. Если пакет не поступает десятки секунд, серверные изменения без новых доказательств не делать.

## Выпуск следующей версии

1. Отделить изменения от остального репозитория.
2. Создать новый каталог версии; старые релизы не переписывать.
3. Не коммитить `.tar.gz`, `.db`, `.key`, `.pem`.
4. Проверить отсутствие IP серверов, паролей, UUID и subscription URL в коммите.
5. Собрать kit и SHA manifest.
6. Commit → push → PR → merge.
7. Создать Release; при известных клиентских ограничениях использовать prerelease.
8. Проверить `PRIVATE`, collaborators, invitations, asset sizes и digests.
9. Скачать manifest и kit обратно и повторно сверить SHA.
