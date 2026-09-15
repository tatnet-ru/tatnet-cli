# tatnet — консольный клиент TatNet

`tatnet` управляет облаком TatNet из терминала: виртуальные машины,
приложения, базы Postgres и Valkey, DNS, объектное хранилище, SSH-ключи.

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
платформу (`optionalDependencies` с полями `os`/`cpu`), поэтому работает и
под `--ignore-scripts`, и из офлайн-кэша. Поддержаны linux, macOS и Windows
на x64 и arm64.

Бинарником с [релизов](https://github.com/tatnet-ru/tatnet-cli/releases)
(файлы несут номер версии, подставьте свою):

```bash
VER=0.1.3
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

Ключ создаётся в панели: **Аккаунт → API-ключи**. Ключ принадлежит одному
аккаунту и несёт свою политику прав, поэтому аккаунт нигде не указывается.

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

tatnet dns record create example.com www A 185.152.80.60
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
current: прод
profiles:
  прод:
    api_key: tn_live_…
    project: 7f3c9c1e-…
  стенд:
    api_key: tn_live_…
    base_url: https://api.stage.tatnet.ru/v1
```

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
