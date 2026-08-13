# Восстановление `v1.0.1` без пароля

Пароль и расшифрование не нужны. Достаточно доступа владельца к приватному GitHub-репозиторию.

## 1. Скачать private Release

Открыть `Sanchecz/Remote-Desktop` → **Releases** → `v1.0.1` и скачать:

- `3x-ui-happ-v1.0.1-kit.zip`;
- нужный `portable-current-*.tar.gz`;
- `SHA256SUMS-release.txt`.

Не пересылать `.tar.gz` другим людям и не загружать его в публичное облако.

## 2. Проверить SHA-256

Linux:

```bash
sha256sum -c SHA256SUMS-release.txt
```

Если скачаны не все assets, можно проверить конкретный файл:

```bash
sha256sum portable-current-main-20260813T124133Z.tar.gz
```

Windows PowerShell:

```powershell
Get-FileHash .\portable-current-main-20260813T124133Z.tar.gz -Algorithm SHA256
```

## 3. Провести аудит без восстановления

Распаковать ZIP, затем:

```bash
bash v1.0.1/scripts/audit_portable_backup.sh \
  /path/to/portable-current-main-20260813T124133Z.tar.gz main

bash v1.0.1/scripts/audit_portable_backup.sh \
  /path/to/portable-current-node2-20260813T124235Z.tar.gz node2
```

Результат каждого запуска должен заканчиваться `AUDIT_OK`.

## 4. Что сохраняет старые ссылки Happ

Для продолжения работы старых URL нужно сохранить:

- `/etc/x-ui/x-ui.db`;
- прежнее доменное имя, внешний порт и путь подписки;
- TLS-сертификат или выпустить новый для того же домена;
- DNS-запись и переключить её только после локальной проверки.

Если в URL жёстко записан IP, смена адреса требует сохранения старого IP или переходного reverse proxy.

## 5. Подготовить новый сервер

1. Использовать `x86_64` и совместимую ОС.
2. Создать отдельный снимок чистого сервера.
3. Сверить `OS-RELEASE.txt`, `VERSIONS.txt`, `LISTENERS.txt`, маршруты и firewall.
4. Не менять старые серверы и не переключать DNS заранее.
5. Распаковать архив в root-only staging:

   ```bash
   install -d -m 700 /root/restore-stage
   tar -xzf /path/to/portable-current-main-20260813T124133Z.tar.gz \
     -C /root/restore-stage
   ```

## 6. Применить в окно работ

1. Снять новый бэкап целевого сервера.
2. Остановить только `x-ui` на новом сервере.
3. Скопировать проверенные `/etc/x-ui`, `/usr/local/x-ui`, service unit и необходимые сертификаты из `snapshot/`, сохраняя владельцев и права.
4. Не копировать вслепую firewall и сетевые настройки.
5. Выполнить `systemctl daemon-reload`.
6. Оставить `openai-chain-*-healthcheck.timer` выключенными.
7. Запустить `x-ui`.
8. Выполнить `xray-linux-amd64 run -test -c bin/config.json`.
9. До DNS проверить панель и подписки через временную запись `hosts`.
10. Проверить по одному профилю каждого протокола.
11. Проверить три обновления Happ и два повторных подключения Telegram.
12. Только затем переключить DNS.

Не применять `rsync --delete` и не копировать весь корень без аудита.

## 7. Смена IP двух узлов

При смене IP основного узла обновить endpoint служебного WireGuard outbound на втором узле и проверить firewall/ACL. При смене IP второго узла проверить ограничения на основном узле; WireGuard peer обычно определяется ключом.

После смены любого IP проверить DNS, TLS, firewall, OpenAI trace, обычный trace второго узла, `.ru`/`.рф`, Telegram и повторные обновления Happ.

## 8. Откат

До переключения DNS не переключать его. После переключения вернуть DNS на старый сервер и дождаться TTL. Старые серверы и снимки не удалять до полного smoke-test.
