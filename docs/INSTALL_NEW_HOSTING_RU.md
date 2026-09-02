# Установка RemoteIt на новый хостинг

Эта инструкция — единая точка входа для развёртывания RemoteIt на новом VDS. Она подходит для двух разных задач:

1. **Чистая установка** — новая пустая система с новым владельцем и новой базой.
2. **Перенос действующего RemoteIt** — сохраняются пользователи, роли, устройства, Remote ID, токены, журнал, настройки, ключи Agent, Android-подпись и незавершённые передачи.

В репозитории нет и не должно быть рабочих паролей, токенов, дампов БД, SSH-ключей, `.env` или закрытого Android-ключа. Для переноса действующей системы исходников из Git недостаточно: нужен отдельный state-архив со старого сервера, хранящийся в зашифрованном виде.

## Быстрый выбор сценария

| Задача | Что использовать |
|---|---|
| Запустить новый независимый RemoteIt | Раздел «Чистая установка» ниже |
| Перенести текущий сервис без смены Remote ID | Раздел «Перенос действующей системы» |
| Обновить код на уже работающем VDS | [PRODUCTION_DEPLOYMENT_RU.md](PRODUCTION_DEPLOYMENT_RU.md), раздел «Обновление» |
| Восстановиться после аварии | [MIGRATION.md](MIGRATION.md) и проверенный state-архив |

## 1. Требования к новому VDS

Рекомендуемая конфигурация для обычной эксплуатации до 300 устройств:

- Ubuntu Server 22.04 или 24.04 x86-64;
- 4 vCPU, 8 ГБ RAM и не менее 80 ГБ NVMe;
- публичный IPv4;
- домен с доступом к изменению DNS;
- Docker Engine и Docker Compose v2;
- `git`, `curl`, `openssl` и `keytool` из JDK 17;
- входящие TCP `80` и `443`, UDP `443`; SSH-порт должен быть доступен только согласно политике владельца сервера;
- исходящий HTTPS-доступ для загрузки Docker-образов и зависимостей сборки.

PostgreSQL, `guacd`, Guacamole и порт приложения наружу не публикуются. В интернет выставляется только Caddy на `80/443`.

Проверьте ресурсы до начала:

```sh
uname -m
df -h /
free -h
```

Ожидаемая архитектура — `x86_64`. На диске перед первой полной сборкой желательно иметь не менее 30 ГБ свободного места.

## 2. Подготовка Ubuntu и Docker

Войдите по SSH под пользователем с `sudo` и установите базовые пакеты:

```sh
sudo apt-get update
sudo apt-get install -y ca-certificates curl git openssl openjdk-17-jre-headless
```

Установите Docker Engine и Compose plugin из [официального репозитория Docker для Ubuntu](https://docs.docker.com/engine/install/ubuntu/):

```sh
sudo install -m 0755 -d /etc/apt/keyrings
sudo curl -fsSL https://download.docker.com/linux/ubuntu/gpg \
  -o /etc/apt/keyrings/docker.asc
sudo chmod a+r /etc/apt/keyrings/docker.asc

. /etc/os-release
printf '%s\n' \
  'Types: deb' \
  'URIs: https://download.docker.com/linux/ubuntu' \
  "Suites: ${UBUNTU_CODENAME:-$VERSION_CODENAME}" \
  'Components: stable' \
  "Architectures: $(dpkg --print-architecture)" \
  'Signed-By: /etc/apt/keyrings/docker.asc' \
  | sudo tee /etc/apt/sources.list.d/docker.sources >/dev/null

sudo apt-get update
sudo apt-get install -y \
  docker-ce \
  docker-ce-cli \
  containerd.io \
  docker-buildx-plugin \
  docker-compose-plugin
```

После установки обязательно должны проходить обе команды:

```sh
sudo docker version
sudo docker compose version
```

Требуется именно Compose v2 с командой `docker compose`, а не старый отдельный `docker-compose`.

Создайте постоянную структуру каталогов:

```sh
sudo install -d -m 0750 /opt/genesisit
sudo install -d -m 0750 /opt/genesisit/releases
sudo install -d -m 0700 /opt/genesisit/shared
sudo install -d -m 0700 /opt/genesisit/backups
sudo install -d -m 0700 /opt/genesisit/exports
```

Назначение каталогов:

- `/opt/genesisit/releases/<версия>` — неизменяемые исходники конкретного выпуска;
- `/opt/genesisit/current` — ссылка на активный выпуск;
- `/opt/genesisit/shared` — закрытая конфигурация и Android signing key;
- `/opt/genesisit/backups` — ежедневные резервные копии;
- `/opt/genesisit/exports` — полные state-архивы для переноса.

## 3. DNS и сетевой доступ

Для текущего проекта рекомендуемый бесшовный вариант — сохранить домен `supportgenesis.ru` и изменить только его DNS A-запись на новый IPv4.

Перед переносом рабочего сервиса:

1. заранее уменьшите TTL DNS, например до 300 секунд;
2. не создавайте AAAA-запись, если IPv6 на новом VDS не настроен и не проверен;
3. откройте TCP `80/443` и UDP `443`;
4. старый VDS не удаляйте до завершения всех проверок и окончания окна отката.

Для нового домена потребуется одновременно изменить:

- `REMOTEIT_PUBLIC_URL` в закрытом `.env`;
- оба имени хоста в `Caddyfile`;
- DNS A/AAAA-записи;
- ссылки и установочные пакеты, содержащие публичный URL.

Смена только домена без переходного периода потребует обновления старых Agent. Самый надёжный перенос действующей системы — оставить прежний домен и поменять его IP.

## 4. Получение исходников

Текущий production-релиз — `1.0.43`. Для воспроизводимой установки используйте именно тег, а не произвольное состояние ветки:

```sh
REMOTEIT_VERSION=1.0.43
sudo git clone \
  --branch "remoteit-v${REMOTEIT_VERSION}" \
  --depth 1 \
  https://github.com/Sanchecz/Remote-Desktop.git \
  "/opt/genesisit/releases/${REMOTEIT_VERSION}"
sudo ln -sfn "/opt/genesisit/releases/${REMOTEIT_VERSION}" /opt/genesisit/current
cd /opt/genesisit/current
```

Убедитесь, что получен ожидаемый тег и дерево не изменено:

```sh
sudo git describe --tags --exact-match
sudo git status --short
```

Первая команда должна вывести `remoteit-v1.0.43`, вторая — ничего.

Вместо `git clone` можно использовать архив GitHub Release. Для `1.0.43` загрузите `RemoteIt-source-1.0.43.tar.gz` и `DELIVERABLES-SHA256.txt` из [страницы выпуска](https://github.com/Sanchecz/Remote-Desktop/releases/tag/remoteit-v1.0.43), затем проверьте именно строку исходного архива:

```sh
grep 'RemoteIt-source-1.0.43.tar.gz$' DELIVERABLES-SHA256.txt | sha256sum -c -
sudo install -d -m 0750 /opt/genesisit/releases/1.0.43
sudo tar -xzf RemoteIt-source-1.0.43.tar.gz -C /opt/genesisit/releases/1.0.43
sudo ln -sfn /opt/genesisit/releases/1.0.43 /opt/genesisit/current
```

Не продолжайте установку при несовпадении SHA-256.

# Чистая установка

Используйте этот раздел только для новой системы, в которую не требуется переносить данные со старого RemoteIt.

## 5. Создание закрытого `.env`

Скопируйте только шаблон:

```sh
sudo cp /opt/genesisit/current/.env.example /opt/genesisit/shared/.env
sudo chmod 0600 /opt/genesisit/shared/.env
sudoedit /opt/genesisit/shared/.env
```

Для генерации независимых значений используйте отдельную команду для каждого секрета:

```sh
openssl rand -hex 32
openssl rand -hex 32
openssl rand -hex 16
openssl rand -hex 24
```

Заполните в `/opt/genesisit/shared/.env`:

```dotenv
POSTGRES_DB=genesisit
POSTGRES_USER=genesisit
POSTGRES_PASSWORD=<первое-случайное-значение>
GENESIS_ADMIN_USERNAME=<логин-первого-владельца>
GENESIS_ADMIN_PASSWORD=<временный-случайный-пароль>
GENESIS_COOKIE_SECURE=true
REMOTEIT_PUBLIC_URL=https://<ваш-домен>
REMOTEIT_ACTION_SIGNING_SECRET=<второе-случайное-значение-не-короче-32-символов>
REMOTEIT_GUACAMOLE_JSON_SECRET=<ровно-32-шестнадцатеричных-символа>
OPENAI_API_KEY=
OPENAI_MODEL=gpt-5.6-terra
```

Правила:

- все четыре значения должны быть разными;
- `REMOTEIT_GUACAMOLE_JSON_SECRET` создаётся командой `openssl rand -hex 16` и содержит ровно 32 шестнадцатеричных символа;
- `GENESIS_ADMIN_PASSWORD` используется только для первого входа и после входа должен быть заменён;
- `REMOTEIT_ACTION_SIGNING_SECRET` нельзя произвольно менять после регистрации Agent;
- `OPENAI_API_KEY` необязателен; пустое значение не мешает основной работе RemoteIt;
- не копируйте готовый `.env` в Git и не отправляйте его в чат.

## 6. Ключ подписи Android

Для совершенно новой независимой установки создайте ключ один раз:

```sh
sudo keytool -genkeypair -v \
  -keystore /opt/genesisit/shared/android-release.jks \
  -alias remoteit \
  -keyalg RSA \
  -keysize 4096 \
  -validity 10000
sudo chmod 0600 /opt/genesisit/shared/android-release.jks
```

Создайте `/opt/genesisit/shared/android-signing.env` с правами `0600`:

```sh
sudoedit /opt/genesisit/shared/android-signing.env
sudo chmod 0600 /opt/genesisit/shared/android-signing.env
```

Содержимое:

```dotenv
GENESIS_ANDROID_STORE_PASSWORD=<пароль-хранилища>
GENESIS_ANDROID_KEY_ALIAS=remoteit
GENESIS_ANDROID_KEY_PASSWORD=<пароль-ключа>
```

Не создавайте новый Android-ключ при переносе или обычном обновлении. Иначе Android не установит новую APK поверх уже установленной версии. Файлы `android-release.jks` и `android-signing.env` должны входить в защищённую резервную копию, но никогда не в Git.

## 7. Домен в Caddy

Если используется `supportgenesis.ru`, текущий `Caddyfile` уже настроен. Для другого домена замените в нём оба имени хоста и адрес перенаправления, затем проверьте синтаксис:

```sh
cd /opt/genesisit/current
sudo docker run --rm \
  -v "$PWD/Caddyfile:/etc/caddy/Caddyfile:ro" \
  caddy:2-alpine \
  caddy validate --config /etc/caddy/Caddyfile
```

Caddy сам получает и обновляет TLS-сертификаты после запуска. DNS должен уже указывать на новый сервер, а порты `80/443` должны быть доступны извне.

## 8. Первая сборка и запуск

Во всех командах сохраняйте фиксированное имя Compose-проекта `genesisit`:

```sh
cd /opt/genesisit/current
sudo docker compose \
  -p genesisit \
  --env-file /opt/genesisit/shared/.env \
  build --pull app
sudo docker compose \
  -p genesisit \
  --env-file /opt/genesisit/shared/.env \
  up -d
```

Первая сборка загружает toolchain для Web, Go, Windows Agent и двух Android APK, поэтому занимает заметное время и место. Не прерывайте её, пока Docker продолжает выводить шаги сборки.

Проверьте контейнеры:

```sh
sudo docker compose \
  -p genesisit \
  --env-file /opt/genesisit/shared/.env \
  ps
```

Ожидаются сервисы `app`, `db`, `caddy`, `guacd`, `guacamole`; `transfers-init` должен завершиться с кодом `0`. `app` и `db` должны стать `healthy`.

## 9. Обязательная проверка после запуска

Проверяйте систему слоями, а не только по одному ответу HTTP `200`.

### Контейнеры и БД

```sh
cd /opt/genesisit/current
sudo docker compose -p genesisit --env-file /opt/genesisit/shared/.env ps
sudo docker compose -p genesisit --env-file /opt/genesisit/shared/.env exec -T db \
  sh -c 'pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB"'
sudo docker compose -p genesisit --env-file /opt/genesisit/shared/.env exec -T app \
  wget -qO- http://127.0.0.1:8080/healthz
```

### Публичный HTTPS и файлы выпуска

```sh
curl -fsS https://<ваш-домен>/healthz
curl -fsS https://<ваш-домен>/downloads/AGENT-RELEASE.json
curl -fsS https://<ваш-домен>/downloads/SHA256SUMS.txt
```

Проверьте контрольные суммы внутри контейнера:

```sh
sudo docker compose -p genesisit --env-file /opt/genesisit/shared/.env exec -T app \
  sh -c 'cd /app/web/downloads && sha256sum -c SHA256SUMS.txt'
```

### Функциональная проверка

1. Откройте панель по HTTPS.
2. Войдите под `GENESIS_ADMIN_USERNAME` и временным паролем.
3. Сразу смените временный пароль владельца.
4. Создайте отдельный ограниченный установочный токен.
5. Установите Agent на тестовое устройство и дождитесь Remote ID.
6. Проверьте статус устройства, терминал, загрузку и скачивание небольшого файла.
7. Проверьте один сеанс удалённого экрана и его корректное завершение.
8. Если используются RDP/SSH, проверьте отдельный тестовый узел через Agent-туннель.

Ответ `/healthz` подтверждает здоровье API, но не заменяет проверку входа, Agent и реального удалённого сеанса.

# Перенос действующей системы

Этот сценарий сохраняет существующие Remote ID и настройки. Не создавайте новый `.env` и новый Android-ключ.

## 10. Экспорт на старом VDS

Установите штатный экспортёр и создайте state-архив в согласованное окно обслуживания:

```sh
cd /opt/genesisit/current
sudo install -m 0750 ops/remoteit-export-state /usr/local/sbin/remoteit-export-state
sudo /usr/local/sbin/remoteit-export-state /opt/genesisit/exports
```

Экспорт кратковременно останавливает только `app`, делает согласованный dump PostgreSQL, архивирует `/opt/genesisit/shared` и volume передач, проверяет внутренние контрольные суммы и снова запускает `app`.

Команда выведет два пути:

- `remoteit-state-<дата>.tar.gz` — закрытый state-архив;
- `remoteit-state-<дата>.tar.gz.sha256` — его контрольная сумма.

Проверьте архив на старом сервере:

```sh
cd /opt/genesisit/exports
sudo sha256sum -c remoteit-state-<дата>.tar.gz.sha256
```

Скопируйте **оба** файла на новый VDS по SSH/SFTP или через закрытое хранилище. Архив равнозначен полному доступу к системе: права `0600`, без публичных ссылок и без загрузки в GitHub.

## 11. Подготовка пустой цели

На новом VDS выполните разделы 1–4 этой инструкции: установите Docker, создайте каталоги, получите тот же или более новый совместимый релиз и настройте `/opt/genesisit/current`.

Не выполняйте разделы создания `.env` и Android-ключа — их восстановит state-архив.

Цель должна быть пустой. До восстановления не должны существовать контейнеры или volumes с префиксом `genesisit_`/`genesisit-`:

```sh
sudo docker ps -a --format '{{.Names}}' | grep '^genesisit-' || true
sudo docker volume ls --format '{{.Name}}' | grep '^genesisit_' || true
```

Если команды что-то вывели, не удаляйте данные вслепую: сначала установите, относится ли найденное к неудачной тестовой попытке или к нужной системе.

## 12. Восстановление на новом VDS

Проверьте внешнюю контрольную сумму перед распаковкой:

```sh
cd /root
sudo sha256sum -c remoteit-state-<дата>.tar.gz.sha256
```

Установите и запустите штатный сценарий восстановления:

```sh
cd /opt/genesisit/current
sudo install -m 0750 ops/remoteit-restore-state /usr/local/sbin/remoteit-restore-state
sudo /usr/local/sbin/remoteit-restore-state \
  /root/remoteit-state-<дата>.tar.gz \
  --confirm-empty-target
```

Сценарий самостоятельно:

- проверит внутренний `MANIFEST.sha256`;
- восстановит закрытый `/opt/genesisit/shared`;
- создаст чистые Docker volumes;
- восстановит PostgreSQL и файлы передач;
- запустит весь Compose-проект.

После этого выполните все проверки из раздела 9. Дополнительно сравните:

- владельца и администраторов;
- число устройств и их Remote ID;
- токены, группы, журнал и настройки;
- Android certificate SHA-256;
- один тестовый Agent, удалённый экран, RDP/SSH и файловую передачу.

## 13. Переключение DNS без «двух серверов»

Не держите два активных экземпляра одной восстановленной системы дольше проверки: Agent не должен одновременно работать с двумя расходящимися базами.

Безопасная последовательность:

1. восстановить и проверить новый VDS локально настолько, насколько позволяет DNS;
2. остановить только `app` на старом VDS либо закрыть ему внешний трафик;
3. заменить DNS A-запись на новый IP;
4. дождаться обновления DNS и выпуска TLS;
5. повторить публичные и функциональные проверки на новом VDS;
6. наблюдать журналы и подключения Agent;
7. сохранить старый VDS выключенным, но не удалять до окончания согласованного периода отката.

Если сохранены домен, база и секреты, Agent переподключатся автоматически и сохранят Remote ID; переустановка на пользовательских компьютерах не требуется.

## 14. Ежедневные резервные копии и очистка безопасного кэша

После успешной установки включите штатные таймеры:

```sh
cd /opt/genesisit/current
sudo install -m 0750 ops/genesisit-backup /usr/local/sbin/genesisit-backup
sudo install -m 0644 \
  ops/genesisit-backup.service \
  ops/genesisit-backup.timer \
  ops/genesisit-docker-maintenance.service \
  ops/genesisit-docker-maintenance.timer \
  /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now genesisit-backup.timer
sudo systemctl enable --now genesisit-docker-maintenance.timer
sudo systemctl list-timers 'genesisit-*'
```

Проверьте резервное копирование вручную один раз:

```sh
sudo /usr/local/sbin/genesisit-backup
sudo ls -lah /opt/genesisit/backups
```

Штатное обслуживание удаляет только старые неиспользуемые image/build-данные по возрасту и сохраняет резерв BuildKit-кэша. Volumes с PostgreSQL и передачами оно не удаляет.

## 15. Обновление и откат кода

Новый выпуск всегда размещайте в новом каталоге. Перед переключением создайте state-архив:

```sh
sudo /usr/local/sbin/remoteit-export-state /opt/genesisit/exports
```

После получения `/opt/genesisit/releases/<новая-версия>`:

```sh
sudo ln -sfn /opt/genesisit/releases/<новая-версия> /opt/genesisit/current
cd /opt/genesisit/current
sudo docker compose -p genesisit --env-file /opt/genesisit/shared/.env build app
sudo docker compose -p genesisit --env-file /opt/genesisit/shared/.env up -d --no-deps app
```

Если проверка кода не прошла, верните symlink на предыдущий каталог и пересоздайте только `app`:

```sh
sudo ln -sfn /opt/genesisit/releases/<предыдущая-версия> /opt/genesisit/current
cd /opt/genesisit/current
sudo docker compose -p genesisit --env-file /opt/genesisit/shared/.env up -d --no-deps --build app
```

Откат данных выполняйте только по [MIGRATION.md](MIGRATION.md) из проверенного state-архива. Не пытайтесь вручную смешивать старую БД с новыми volumes.

## 16. Что нельзя делать

На production не выполняйте без отдельного проверенного плана:

```text
docker compose down -v
docker system prune --all --volumes
docker volume rm genesisit_postgres_data
```

Также нельзя:

- коммитить `.env`, `*.jks`, `*.keystore`, дампы и state-архивы;
- менять Compose project name `genesisit`, иначе Docker создаст другой набор volumes;
- генерировать новый `REMOTEIT_ACTION_SIGNING_SECRET` при обычном обновлении;
- генерировать новый Android signing key при переносе;
- удалять старый VDS до проверки нового и окончания окна отката;
- объявлять перенос успешным только потому, что открылась главная страница.

## 17. Финальный чек-лист

- [ ] Получен точный release-тег, `git status --short` пуст.
- [ ] `.env` и Android signing-файлы находятся только в `/opt/genesisit/shared`.
- [ ] На `.env`, `.jks` и signing env стоят права `0600` или строже.
- [ ] DNS указывает на новый VDS, TLS действителен.
- [ ] `db` и `app` healthy; остальные постоянные контейнеры запущены.
- [ ] `/healthz`, `AGENT-RELEASE.json` и `SHA256SUMS.txt` доступны по HTTPS.
- [ ] Внутренняя проверка `SHA256SUMS.txt` прошла полностью.
- [ ] Владелец вошёл и заменил временный пароль.
- [ ] Тестовый Agent получил или сохранил Remote ID и находится в сети.
- [ ] Проверены удалённый экран, терминал и передача файла.
- [ ] При использовании проверены RDP/SSH-туннель и Android-приложения.
- [ ] Создана и проверена резервная копия.
- [ ] Включены `genesisit-backup.timer` и `genesisit-docker-maintenance.timer`.
- [ ] Старый VDS сохранён для отката, если выполнялся перенос.

Дополнительные документы:

- [Первый production-запуск и эксплуатация](PRODUCTION_DEPLOYMENT_RU.md)
- [Перенос и восстановление](MIGRATION.md)
- [Состав переносимого релиза](RELEASE.md)
