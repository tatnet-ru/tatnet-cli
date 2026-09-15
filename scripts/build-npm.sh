#!/usr/bin/env bash
# Собирает пакеты npm из артефактов уже выпущенного релиза GitHub.
#
# Источник — опубликованные бинарники, а не локальная сборка: в npm должно
# уехать ровно то, что лежит в релизе и сверяется checksums.txt. Сборка «тут
# же, из исходников» дала бы другой бинарник с тем же номером версии.
#
#   ./scripts/build-npm.sh 0.1.2        # готовит npm/dist
#   ./scripts/build-npm.sh 0.1.2 --publish
set -euo pipefail

cd "$(dirname "$0")/.."

VERSION="${1:-}"
PUBLISH="${2:-}"
if [[ -z "$VERSION" ]]; then
  echo "укажите версию без v, например: $0 0.1.2" >&2
  exit 1
fi
if [[ "$VERSION" == v* ]]; then
  echo "версия указывается без префикса v: ${VERSION#v}" >&2
  exit 1
fi

REPO="tatnet-ru/tatnet-cli"
DIST="npm/dist"
WORK="$DIST/.download"

rm -rf "$DIST"
mkdir -p "$WORK"

# Пары «goos/goarch → ключ npm»: process.platform + '-' + process.arch.
PLATFORMS=(
  "darwin:amd64:darwin:x64:tar.gz"
  "darwin:arm64:darwin:arm64:tar.gz"
  "linux:amd64:linux:x64:tar.gz"
  "linux:arm64:linux:arm64:tar.gz"
  "windows:amd64:win32:x64:zip"
  "windows:arm64:win32:arm64:zip"
)

echo "качаю артефакты релиза v$VERSION"
for spec in "${PLATFORMS[@]}"; do
  IFS=: read -r goos goarch nodeos nodearch ext <<<"$spec"
  asset="tatnet_${VERSION}_${goos}_${goarch}.${ext}"
  gh release download "v$VERSION" --repo "$REPO" --pattern "$asset" --dir "$WORK" --clobber
done

echo "сверяю контрольные суммы релиза"
gh release download "v$VERSION" --repo "$REPO" --pattern checksums.txt --dir "$WORK" --clobber
( cd "$WORK" && grep -E '\.(tar\.gz|zip)$' checksums.txt | while read -r sum name; do
    [[ -f "$name" ]] || continue
    actual=$(shasum -a 256 "$name" | awk '{print $1}')
    if [[ "$actual" != "$sum" ]]; then
      echo "сумма не сошлась у $name" >&2
      exit 1
    fi
  done )

for spec in "${PLATFORMS[@]}"; do
  IFS=: read -r goos goarch nodeos nodearch ext <<<"$spec"
  pkg="@tatnet/cli-${nodeos}-${nodearch}"
  # Каталог плоский: скоуп в пути создал бы лишний уровень вложенности.
  dir="$DIST/plat-${nodeos}-${nodearch}"
  mkdir -p "$dir/bin"

  asset="$WORK/tatnet_${VERSION}_${goos}_${goarch}.${ext}"
  if [[ "$ext" == "zip" ]]; then
    unzip -qo "$asset" -d "$WORK/x-$pkg"
  else
    mkdir -p "$WORK/x-$pkg" && tar xzf "$asset" -C "$WORK/x-$pkg"
  fi

  exe="tatnet"
  [[ "$nodeos" == "win32" ]] && exe="tatnet.exe"
  cp "$WORK/x-$pkg/$exe" "$dir/bin/$exe"
  chmod 0755 "$dir/bin/$exe"

  python3 - "$dir/package.json" "$pkg" "$VERSION" "$nodeos" "$nodearch" <<'PY'
import json, sys
path, name, version, nodeos, nodearch = sys.argv[1:6]
json.dump({
    "name": name,
    "version": version,
    "description": f"Бинарник tatnet для {nodeos}-{nodearch}",
    "repository": {"type": "git", "url": "git+https://github.com/tatnet-ru/tatnet-cli.git"},
    "os": [nodeos],
    "cpu": [nodearch],
    "files": ["bin"],
    # Yarn PnP иначе держал бы бинарник внутри zip, откуда его не запустить.
    "preferUnplugged": True,
}, open(path, "w"), ensure_ascii=False, indent=2)
open(path, "a").write("\n")
PY
  echo "  $pkg $(du -h "$dir/bin/$exe" | awk '{print $1}')"
done

# Главный пакет: shim + зависимости, прибитые к ТОЧНОЙ версии. Диапазон здесь
# означал бы shim одной версии поверх бинарника другой.
mkdir -p "$DIST/tatnet/bin"
cp npm/tatnet/bin/tatnet.js "$DIST/tatnet/bin/tatnet.js"
cp README.md "$DIST/tatnet/README.md"
python3 - "$DIST/tatnet/package.json" "$VERSION" <<'PY'
import json, sys
path, version = sys.argv[1:3]
manifest = json.load(open("npm/tatnet/package.json"))
manifest["version"] = version
manifest["optionalDependencies"] = {k: version for k in manifest["optionalDependencies"]}
json.dump(manifest, open(path, "w"), ensure_ascii=False, indent=2)
open(path, "a").write("\n")
PY
echo "  tatnet (shim)"

rm -rf "$WORK"
echo "готово: $DIST"

if [[ "$PUBLISH" == "--publish" ]]; then
  # Платформенные пакеты уходят ПЕРВЫМИ: главный на них ссылается, и обратный
  # порядок оставил бы в реестре tatnet, который нечем удовлетворить.
  for spec in "${PLATFORMS[@]}"; do
    IFS=: read -r _ _ nodeos nodearch _ <<<"$spec"
    ( cd "$DIST/plat-${nodeos}-${nodearch}" && npm publish --access public )
  done
  ( cd "$DIST/tatnet" && npm publish --access public )
  echo "опубликовано: tatnet@$VERSION"
fi
