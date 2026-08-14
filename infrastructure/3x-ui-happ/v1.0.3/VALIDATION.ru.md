# Проверки `v1.0.3`

## Сервер и подписки

- x-ui/Xray: active после контролируемого применения routing;
- Xray: `Configuration OK`;
- SQLite: `integrity_check=ok`;
- 10 активных подписок: 10/10 HTTP `200` со строгим TLS;
- существующие URL, UUID, `sub_id`, клиенты, назначения и inbounds сохранены;
- routing в HTTP-заголовке отсутствует;
- в Base64-теле каждой подписки ровно одна строка `happ://routing/add/...`;
- `Routing-Enable: true`;
- `Profile-Update-Interval: 1`.

## Routing JSON

- верхнеуровневых ключей: 22;
- дубликатов ключей без учёта регистра: нет;
- `GlobalProxy`, `UseChunkFiles`, `FakeDNS`: строки;
- `DirectSites`: 469;
- `DirectIp`: 12;
- `ProxySites`: 44;
- `ProxyIp`: 12;
- `BlockSites`: 431;
- `BlockIp`: 0;
- обязательные Telegram domains и IPv4/IPv6 присутствуют;
- `geoip:ru` присутствует;
- проблемные `TORRENT`, `TWITCH-ADS`, `WHITELIST`, `DIRECT` отсутствуют;
- `Geoipurl`/`Geositeurl` пустые;
- compact SHA-256: `239cf1eebc297157ff13fa06b665045562d9629c3146469bfec2b0c9fac962c4`;
- deeplink SHA-256: `09bd9a430450e4e7f3c439876a9a05c82000b90d4cb2a47550f8526a55909283`.

## Телефон

- iPhone: полная подписка импортирована без прежнего вылета;
- Android: Happ подключает VPN/Xray, но первый трафик Telegram после reconnect иногда задерживается 65–91 секунду;
- после поступления первого пакета серверная обработка занимает около 0,2 секунды;
- задержка воспроизведена с официальными сборками Happ и несколькими TUN/профильными вариантами;
- Amnezia восстанавливает Telegram быстрее в сопоставимом сценарии.

Вывод: сервер и подписка проходят проверки; мгновенный reconnect Telegram в Happ Android не подтверждён.

## Архивы восстановления

Два raw-архива созданы непосредственно перед изменением routing и прошли первоначальный аудит. Они используются как полная переносимая база, а актуальный routing `v1.0.3` хранится отдельно в kit. После восстановления routing нужно применить из `routing/RoscomVPN.routing.json` и повторить проверки этой страницы.
