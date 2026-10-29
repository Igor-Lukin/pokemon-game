---
name: frontend-docks
description: Use when changing pokemon-game UI - game.js, game.html, game.css, pokedex.js, admin templates, docks, popups, context menus, battle cards. Covers vanilla JS conventions, event delegation, z-index, --pk-* design tokens and cache pitfalls.
---

# Фронтенд pokemon-game

Стек: серверные Go-шаблоны + ванильный JS/CSS. Сборщика нет, npm нет, фреймворка нет. После правок JS — `node --check`; браузеру нужен Ctrl+F5 (Go не шлёт no-cache на `/api`).

## Файлы

| Файл | Что внутри |
|---|---|
| `client/static/js/game.js` (~150 КБ) | вся игровая логика: локация, команда, инвентарь, питомник, покедекс-вызовы, бой, PvP, онлайн, диалоги, карточка тренера |
| `client/static/css/game.css` (~63 КБ) | стиль «Каталог/атлас», дизайн-токены `--pk-*` |
| `server/templates/game.html` | разметка панелей/доков/попапов |
| `client/static/js/pokedex.js` + `pokedex.css` | отдельный покедекс |
| `client/static/templates/battle_card.html` | шаблон боевой карточки, грузится fetch'ем |
| `server/templates/admin/*.html` | админка (обычные Go-шаблоны) |

## Соглашения

- **Не переписывать `game.js` целиком.** Искать нужную функцию и менять точечно; файл большой и связанный.
- **Делегирование событий обязательно** для элементов, которые перерисовываются (`loadTeam`, `render...`): вешать обработчик на контейнер/`document`, а не на каждый узел.
- **Данные в `data-*`**: `data-pok-id` (экземпляр), `data-species-id` (вид), `data-ability-name/desc`, `data-item-id`. Клик читает их из `closest(...)`.
- **Доки** — fixed-панели: `#team-dock`, `#trainer-dock`, `#nursery-panel`, `#dialogue-dock`, `#online-players`. Открытие/закрытие — парные функции, не плодить новые слои без нужды.
- **Попапы** — общий обработчик закрытия; новые попапы добавлять туда же и делать исключение для кликабельных элементов (как `.divAbility`). Образцы: `showAttackInfoPopup`, `showAbilityInfo`, `showToast`, `showBattleSwitchMenu`.
- **Уведомления** — только `showToast` (в проекте не было alert).
- **z-index**: панели/доки ниже, попапы выше, покедекс `fixed/right` может перекрывать док команды — учитывать при открытии из контекстного меню.
- **CSS**: использовать существующие `--pk-paper/--pk-ink/...`, не вводить новую палитру. Стили держать в `game.css`, а не инлайном (инлайн допустим для позиций спрайтов/прогресса).
- **Имена/подписи** — по-русски, как в остальном UI. Атаки называть русским названием.

## Ключевые точки, которые уже есть

- Локация: `loadLocation`, `getCurrentUserLocation`, `restartOnlinePolling`.
- Команда: `loadTeam`, `bindBoxLeftEvents`, контекстное меню, `teamCtxPokedex` (Покедекс вида из меню экземпляра).
- Покедекс: `openPokedex`, `selectPokemon`; максимум вида берётся динамически (`maxPokemonId` из `/api/pokedex/list`), не хардкодить 151.
- Бой: `startBattle`, `createBattleCard`, `loadBattleCardTemplate`, `updateBattleCardHp`, `renderTeamSelection`, `performMove`, `performSwitch`, `runBattle`.
- PvP: `startPvpBattle` → `enterPvpBattle`, `pvpRefresh`, поллинг `/api/pvp/state`, `pvpState` — глобальный флаг режима.
- NPC: `startTrainerBattle`, `openDialogue`, `renderDialogueNode`, `chooseOption`.
- Инвентарь: `showQuantitySelector` (поле + `+/-`, кламп на blur/Enter, не перезаписывать ввод на каждый символ).

## Грабли

- **Чат мёртв**: `game.js` создаёт `new WebSocket("ws://localhost:8080/chat")`, серверного обработчика `/chat` нет. Не строить на этом новые фичи; чинить отдельной задачей.
- **Кэш**: после правки JS/API браузер может держать старый ответ — Ctrl+F5; в консоли смотреть, что реально пришло.
- **Дубли спрайтов**: `normal/` и `mini/` содержат одинаковые файлы; клиент получает mini заменой `/normal/` → `/mini/`. Пути не ломать.
- **Кодировка**: исходники UTF-8; PowerShell `Get-Content` без `-Encoding UTF8` показывает кракозябры (файл при этом цел).
- **Не менять `battle_card.html` вслепую**: на него завязан `createBattleCard` через селекторы `.image img`, `.name`, `.lvl`, `.boxleft .typ`, `.status-badge`, `.items`.
- **Админка** использует те же CSS-переменные и простые формы; после правки шаблона проверить рендер списка и формы.

## Мини-чеклист UI-правки

1. Найти существующий паттерн (док/попап/контекст-меню) и повторить его.
2. `node --check` изменённых JS.
3. Открыть страницу, Ctrl+F5, проверить: открытие/закрытие, повторное открытие после перерисовки, клик вне, мобильная ширина (если актуально).
4. Проверить, что не сломались соседние доки и боевой экран.
