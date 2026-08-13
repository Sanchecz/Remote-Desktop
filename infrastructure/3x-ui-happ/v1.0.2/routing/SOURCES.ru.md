# Источники и ограничения routing `v1.0.2`

## Формат Happ

Канонические имена и строковые типы сверены с:

- документацией Happ: <https://www.happ.su/main/ru/dev-docs/routing>
- официальным генератором: <https://github.com/Happ-proxy/routing_generator>
- управлением приложением: <https://www.happ.su/main/ru/dev-docs/app-management>

Профиль не загружает внешние GEO-файлы и не требует нестандартных секций.

## Происхождение явных правил

Для прежнего раскрытия категорий использовался закреплённый снимок `hydraponique/roscomvpn-geosite`:

- reference: `202604152235`;
- commit: `24f73e9f89ffbe665c67082c9510614fd1da8547`;
- `geosite.dat` SHA-256: `765b86e4b6aed5da1a206304b5500c7668687fa1df8e8322c8a4961e1b672190`;
- `data/torrent` — источник tracker/DHT-доменов;
- `data/twitch-ads` SHA-256: `cb78b9d9a6373719911f368b7d6c629379a9a9993e245dc08f0e729bdc492104e`;
- `data/whitelist` SHA-256: `c369ee5b0c1991eeac520135cbdeb8bb34655cc37f53bc1e6bc1d947da97b550b`.

Для анализа `geoip:direct` использовался `hydraponique/roscomvpn-geoip`:

- tag: `202604160537`;
- commit: `f847027406d522dc2bbcd77d207bdcd1dbf02e2f`;
- `geoip.dat` SHA-256: `cdeb5a1d038c75dd42add7c3b6205866f53a0bb601bb14f07dafffa9e870a056`.

Большой набор внешних CIDR не встроен в подписку. Private/loopback/link-local сети представлены 11 явными CIDR, российские домены — явными доменами и regex.

## Ограничение torrent-блокировки

431 правило блокирует известные домены trackers и DHT routers. Это не полная DPI-блокировка зашифрованного BitTorrent по неизвестным IP. Server-side sniffing не включается, потому что уже вызывал регрессию Telegram в Happ. Xray прямо указывает, что протокольное правило `bittorrent` требует sniffing, а само распознавание ограничено для шифрованного/обфусцированного трафика.

Справочник Xray routing: <https://xtls.github.io/en/config/routing.html>
