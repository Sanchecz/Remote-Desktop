# Runbook для Codex: 3x-ui, Happ, Telegram и OpenAI

Этот файл нужно передать Codex целиком при следующем обслуживании. Он фиксирует ограничения, подтверждённые регрессии и порядок безопасной работы.

## Неприкосновенные условия

1. Не менять SSH-пользователей, пароли и способ входа без прямого запроса владельца.
2. Не менять существующие URL подписок, UUID, `sub_id`, клиентов, inbounds и порты без отдельного согласования.
3. Перед production-изменением делать новый полный backup и отдельную SQLite-копию.
4. Не выполнять `docker compose down/up`, массовое пересоздание, cleanup, миграцию или широкий restart, если это не доказано необходимо.
5. Сначала читать фактическое состояние обоих серверов; локальная документация может устареть.
6. Не считать HTTP `200` полной проверкой: нужны TLS, Base64, содержимое подписки, Xray config, Telegram и клиентский Happ-тест.
7. Не публиковать raw-бэкапы, пока GitHub-репозиторий не подтверждён как `PRIVATE`.

## Стабильная точка `v1.0.2`

- 10 клиентов;
- 113 назначений клиент → inbound;
- 10 активных `sub_id`;
- `subUpdates=1`;
- `subEncrypt=true`;
- Xray `Configuration OK`;
- публичный server-side sniffing выключен;
- оба автоматических `openai-chain-*-healthcheck.timer` выключены;
- Telegram ранее подтверждён пользователем как рабочий;
- Happ iOS подтвердил полную подписку с каноническим routing без вылета.

## Критически важная схема доставки routing

Фактические настройки основного 3x-ui должны быть:

```text
subEnableRouting=true
subRoutingRules=<пустая строка>
subIncyEnableRouting=true
subIncyRoutingRules=happ://routing/add/<base64 canonical JSON>
```

3x-ui добавляет `subIncyRoutingRules` строкой в тело подписки перед Base64-кодированием. Happ официально понимает routing deeplink в теле. Поле используется как безопасный способ доставки строки, несмотря на историческое имя `Incy`.

Нельзя возвращать 30-КБ routing в HTTP-заголовок `subRoutingRules`: рабочий iOS-вариант подтверждён именно с телом и маленькими заголовками.

## Контракт канонического JSON

- 22 верхнеуровневых ключа;
- нет ключей, совпадающих без учёта регистра;
- только `Geoipurl` и `Geositeurl`, оба пустые;
- только `RemoteDNSIP`, `DomesticDNSIP`, `FakeDNS`;
- `GlobalProxy`, `UseChunkFiles`, `FakeDNS` — строки `"true"`/`"false"`;
- `DirectSites=469`, `ProxySites=18`, `BlockSites=431`, `DirectIp=11`;
- нет `geosite:*` и `geoip:*`;
- compact SHA: `9c1c83483fccdce7f7ddb127863b36699d766d407422d021b2e8cbd3c1fbfb9a`;
- deeplink SHA: `46f76e0a81e24d28e8f7bb492baeaab4779cda71adf8436b0d9b95f63fb4976b`.

Перед публикацией всегда запускать:

```bash
python3 scripts/build_happ_link.py routing/RoscomVPN.routing.json \
  --output /tmp/RoscomVPN.deeplink.txt --print-hashes
python3 scripts/verify_release.py
```

## Почему iOS раньше вылетал

Старый профиль содержал одновременно `GeoipUrl`/`Geoipurl` и `GeositeUrl`/`Geositeurl`, устаревшие варианты `RemoteDNSIp`, `DomesticDNSIp`, `FakeDns`, а также boolean вместо строковых значений. Отдельные профили работали; подписка без routing импортировалась; прежний routing в теле снова вызывал вылет; канонический полный routing заработал. Поэтому менять VPN-протоколы для этой ошибки не нужно.

## Что нельзя повторять

### Server-side sniffing

Не включать sniffing на публичных inbounds «для улучшения routing» или полной блокировки torrent. Ранее это коррелировало с зависанием Telegram в Happ. Xray protocol-rule для BitTorrent без sniffing не является полной DPI-блокировкой; текущая защита блокирует известные tracker/DHT-домены.

### Автоматические healthcheck-таймеры

Не включать прежние пятиминутные таймеры. Они меняли runtime Xray, добавляя/удаляя служебный inbound, и совпали с регрессией Telegram. Разрешён ручной запуск healthcheck без включения таймера.

### GEO-категории

Не возвращать `geosite:torrent`, `geosite:twitch-ads`, `geosite:whitelist`, `geoip:direct` без проверки фактических файлов Happ. Эти отсутствующие секции уже вызывали ошибки ядра. В стабильном профиле все правила явные.

## Backup-first процедура

На каждом узле:

```bash
bash scripts/portable-backup.sh main
bash scripts/portable-backup.sh node2
```

Запускать соответствующую роль только на соответствующем сервере. Затем проверить:

- `tar -tzf`;
- безопасные пути архива;
- `SNAPSHOT-SHA256SUMS.txt`;
- `DB-INTEGRITY.txt` и повторный `PRAGMA integrity_check`;
- `STATE.txt` и роль;
- локальный SHA после скачивания;
- `audit_portable_backup.sh` → `AUDIT_OK`.

## Минимальная проверка production

До и после изменения зафиксировать:

- PID x-ui и Xray;
- `systemctl is-active/is-enabled x-ui`;
- SQLite integrity;
- число clients/inbounds/assignments/sub_id;
- различия таблиц/настроек относительно backup;
- Xray `run -test`;
- все 10 подписок через публичный TLS endpoint;
- отсутствие `Routing` header;
- наличие `Routing-Enable: true`;
- одну routing-строку в расшифрованном теле;
- deeplink SHA и JSON-контракт;
- ручные healthchecks;
- клиентский iPhone refresh;
- Telegram: сообщения, истории, файлы, статус, повторное подключение;
- `.ru` direct и иностранный proxy;
- OpenAI авторизацию на обоих серверных профилях.

Не утверждать «100%», если не выполнен реальный тест телефона/авторизации.

## Выпуск новой версии

1. Проверить `git status -sb` и отделить чужие изменения.
2. Начать ветку `agent/3xui-happ-vX.Y.Z` от свежего `origin/main`.
3. Создать только новый каталог версии; старые выпуски не переписывать.
4. Извлечь routing из свежей backup-БД и проверить его по фактической подписке.
5. Не помещать raw `.tar.gz`, `.db`, `.key`, `.pem` в Git.
6. Собрать documentation kit и SHA manifest.
7. Проверить отсутствие IP серверов, SSH-паролей, UUID и subscription URL в коммите.
8. Commit → push → draft PR → проверки → merge.
9. Создать новый stable Release, добавить ZIP, оба raw-архива и release manifest.
10. Проверить private visibility, collaborators, invitations, asset sizes/digests, `draft=false`, `prerelease=false`.
11. Скачать manifest и хотя бы один небольшой asset обратно и сверить SHA.

Если репозиторий оказался публичным, немедленно остановить загрузку raw-assets и сообщить владельцу. Не пытаться «быстро закончить» публикацию.
