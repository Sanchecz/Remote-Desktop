# Проверки `v1.0.2`

## Production после клиентского подтверждения

- 3x-ui: `active`;
- Xray: тот же PID, `Configuration OK`;
- 10 уникальных активных подписок: HTTP `200`, TLS проверен;
- 113 назначений профилей сохранены;
- минимальное число профилей в подписке: 1;
- максимальное число профилей в подписке: 13;
- routing в HTTP-заголовке: отсутствует;
- routing в теле: ровно одна строка на подписку;
- `Routing-Enable: true`;
- `Profile-Update-Interval: 1`;
- SQLite: `integrity_check=ok`;
- ручной healthcheck основного служебного канала: `OK`;
- ручной healthcheck второго узла: OpenAI выходит через основной немецкий узел.

## Routing JSON

- ключей верхнего уровня: 22;
- уникальность без учёта регистра: да;
- `GlobalProxy`, `UseChunkFiles`, `FakeDNS`: строки;
- `DirectSites`: 469;
- `ProxySites`: 18;
- `BlockSites`: 431;
- `DirectIp`: 11;
- `geosite:*`/`geoip:*`: 0;
- `Geoipurl`/`Geositeurl`: пустые;
- компактный JSON SHA-256: `9c1c83483fccdce7f7ddb127863b36699d766d407422d021b2e8cbd3c1fbfb9a`;
- deeplink SHA-256: `46f76e0a81e24d28e8f7bb492baeaab4779cda71adf8436b0d9b95f63fb4976b`;
- форматированный JSON SHA-256: `95e7a30645a2f29f8e35ce62ca3b378748a76ab08af3eacf6401bb5e68b3e0d1`.

## Клиентский iPhone-тест

- подписка без routing импортировалась — это исключило VPN-профили как причину аварии;
- прежний неканонический routing в теле всё ещё вызывал вылет;
- полный канонический routing с теми же 469/18/431 правилами обновился без вылета;
- итог пользователя: «все ок».

## Post-fix архивы

Основной узел:

- файл: `portable-current-main-20260813T221137Z.tar.gz`;
- размер: 80 581 124 байта;
- SHA-256: `ba642ed81a5c135fae7e2388b38266425f92377980d444327498a41d5fbe4448`;
- tar paths: 369;
- внутренние SHA: успешно;
- SQLite: `ok`;
- внутри подтверждён deeplink SHA `46f76e...4976b`;
- активных клиентов: 10;
- результат: `AUDIT_OK role=main`.

Второй узел:

- файл: `portable-current-node2-20260813T221230Z.tar.gz`;
- размер: 80 563 383 байта;
- SHA-256: `111657b4bc7ea9a7ac7f915c27b46804760f335f75ae767cb16bbfb22c79b09f`;
- tar paths: 383;
- внутренние SHA: успешно;
- SQLite: `ok`;
- служебный expected-state сохранён;
- ручной OpenAI healthcheck: `OK`;
- результат: `AUDIT_OK role=node2`.

После копирования на локальную машину размеры и SHA-256 обоих архивов совпали с серверными значениями.

## GitHub-проверки

Перед загрузкой raw-assets необходимо подтвердить:

- репозиторий `PRIVATE`;
- архивы отсутствуют в Git diff/history;
- нет незавершённых приглашений;
- список collaborators ожидаемый;
- manifest release assets совпадает с локальными SHA;
- после публикации GitHub-side digest каждого asset совпадает;
- `draft=false`, `prerelease=false`.
