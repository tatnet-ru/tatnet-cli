#!/usr/bin/env node
"use strict";

// Тонкая обёртка: находит бинарник своей платформы и отдаёт ему управление.
//
// Сам бинарник лежит в отдельном пакете на платформу, подключённом через
// optionalDependencies с полями os/cpu — npm ставит ровно один подходящий.
// Скачивания в postinstall здесь намеренно нет: он не работает при
// --ignore-scripts, требует сети в момент установки и ломает офлайн-кэш,
// а npx выполняется как раз в тех средах, где всё это встречается.

const { spawnSync } = require("node:child_process");
const fs = require("node:fs");
const path = require("node:path");

// Платформенные пакеты — в скоупе @tatnet: незанятые имена вида
// tatnet-cli-win32-arm64 реестр отвергает эвристикой антиспама, а имена
// внутри собственной организации под неё не попадают. Главный пакет
// остаётся неймспейсным (`npx tatnet`) — его имя коротко и проходит.
const PACKAGES = {
  "darwin-arm64": "@tatnet/cli-darwin-arm64",
  "darwin-x64": "@tatnet/cli-darwin-x64",
  "linux-arm64": "@tatnet/cli-linux-arm64",
  "linux-x64": "@tatnet/cli-linux-x64",
  "win32-arm64": "@tatnet/cli-win32-arm64",
  "win32-x64": "@tatnet/cli-win32-x64",
};

const RELEASES = "https://github.com/tatnet-ru/tatnet-cli/releases";

function fail(message) {
  process.stderr.write("tatnet: " + message + "\n");
  process.exit(1);
}

function resolveBinary() {
  const key = process.platform + "-" + process.arch;
  const pkg = PACKAGES[key];
  if (!pkg) {
    fail(
      "нет сборки для " + key + ".\n" +
        "Поддерживаются: " + Object.keys(PACKAGES).join(", ") + ".\n" +
        "Соберите из исходников: go install github.com/tatnet-ru/tatnet-cli/cmd/tatnet@latest"
    );
  }

  const exe = process.platform === "win32" ? "tatnet.exe" : "tatnet";
  let manifest;
  try {
    // Через package.json, а не напрямую по файлу: так путь находится
    // одинаково и в обычном node_modules, и в pnpm со ссылками.
    manifest = require.resolve(pkg + "/package.json");
  } catch {
    fail(
      "пакет " + pkg + " не установлен.\n" +
        "Так бывает при установке с --no-optional или --ignore-optional.\n" +
        "Поставьте его явно: npm i " + pkg + "\n" +
        "Либо возьмите бинарник: " + RELEASES
    );
  }

  const binary = path.join(path.dirname(manifest), "bin", exe);
  if (!fs.existsSync(binary)) {
    fail("пакет " + pkg + " установлен, но бинарника в нём нет: " + binary);
  }
  return binary;
}

function ensureExecutable(binary) {
  if (process.platform === "win32") return;
  try {
    fs.accessSync(binary, fs.constants.X_OK);
  } catch {
    // Некоторые установщики теряют бит запуска при распаковке.
    try {
      fs.chmodSync(binary, 0o755);
    } catch (err) {
      fail("бинарник не исполняемый и права поправить не удалось: " + err.message);
    }
  }
}

const binary = resolveBinary();
ensureExecutable(binary);

const result = spawnSync(binary, process.argv.slice(2), { stdio: "inherit" });

if (result.error) {
  fail("не удалось запустить " + binary + ": " + result.error.message);
}
// Код возврата обязан дойти до вызывающего: на CLI строят скрипты, и
// потерянный ненулевой код превратил бы отказ в успех.
if (result.signal) {
  process.exit(1);
}
process.exit(result.status === null ? 1 : result.status);
