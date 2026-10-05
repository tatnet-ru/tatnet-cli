# tatnet — консольный клиент TatNet

[tatnet.ru](https://tatnet.ru) · [панель](https://min.tatnet.ru) ·
[документация](https://docs.tatnet.ru) ·
[справочник API](https://api.tatnet.ru/v1/docs)

`tatnet` управляет облаком [TatNet](https://tatnet.ru) из терминала:
виртуальные машины, приложения, базы Postgres и Valkey, DNS, объектное
хранилище, SSH-ключи, VPC, NAT и плавающие IP.

Построен поверх [tatnet-go](https://github.com/tatnet-ru/tatnet-go) —
клиента, генерируемого из контракта `/v1`. Рукописного HTTP здесь нет, и
контракт — тот же самый документ, что публикует API.

## Quick start / English

TatNet CLI deploys static and SSR websites and manages apps, virtual machines,
Postgres and Valkey databases, DNS, S3 and networking on TatNet, a Russian cloud.
Install with `npm install -g tatnet@latest`, create a scoped API key in the
[TatNet dashboard](https://min.tatnet.ru/api-keys), then run `tatnet auth login`.
For a first upload, choose a project with `tatnet profile set-project <project>`
and run `tatnet deploy --logs` from your website directory. The command waits
for `live`, printing the URL to stdout and logs to stderr.

- [CLI overview](https://tatnet.ru/cli) and [installation docs](https://docs.tatnet.ru/docs/cli/intro).
- [Deploy from CI](https://docs.tatnet.ru/docs/guides/en/cli-ci-deploy).
- [Diagnose a build](https://docs.tatnet.ru/docs/guides/en/debug-build).
- [Publish from Claude Code using MCP](https://docs.tatnet.ru/docs/guides/en/claude-code-deploy).

## Установка

Без установки вообще — если есть Node:

```bash
npx tatnet vm list
```

Постоянно:

```bash
npm i -g tatnet
```

Какая версия стоит — `tatnet --version` (печатает версию, коммит и дату
сборки). Обновить: `npm i -g tatnet@latest`; новая версия появляется в npm
через несколько минут после выпуска в GitHub Releases.

Бинарник качается не при установке, а приезжает готовым подпакетом на вашу
платформу (`@tatnet/cli-<os>-<arch>` в `optionalDependencies` с полями
`os`/`cpu`), поэтому работает и под `--ignore-scripts`, и из офлайн-кэша.
Поддержаны linux, macOS и Windows на x64 и arm64.

### Что именно вы ставите

В пакете лежит скомпилированный бинарник — как у `esbuild`, `swc`, `biome`
и `turbo`. Проверить, что в нём, можно не на слово:

```bash
npm audit signatures            # подпись и происхождение пакета
npm view tatnet dist.integrity  # хеш тарболла в реестре
```

Пакеты публикуются с **provenance-аттестацией**: npm подписывает связь
«этот тарболл собран вот этим прогоном GitHub Actions из вот этого коммита
публичного репозитория», и на странице пакета видно, каким именно. Сам
бинарник при этом не собирается на месте публикации — он берётся из
[релиза](https://github.com/tatnet-ru/tatnet-cli/releases) со сверкой
`checksums.txt`, а релиз собирает goreleaser в открытом прогоне.

Цепочка целиком проверяема снаружи: коммит → прогон сборки → артефакт
релиза с контрольной суммой → тот же файл в пакете npm. Ничего не
скачивается при установке и ничего не выполняется в `postinstall` — его
здесь нет вовсе.

Бинарником с [релизов](https://github.com/tatnet-ru/tatnet-cli/releases)
(файлы несут номер версии, подставьте свою):

```bash
VER=0.1.8
curl -sSL "https://github.com/tatnet-ru/tatnet-cli/releases/download/v$VER/tatnet_${VER}_linux_amd64.tar.gz" | tar xz
sudo install tatnet /usr/local/bin/
```

Либо пакетом — `tatnet_${VER}_linux_amd64.deb` / `.rpm`. Рядом с
артефактами лежит `checksums.txt`.

Из исходников, если стоит Go:

```bash
go install github.com/tatnet-ru/tatnet-cli/cmd/tatnet@latest
```

## Начало работы

Ключ создаётся в панели: **[min.tatnet.ru/api-keys](https://min.tatnet.ru/api-keys)**.
Ключ принадлежит одному аккаунту и несёт свою политику прав, поэтому аккаунт
нигде не указывается.

```bash
tatnet auth login                  # спросит ключ, проверит его и сохранит
tatnet auth status                 # чей ключ и что он может
tatnet project list
tatnet profile set-project прод    # проект по умолчанию — нужен только для создания
```

Командам над существующим ресурсом проект не нужен — приложениям, ВМ,
кластерам Postgres и Valkey: `tatnet vm stop web-1` или `tatnet app get магазин`
найдут ресурс по имени во всём аккаунте и сами узнают его проект. Заданный
проект лишь сужает поиск. Проект нужен только при создании — `app create`,
`vm create`, `pg create`, `valkey create` и `deploy` (первая выкладка
создаёт приложение): ресурс создаётся в нём.

Ключ проверяется до записи в конфиг: сохранённый нерабочий ключ выглядел бы
как настроенный CLI и падал бы на первой же команде.

## Примеры

```bash
tatnet vm list
tatnet vm get web-1                       # по имени или hostname, не только по id
tatnet vm stop web-1
tatnet vm backup create web-1 --name до-обновления

tatnet app list --all-projects            # все доступные проекты аккаунта
tatnet app get магазин                    # проект не нужен — узнаётся из приложения
tatnet app deploy магазин
tatnet app env set магазин DATABASE_URL 'postgres://…' --secret
tatnet app job run магазин миграции
tatnet app run list магазин --job миграции

tatnet pg list
tatnet pg parameters get основная         # заданное рядом с применённым
tatnet pg ca основная --out ca.crt
tatnet pg backup list основная

tatnet dns record create example.com www A 203.0.113.10
tatnet s3 bucket create фото --public
tatnet s3 object presign фото отчёт.pdf
```

## Деплой из папки

Выложить то, что лежит на диске, без репозитория:

```bash
cd ~/проекты/лендинг
tatnet deploy
```

Первый запуск создаёт приложение по имени папки и запоминает его в
`.tatnet/project.json` — следующие выкладывают туда же. В stdout уходит
**только адрес**, поэтому так тоже можно:

```bash
url=$(tatnet deploy --logs)   # лог сборки идёт в stderr, адрес — в stdout
```

**Что уезжает.** Папка целиком, кроме: `.git`, `node_modules`, `.tatnet` и
файлов окружения (`.env`, `.env.*`, но не `.env.example`). Остальное
исключается правилами `.tatnetignore`, а если его нет — `.gitignore`.
Пропущенные файлы окружения перечисляются в выводе: переменные задаются
`tatnet app env`, а не случайно вместе с папкой.

**Чего у такой выкладки нет.** Коммита: ни автора, ни диффа, ни возможности
пересобрать её из системы контроля версий. Вместо него у сборки есть
отпечаток архива (`tatnet app build list`) — по нему видно, что две выкладки
одинаковы или различны, но история им не заменяется. Для командного прода
подключайте репозиторий: тогда каждый деплой отвечает на вопрос «из чего он
собран».

| Флаг | Зачем |
| --- | --- |
| `--logs` | печатать лог сборки |
| `--no-wait` | не ждать окончания сборки |
| `--app <имя\|id>` | выложить в конкретное приложение |
| `--name <имя>` | имя создаваемого приложения |
| `--no-create` | не создавать приложение, если его нет |
| `--max-size <МиБ>` | предел размера исходников (по умолчанию 100) |

## Вывод

| `-o` | что делает |
|---|---|
| `table` | по умолчанию, только ключевые колонки |
| `wide` | плюс идентификаторы и подробности |
| `json` | ровно то, что вернул API — для скриптов |
| `yaml` | то же в YAML |

Списки вычитываются **целиком**: поле `count` в ответах API — размер
страницы, а не размер набора, поэтому конец определяется короткой
страницей. `--limit` ограничивает явно.

## Профили

Конфиг лежит в `~/.config/tatnet/config.yaml` (или `$TATNET_CONFIG`),
пишется режимом 0600 — в нём ключ.

```yaml
current: рабочий
profiles:
  рабочий:
    api_key: tn_live_…
    project: 7f3c9c1e-…
  второй-аккаунт:
    api_key: tn_live_…
```

`base_url` в профиле нужен, только если вы ходите не в боевой API —
например, в поднятый локально (`http://localhost:8000/v1`). По умолчанию
адрес берётся из клиента и указывать его не нужно.

```bash
tatnet profile list
tatnet profile use стенд
tatnet --profile стенд vm list
```

Переменные окружения перебивают профиль, флаги — переменные:
`TATNET_API_KEY`, `TATNET_BASE_URL`, `TATNET_PROJECT`, `TATNET_PROFILE`,
`TATNET_OUTPUT`, `TATNET_CONFIG`.

## `tatnet api` — всё остальное

Для разделов без отдельных команд — балансировщиков, Kubernetes,
функций, томов, доменов и сертификатов — доступен прямой вызов API:

```bash
tatnet api --list load-balancers          # что вообще есть
tatnet api /vpcs
tatnet api /floating-ips -X POST -f name=fip-1 -f cluster_id=…
tatnet api /projects/<id>/kubernetes-clusters/<id>/kubeconfig
```

Адрес проверяется по вшитому контракту **до** отправки: иначе опечатка
вернула бы 404, неотличимый от «такого ресурса нет». Если API новее этого
CLI — `--allow-unknown`.

## Разрушающие действия

Удаление, сброс пароля и замена узлов спрашивают подтверждение. Когда ввод
не терминал (скрипт, CI), вопрос задать некому — такие команды **требуют
`--yes`**, а не соглашаются молча.

## Что здесь проверяется тестами

Контракт связан с деревом команд гейтом (`internal/cli/coverage_test.go`):

* каждая операция, объявленная командой, обязана существовать в контракте —
  переименовали операцию в API, CLI падает на сборке, а не у клиента;
* покрытый раздел покрыт **целиком** — новая операция в нём валит тест, а не
  остаётся тихо недоступной;
* отложенный раздел не покрыт **частично** — наполовину выведенный раздел
  это список, которому уже нельзя верить;
* вшитый контракт совпадает с контрактом той версии `tatnet-go`, на которой
  собран CLI.

Обновление контракта: `./scripts/sync-contract.sh`, затем `go test ./...` —
гейт покажет, что появилось нового.

## Разработка

```bash
go build ./...
go test ./...
go run ./cmd/tatnet --help
```

## Ссылки

| | |
|---|---|
| Облако TatNet | [tatnet.ru](https://tatnet.ru) |
| Панель управления | [min.tatnet.ru](https://min.tatnet.ru) |
| Ключи доступа | [min.tatnet.ru/api-keys](https://min.tatnet.ru/api-keys) |
| Документация платформы | [docs.tatnet.ru](https://docs.tatnet.ru) |
| Справочник `/v1` | [api.tatnet.ru/v1/docs](https://api.tatnet.ru/v1/docs) |
| Контракт OpenAPI | [api.tatnet.ru/v1/openapi.json](https://api.tatnet.ru/v1/openapi.json) |
| Go-клиент того же API | [tatnet-ru/tatnet-go](https://github.com/tatnet-ru/tatnet-go) |
| Пакет в npm | [npmjs.com/package/tatnet](https://www.npmjs.com/package/tatnet) |

## Лицензия

[Apache-2.0](LICENSE).

## Поиск приложений и проверка выката

`app list` использует проект из `--project`, `$TATNET_PROJECT` или профиля.
В табличном выводе CLI показывает этот охват и его источник в stderr.
`--all-projects` игнорирует проект профиля и окружения и перечисляет все
проекты, доступные ключу; одновременно задавать `--project` нельзя.
Фильтры сопоставляют точные значения, `--limit` ограничивает число совпадений.

```bash
tatnet app list --all-projects --repo manzhikov/tatnet-frontend --branch main
tatnet app list --all-projects --domain min.tatnet.ru
tatnet app deploy web --commit <полный-SHA> --wait --logs
tatnet app wait web --commit <полный-SHA> --deadline 30m
tatnet app wait web --build <build-id>
```

Ожидание закрепляет конкретную сборку и подтверждает `status=success` вместе
с `deploy_state=live`. Сборка другого коммита не удовлетворяет ожидание.
Ошибки сборки/запуска и истечение срока дают ненулевой код выхода; при
ошибке CLI пытается прочитать лог. `--logs` включает ожидание в `app deploy`.
`tatnet deploy` из папки также ждёт `live`; `--no-wait` только ставит сборку
в очередь. Строки логов и сведения об охвате идут в stderr, JSON остаётся
пригодным для обработки скриптами.

## Сети аккаунта

```bash
tatnet vpc list --cluster <region-id>
tatnet vpc create --name private --cluster <region-id> --subnet 10.20.0.0/24
tatnet vpc get private
tatnet vpc nat enable private                 # выделяет новый публичный IP
tatnet vpc nat enable private --floating-ip <fip-id>
tatnet vpc reserved-ip create private --name fixed --address 10.20.0.5 --mac 02:00:00:00:00:01
tatnet vpc reserved-ip delete private <reservation-id> --yes
tatnet floating-ip create --cluster <region-id> --name public
tatnet floating-ip attach public --interface <vm-interface-id>
tatnet floating-ip detach public --yes
tatnet floating-ip delete public --yes
tatnet vpc nat disable private --yes
tatnet vpc delete private --yes
```

Сеть выбирается по UUID или точному имени; при одинаковых именах нужно
указать UUID. Плавающий IP можно выбрать по UUID, имени или адресу.
Эти ресурсы принадлежат аккаунту и не требуют `--project`.

## Инференс

`tatnet ai` обращается к общему API `https://ai.tatnet.cloud/v1`.
Ключ инференса `tnai_live_…` отличается от ключа управления облаком:
задайте `TATNET_INFERENCE_API_KEY` или `--inference-key`.
Вывод этих команд — JSON ответа шлюза; тарифы и проверки не копируются в CLI.

```sh
tatnet ai models --kind image
tatnet ai quote --kind image --body @image-request.json
tatnet ai generate --kind image --body @image-request.json
tatnet ai job img_... --kind image
tatnet ai models --kind video
tatnet ai quote --kind video --body @video-request.json
tatnet ai generate --kind video --body @video-request.json
tatnet ai job vid_... --kind video
tatnet ai generate --kind chat --body @chat-request.json
```

`generate` запускает оплачиваемую операцию. Сначала `quote` позволяет
проверить цену изображения/видео. Создание задачи не повторяется при
редиректе или ошибке сети. `job` возвращает состояние и готовые ссылки.
