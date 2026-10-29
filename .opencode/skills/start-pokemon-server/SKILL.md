---
name: start-pokemon-server
description: Use ONLY when the user asks to start/launch/raise/restart the pokemon-game server in D:\\pokemon-game ("поднять сервер", "запустить сервер", "перезапустить сервер", "start/raise/restart server"). Starts it fully detached so the agent's tool call returns immediately and the session never hangs. Do not load for tests, builds, or unrelated services.
---

# Запуск сервера pokemon-game в фоне

Сервер **никогда** не запускается через `go run ./server` из агентского
Bash-вызова: такой процесс держит сессию/пайп, и агент «зависает».
Правильный способ — собрать бинарник и стартовать его полностью
detached через WMI (`Win32_Process.Create`).

## Одна команда

```powershell
& "D:\pokemon-game\.opencode\skills\start-pokemon-server\start-server.ps1"
```

Скрипт сам:

1. проверяет порт 8080 (если занят — сообщает PID и выходит);
2. собирает `pokemon-server.exe` в `%TEMP%\opencode`;
3. пишет `run-pokemon.cmd` с `SESSION_KEY=local-test-key`;
4. запускает `.cmd` через `Invoke-CimMethod Win32_Process Create`
   (дочерний процесс отвязан, tool-call сразу возвращается);
5. ждёт появления порта максимум ~2 секунды и печатает `UP pid=...`.

Остановка и статус:

```powershell
& "...\start-server.ps1" -Stop
& "...\start-server.ps1" -Status
```

## Если нельзя использовать скрипт

Ручной вариант, тот же порядок:

```powershell
# 1) собрать (это единственный блокирующий шаг, обычно 2-5 секунд)
Push-Location D:\pokemon-game
go build -o "$env:TEMP\opencode\pokemon-server.exe" ./server
Pop-Location

# 2) отвязанный запуск, НЕ -Wait и НЕ go run
$cmd = "$env:TEMP\opencode\run-pokemon.cmd"
Invoke-CimMethod -ClassName Win32_Process -MethodName Create `
  -Arguments @{ CommandLine = "cmd.exe /c `"$cmd`"" }

# 3) короткая проверка (без длинных циклов ожидания)
Get-NetTCPConnection -LocalPort 8080 -State Listen
```

`run-pokemon.cmd`:

```bat
@echo off
set SESSION_KEY=local-test-key
cd /d D:\pokemon-game
"%TEMP%\opencode\pokemon-server.exe" > "%TEMP%\opencode\srv.out" 2> "%TEMP%\opencode\srv.err"
```

## Запрещено

- `go run ./server` в Bash-tool (даже через `Start-Process` с редиректом):
  процесс остаётся привязанным к сессии.
- Длинные `Start-Sleep` / циклы ожидания по 5-10 секунд: проверять порт
  максимум 1-2 секунды, иначе пользователь воспринимает это как зависание.
- Оставлять сервер после теста: по завершении — `-Stop`.

## Ответ пользователю

После успешного запуска — одна короткая строка:

`Сервер запущен фоном: http://localhost:8080 (PID <pid>).`

Если порт уже занят — сообщить PID и спросить, перезапускать ли.
Если запуск не удался — показать `%TEMP%\opencode\srv.err`.
