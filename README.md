# tatnet — консольный клиент TatNet

[tatnet.ru](https://tatnet.ru) · [панель](https://min.tatnet.ru) ·
[документация](https://docs.tatnet.ru) ·
[справочник API](https://api.tatnet.ru/v1/docs)

`tatnet` управляет облаком [TatNet](https://tatnet.ru) из терминала:
виртуальные машины, приложения, базы Postgres и Valkey, DNS, объектное
хранилище, SSH-ключи.

Построен поверх [tatnet-go](https://github.com/tatnet-ru/tatnet-go) —
клиента, генерируемого из контракта `/v1`. Рукописного HTTP здесь нет, и
контракт — тот же самый документ, что публикует API.

## Установка

Без установки вообще — если есть Node:

```bash
npx tatnet vm list
```

Постоянно:

```bash
npm i -g tatnet
```

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
VER=0.1.7
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
tatnet profile set-project прод    # проект по умолчанию
```

Ключ проверяется до записи в конфиг: сохранённый нерабочий ключ выглядел бы
как настроенный CLI и падал бы на первой же команде.

## Примеры

```bash
tatnet vm list
tatnet vm get web-1                       # по имени или hostname, не только по id
tatnet vm stop web-1
tatnet vm backup create web-1 --name до-обновления

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

Своими командами выведены 91 операция из 161. Остальные — балансировщики,
сети, Kubernetes, функции, тома, домены, сертификаты — доступны прямым
вызовом:

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
