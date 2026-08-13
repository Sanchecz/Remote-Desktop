# Восстановление `v1.0.2` на другом сервере

Архивы private Release не имеют отдельного пароля. Для скачивания нужен доступ владельца к приватному репозиторию.

## 1. Скачать комплект

Открыть `Sanchecz/Remote-Desktop` → **Releases** → `v1.0.2` и скачать:

- `3x-ui-happ-v1.0.2-kit.zip`;
- нужный `portable-current-*.tar.gz`;
- `SHA256SUMS-release.txt`.

Не пересылать raw `.tar.gz` другим людям и не загружать в публичное облако.

## 2. Проверить SHA-256

Linux:

```bash
sha256sum -c SHA256SUMS-release.txt
```

Windows PowerShell:

```powershell
Get-FileHash .\portable-current-main-20260813T221137Z.tar.gz -Algorithm SHA256
```

Ожидаемые архивы:

```text
ba642ed81a5c135fae7e2388b38266425f92377980d444327498a41d5fbe4448  portable-current-main-20260813T221137Z.tar.gz
111657b4bc7ea9a7ac7f915c27b46804760f335f75ae767cb16bbfb22c79b09f  portable-current-node2-20260813T221230Z.tar.gz
```

## 3. Выполнить audit без восстановления

После распаковки ZIP:

```bash
bash v1.0.2/scripts/audit_portable_backup.sh \
  /path/to/portable-current-main-20260813T221137Z.tar.gz main

bash v1.0.2/scripts/audit_portable_backup.sh \
  /path/to/portable-current-node2-20260813T221230Z.tar.gz node2
```

Оба запуска должны завершиться `AUDIT_OK`.

## 4. Что обеспечивает сохранение старых ссылок Happ

Главный источник истины — `/etc/x-ui/x-ui.db`. Для продолжения работы старых URL также необходимо сохранить:

- прежний публичный host/IP, порт и path подписки;
- TLS-сертификат для того же host/IP;
- DNS-запись, если ссылка использует домен;
- UUID, `sub_id`, клиенты и назначения из базы;
- текущие настройки `subEncrypt` и `subUpdates`.

В `v1.0.2` routing доставляется не заголовком, а телом подписки. После восстановления должны быть:

```text
subEnableRouting=true
subRoutingRules=<пусто>
subIncyEnableRouting=true
subIncyRoutingRules=happ://routing/add/...
```

Нельзя переносить deeplink обратно в `subRoutingRules` без отдельного iOS-теста.

## 5. Подготовить новый сервер

1. Использовать совместимую `x86_64` ОС.
2. Снять snapshot чистого нового сервера у провайдера.
3. Не выключать старые серверы и не переключать DNS заранее.
4. Сверить `OS-RELEASE.txt`, `VERSIONS.txt`, `LISTENERS.txt`, маршруты и firewall.
5. Распаковать архив в закрытый staging:

   ```bash
   install -d -m 700 /root/restore-stage
   tar -xzf /path/to/portable-current-main-20260813T221137Z.tar.gz \
     -C /root/restore-stage
   ```

6. Определить единственный верхний каталог архива и работать только с его `snapshot/`.

## 6. Применить в согласованное окно

1. Снять backup целевого сервера.
2. Остановить только `x-ui` на новом сервере.
3. Восстановить `/etc/x-ui`, `/usr/local/x-ui`, unit-файл, сертификаты и служебные файлы из snapshot, сохраняя права.
4. Не копировать вслепую сетевые адреса/firewall; адаптировать их под новый интерфейс и публичный IP.
5. Выполнить `systemctl daemon-reload`.
6. Оставить `openai-chain-*-healthcheck.timer` выключенными.
7. Запустить x-ui.
8. Выполнить Xray `run -test` по сгенерированному config.
9. До переключения DNS проверить панель и подписку через временную запись `hosts`/`--resolve`.

## 7. Проверить до переключения трафика

- SQLite `integrity_check=ok`;
- x-ui/Xray active и ожидаемые PID;
- все подписки возвращают HTTP `200`;
- тело Base64 декодируется;
- в теле ровно один `happ://routing/add/...`;
- заголовок `Routing` отсутствует;
- `Routing-Enable: true` и интервал `1`;
- deeplink SHA совпадает с `46f76e...4976b`;
- Telegram подключается, грузит сообщения, истории и файлы;
- `.ru`/`.рф` видят прямой адрес устройства;
- иностранный тестовый сайт видит VPN;
- OpenAI на втором узле выходит через основной;
- Happ iOS обновляет подписку без вылета.

Только после этого переключать DNS/адрес и сохранять старый сервер для быстрого отката.

## 8. Если публичный IP изменился

Если URL подписки содержит старый IP, автоматически сохранить ту же ссылку на новом IP невозможно без сохранения старого адреса или reverse proxy/перенаправления на старом адресе. Если URL использует домен, можно переключить DNS после теста, сохранив path, port и сертификат.

Служебный WireGuard-канал между узлами не привязан жёстко к исходному IP клиента, но endpoint стороны-сервера и healthcheck expected-state нужно проверить после изменения адресов.
