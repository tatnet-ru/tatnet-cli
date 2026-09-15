#!/usr/bin/env bash
# Обновляет вшитый контракт из версии tatnet-go, на которой собран CLI.
#
# Источник правды — контракт, опубликованный в клиенте: CLI ходит его
# методами, и проверять адреса по другому документу значит проверять не то.
# Тест TestEmbeddedContractMatchesClientModule следит за расхождением.
set -euo pipefail

cd "$(dirname "$0")/.."

dir="$(go list -m -f '{{.Dir}}' github.com/tatnet-ru/tatnet-go)"
if [[ -z "$dir" ]]; then
  echo "не найден модуль github.com/tatnet-ru/tatnet-go" >&2
  exit 1
fi

version="$(go list -m -f '{{.Version}}' github.com/tatnet-ru/tatnet-go)"
cp "$dir/openapi/v1.json" internal/contract/v1.json
chmod u+w internal/contract/v1.json

ops=$(python3 -c "
import json,sys
d=json.load(open('internal/contract/v1.json'))
print(sum(1 for p in d['paths'].values() for m in p if m in ('get','post','put','patch','delete')))
")
echo "контракт обновлён из tatnet-go $version: операций $ops"
echo "дальше: go test ./... — гейт покрытия скажет, что появилось нового"
