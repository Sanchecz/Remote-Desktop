# Состав переносимого релиза

Каждый релиз RemoteIt должен содержать:

- исходники `server`, `agent`, `web`, `mobile` и `installer`;
- `Dockerfile`, `docker-compose.yml`, `Caddyfile` и `.env.example` без секретов;
- операционные скрипты из `ops/`;
- `docs/MIGRATION.md`;
- готовые Windows/Linux/macOS агенты, Windows Setup/Console и Android APK;
- `AGENT-RELEASE.json`, `SHA256SUMS.txt` и общий `RELEASE-SHA256SUMS.txt`.

Версия исходников, Agent manifest, APK и опубликованных установщиков обязана совпадать. Архив исходников создаётся из очищенного дерева, а не из рабочей папки с `work/`, `.env`, ключами или тестовыми доказательствами.

Бизнес-состояние не входит в публичный релиз. Перед переносом оно создаётся отдельно командой `remoteit-export-state`; порядок восстановления описан в `MIGRATION.md`.
