# Проверки `v1.0.1`

## Серверная точка

`v1.0.1` использует неизменённые снимки стабильной точки `v1.0.0`:

- main snapshot: `2026-08-13T12:41:34Z`;
- node2 snapshot: `2026-08-13T12:42:36Z`;
- оба `tar -tzf`: успешно;
- SQLite обоих архивов: `integrity_check=ok`;
- внутренние SHA файлов snapshot: успешно;
- `audit_portable_backup.sh`: `AUDIT_OK` для `main` и `node2`;
- x-ui в снимках: `active`, `enabled`;
- healthcheck timers: `inactive`, `disabled`.

Исходные SHA-256 raw-архивов:

```text
main   30b5a27d2d48ad476c3b24c7852e7ed676ee218c74b5c355d5ee27b90cf55817
node2  b42cfb04d513dd49ea41aaf75bfb6b56b6a0508d70bc30932e8be994059884ad
```

## Routing

- Happ link SHA-256: `2e4ac2d88e30813e752c829e1cabfaeb3f0f8a50ab8f64e8f03492c54e5ebe4b`;
- canonical JSON SHA-256: `dbd532802b40020c8f51205ccfc5014a4c60a4516693dea0af069ec5eae7cba4`;
- formatted JSON SHA-256: `8451f44a26150c1158ede6f8c3db83f4c2da0991583bcbeeea0e1ef87a4af2e6`;
- `geosite:*`/`geoip:*`: 0;
- непустые GEO URL: 0;
- `DirectSites`: 469;
- `ProxySites`: 18;
- `BlockSites`: 431;
- `DirectIp`: 11.

## Подписки и клиентская проверка стабильной точки

- клиенты: 10;
- назначения: 113;
- HTTP 200: 10/10;
- актуальный routing: 10/10;
- интервал 1 час: 10/10;
- Xray: `Configuration OK`;
- пользователь подтвердил исправленную работу Telegram и отсутствие прежних GEO-ошибок.

## GitHub перед публикацией raw-assets

- репозиторий: `PRIVATE`;
- единственный collaborator: `Sanchecz`;
- pending invitations: 0;
- raw `.tar.gz` отсутствуют в staged Git diff;
- серверы и подписки при создании `v1.0.1` не изменялись.

После публикации необходимо повторно проверить те же три условия доступа, GitHub-side digest каждого asset и контрольную загрузку из Release.
