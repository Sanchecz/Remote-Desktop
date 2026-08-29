# Первый production-запуск RemoteIt

Эта инструкция разворачивает новый независимый экземпляр RemoteIt на чистом VDS. Она не содержит рабочих паролей, токенов, IP-адресов, SSH-ключей или приватных ключей подписи.

## 1. Требования

- Ubuntu 22.04/24.04 x86-64;
- 4 vCPU, 8 ГБ RAM и от 80 ГБ NVMe для обычной эксплуатации до 300 устройств;
- Docker Engine и Docker Compose v2;
- домен, A/AAAA-записи которого направлены на VDS;
- входящие TCP 80/443 и UDP 443; SSH настраивается владельцем VDS отдельно;
- установленный `openssl`; для создания Android-ключа нужен `keytool` из JDK.

## 2. Каталоги и исходники

Распакуйте release-архив в отдельный неизменяемый каталог. Символическая ссылка `current` позволяет откатиться без поиска файлов:

```sh
sudo install -d -m 0750 /opt/genesisit/releases /opt/genesisit/shared /opt/genesisit/backups
sudo install -d -m 0750 /opt/genesisit/releases/1.0.20
sudo tar -xzf remoteit-1.0.20-source.tar.gz -C /opt/genesisit/releases/1.0.20
sudo ln -sfn /opt/genesisit/releases/1.0.20 /opt/genesisit/current
cd /opt/genesisit/current
```

Проверьте SHA-256 до распаковки:

```sh
sha256sum -c remoteit-1.0.20-source.tar.gz.sha256
```

## 3. Закрытая конфигурация

Создайте рабочий файл только в `shared`, а не внутри Git/release-каталога:

```sh
sudo cp .env.example /opt/genesisit/shared/.env
sudo chmod 0600 /opt/genesisit/shared/.env
sudo openssl rand -hex 32
sudo openssl rand -hex 32
sudo openssl rand -base64 24
```

Запишите разные сгенерированные значения в:

- `POSTGRES_PASSWORD` — пароль БД;
- `REMOTEIT_ACTION_SIGNING_SECRET` — постоянный секрет подписанных административных заданий, минимум 32 символа;
- `GENESIS_ADMIN_PASSWORD` — временный пароль первого владельца;
- `REMOTEIT_PUBLIC_URL` — итоговый HTTPS-адрес;
- `OPENAI_API_KEY` — необязательный ключ для расширенного AI-объяснения.

`REMOTEIT_ACTION_SIGNING_SECRET` нельзя произвольно менять после установки Agent: изменение нарушит доверие уже зарегистрированных устройств.

## 4. Домен и TLS

Замените домены в `Caddyfile` на свои. Убедитесь, что DNS уже отвечает новым IP, затем проверьте конфигурацию:

```sh
docker run --rm -v "$PWD/Caddyfile:/etc/caddy/Caddyfile:ro" caddy:2-alpine caddy validate --config /etc/caddy/Caddyfile
```

Caddy автоматически выпустит и обновит TLS-сертификат после запуска.

## 5. Ключ подписи Android

Создайте новый приватный ключ только один раз и храните его вместе с резервными копиями:

```sh
sudo keytool -genkeypair -v \
  -keystore /opt/genesisit/shared/android-release.jks \
  -alias remoteit \
  -keyalg RSA -keysize 4096 -validity 10000
sudo chmod 0600 /opt/genesisit/shared/android-release.jks
```

Создайте `/opt/genesisit/shared/android-signing.env` с правами `0600`:

```dotenv
GENESIS_ANDROID_STORE_PASSWORD=replace-with-keystore-password
GENESIS_ANDROID_KEY_ALIAS=remoteit
GENESIS_ANDROID_KEY_PASSWORD=replace-with-key-password
```

Не публикуйте эти два файла в GitHub. Все будущие APK должны подписываться тем же ключом, иначе Android не установит обновление поверх существующего приложения.

## 6. Первая сборка и запуск

```sh
cd /opt/genesisit/current
sudo docker compose -p genesisit --env-file /opt/genesisit/shared/.env build app
sudo docker compose -p genesisit --env-file /opt/genesisit/shared/.env up -d
sudo docker compose -p genesisit --env-file /opt/genesisit/shared/.env ps
```

Проверьте уровни по порядку:

```sh
sudo docker compose -p genesisit --env-file /opt/genesisit/shared/.env exec -T app \
  wget -qO- http://127.0.0.1:8080/healthz
curl -fsS https://YOUR_DOMAIN/healthz
curl -fsS https://YOUR_DOMAIN/downloads/AGENT-RELEASE.json
curl -fsS https://YOUR_DOMAIN/downloads/SHA256SUMS.txt
```

Все контейнеры должны быть `healthy`, а версия `AGENT-RELEASE.json` — совпадать с релизом.

## 7. Первый вход и первое устройство

1. Откройте итоговый HTTPS-адрес.
2. Войдите с `GENESIS_ADMIN_USERNAME` и временным `GENESIS_ADMIN_PASSWORD`.
3. Сразу замените временный пароль в «Настройки».
4. Откройте «Токены» и создайте ограниченный токен установки: имя, группа, лимит устройств и срок.
5. Передайте пользователю публичную ссылку установки или скачайте готовый Agent со встроенным токеном.
6. После установки устройство автоматически появится в «Устройства» с именем, Remote ID, IP, ОС и статусом.
7. Проверьте терминал, файловую передачу и один управляемый удалённый сеанс на тестовом компьютере.

Не используйте один бессрочный токен для всех организаций: отдельные токены упрощают отзыв, аудит и ограничение числа установок.

## 8. Обновление без потери данных

Один раз установите штатные команды экспорта и ежедневного резервного копирования:

```sh
cd /opt/genesisit/current
sudo install -m 0750 ops/remoteit-export-state /usr/local/sbin/remoteit-export-state
sudo install -m 0750 ops/genesisit-backup /usr/local/sbin/genesisit-backup
sudo install -m 0644 ops/genesisit-backup.service ops/genesisit-backup.timer /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now genesisit-backup.timer
```

Перед обновлением создайте state-архив по [MIGRATION.md](MIGRATION.md) и проверьте созданные `.sha256`/`.contents.txt`. Новый release распаковывайте в новый каталог; `.env`, Android-ключ, PostgreSQL и Docker volumes не копируйте поверх исходников.

```sh
sudo /usr/local/sbin/remoteit-export-state /opt/genesisit/backups
sudo ln -sfn /opt/genesisit/releases/NEW_VERSION /opt/genesisit/current
cd /opt/genesisit/current
sudo docker compose -p genesisit --env-file /opt/genesisit/shared/.env build app
sudo docker compose -p genesisit --env-file /opt/genesisit/shared/.env up -d --no-deps app
```

Не выполняйте `docker compose down -v`, `docker system prune --volumes` или ручное удаление PostgreSQL volume. Agent обновляется автоматически только после проверки HTTPS, размера и SHA-256.

## 9. Откат

Сохраните ссылку на предыдущий release. При проблеме переключите `current` назад и пересоздайте только `app` из предыдущего кода:

```sh
sudo ln -sfn /opt/genesisit/releases/PREVIOUS_VERSION /opt/genesisit/current
cd /opt/genesisit/current
sudo docker compose -p genesisit --env-file /opt/genesisit/shared/.env up -d --no-deps --build app
```

Если релиз менял схему данных, используйте проверенный state-архив и полную процедуру восстановления из [MIGRATION.md](MIGRATION.md), а не частичное ручное редактирование БД.

## 10. Ежедневная проверка

- контейнеры `app`, `db`, `caddy` healthy;
- публичный `/healthz` отвечает без редирект-цикла;
- резервная копия создана сегодня и её SHA-256 проверяется;
- свободно не менее 20% диска;
- новые Agent имеют текущую версию;
- в журнале нет повторяющихся ошибок входа, обновления или регистрации;
- тестовый предпросмотр не уведомляет пользователя до первого управляющего действия;
- реальный удалённый сеанс уведомляет пользователя и корректно завершается.

## Создатель

RemoteIt — частный проект. Создатель: [@Sanchcz](https://t.me/Sanchcz).
