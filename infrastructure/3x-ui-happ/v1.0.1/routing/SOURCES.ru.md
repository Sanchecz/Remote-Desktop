# Источники явных routing-правил

Стабильный профиль `v1.0.0` не загружает внешние GEO-файлы и не ссылается на секции `geosite:*`/`geoip:*`. В файле сохранены только обычные доменные правила, регулярные выражения и явные CIDR.

Для воспроизводимого преобразования прежних категорий использован закреплённый снимок `hydraponique/roscomvpn-geosite`:

- reference: `202604152235`;
- commit: `24f73e9f89ffbe665c67082c9510614fd1da8547`;
- `geosite.dat` SHA-256: `765b86e4b6aed5da1a206304b5500c7668687fa1df8e8322c8a4961e1b672190`;
- `data/torrent`: источник явных правил трекеров;
- `data/twitch-ads` SHA-256: `cb78b9d9a6373719911f368b7d6c629379a9a9993e245dc08f0e729bdc492104e`;
- `data/whitelist` SHA-256: `c369ee5b0c1991eeac520135cbdeb8bb34655cc37f53bc1e6bc1d947da97b550b`.

Для анализа прежнего `geoip:direct` использован закреплённый снимок `hydraponique/roscomvpn-geoip`:

- tag: `202604160537`;
- commit: `f847027406d522dc2bbcd77d207bdcd1dbf02e2f`;
- `geoip.dat` SHA-256: `cdeb5a1d038c75dd42add7c3b6205866f53a0bb601bb14f07dafffa9e870a056`;
- `release/text/direct.txt`: 34 855 CIDR.

Эти 34 855 CIDR не встроены в профиль: они чрезмерно увеличили бы subscription header. Вместо этого локальные, loopback, link-local и служебные сети представлены 11 явными CIDR, а российские домены — доменными и regex-правилами.

Справочные материалы:

- Happ routing: <https://www.happ.su/main/ru/dev-docs/routing>
- Happ app management: <https://www.happ.su/main/ru/dev-docs/app-management>
- Xray routing: <https://xtls.github.io/en/config/routing.html>
- закреплённый geosite-источник: <https://github.com/hydraponique/roscomvpn-geosite/tree/202604152235>
