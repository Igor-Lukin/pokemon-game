# AI_MEMORY — институциональная память проекта pokemon-game

> ЭТОТ ФАЙЛ — память ИИ по проекту, живёт ВНУТРИ проекта (не на рабочем ПК).
> По решению владельца (2026-07-17) НИЧЕГО про pokemon-game НЕ хранить в личной
> памяти ассистента на серверном ПК — только здесь. Новый чат: читать этот файл +
> HANDOFF_main.txt (журнал тимлида) + нужные HANDOFF_*.txt.
>
> Проект: Go + SQLite + серверный HTML + админка. Бой PvE (игрок vs дикий).
> Диск: `\\tsclient\D\pokemon-game` (проброшен из RDP-клиента). На RDP-машине Go
> НЕТ (компилировать нельзя), есть python+sqlite3. Модель работы: тимлид пишет ТЗ
> (HANDOFF_<тема>.txt) → ИИ-агент делает → ревью по коду → отчёт TEST_<тема>.txt.
> Хобби-проект без git-прод-пайплайна: «готово» = живой прогон OK (билд/vet — не
> финал), деплой/мердж как DoD тут НЕ применяется.

---

## КОНВЕНЦИИ / ОБРАТНАЯ СВЯЗЬ ВЛАДЕЛЬЦА

### Атаки — по названию, не по id
Владельцу называть атаки РУССКИМ НАЗВАНИЕМ, не числовым id из БД.
- Why: владелец не программист, id не помнит; по названию понимает сразу.
- Apply: в ответах «Регенерация», «Древняя сила» и т.п. Id — только в скобках при
  неоднозначности или в коде/TEST (там нужен для диспетчеризации).

### Роль ассистента
Только разведка + ТЗ. Реализацию (код) делает ИИ-агент, сборку/прогон — владелец
на осн. ПК. Растровый арт ИИ НЕ рисует — PNG-ассеты поставляет владелец.

---

## ПОЛ + ПРИВЛЕКАТЕЛЬНОСТЬ (реализовано 2026-07-15)
У экземпляра `user_pokemons`: `gender` ('M'/'F'/'N'), `attractiveness` ('A'/'B'/'C').
- **ЕДИНАЯ ТОЧКА ПРАВДЫ** — `server/db/gender.go`: `RollGender(pokemonID)` (пол по
  ратио вида `pokemons.male/female_percentage`; обе доли 0 → 'N') и
  `RollAttractiveness()` (веса в `attractivenessLetters`). ЛЮБАЯ точка создания
  экземпляра (ловля, разведение) ОБЯЗАНА звать эти хелперы, не дублировать.
- РАЗВЕДЕНИЕ [13] ГОТОВО (2026-07-21): `db.BreedCompat(a,b)→(ok,причина)` — боевое
  правило (один вид + одна буква + M/F ЛИБО N/N с Амуритом на обоих; Амурит =
  items.effect_type='breed_amurite', id=150, не расходуется). Старая `db.CanBreed`
  осталась как есть, но НЕ используется разведением. Логика — `handlers/api/breeding.go`
  (baseFormOf, childIV max+30%/кап31, breed/eggs/hatch, ручки incubation=1h/level=1).
  Таблица `eggs`. Потомок только через `utils.CreateUserPokemon`. Фронт — в #nursery-panel.
- Гендерные атаки (`gender_moves.go`, гейт `oppositeGender`): 392 Соблазн
  (влюблённость ~50% пропуск хода) и 406 Обаяние (СпАтака −2), обе disabled=0.
  PENDING: у 406 Обаяние 0 носителей в `pokemon_attacks` — кому выдать, решает владелец.

---

## [5] ПРЕДМЕТЫ: ЛОВЛЯ + МОНСТРОБОЛЫ — ГОТОВО и ПРОВЕРЕНО (2026-07-15)
Фаза 1 (ловля покеболами) и фаза 2 (условные болы) собраны, живой прогон OK.
- **ЕДИНАЯ точка создания экземпляра — `utils.CreateUserPokemon`** (в utils, НЕ в db —
  из-за цикла импорта db→utils→db; admin.go GivePokemonHandler отрефакторен на неё).
  Расширена параметрами geneBonus/happinessBonus/forceNursery.
- Бой серверный stateful: `battleSessions` + `battleMu` (battle.go). Ловля:
  `catch.go` CatchHandler + catchChance + `/api/battle/catch`; формула
  (3*MaxHP−2*HP)/3MaxHP * catch_rate * ballMult * statusMult; IV/nature дикого
  протянуты (wild.go WildPokemonGenerate.IV → BattleState.WildIV/WildNatureID).
  Фронт: кнопка 🎯 + ball-picker + performCatch. Enemy-turn при провале —
  переиспользован блок из SwitchHandler.
- Слоты: position 1..6 команда, 0/NULL питомник. ⚠️ Побочка рефактора: admin-выдача
  теперь в первый свободный слот команды, а не всегда в питомник.
- Схема: items.catch_multiplier (×100), pokemons.catch_rate (деф.45); фаза2 добавила
  items.catch_condition('type'/'speed_gt'/'level_lt'/'opposite_gender')+catch_cond_value,
  gene_bonus(+IV кап31), happiness_bonus, force_nursery; catch.go ballMultiplierFor().
- Болы: id1 покебол, 101 Great, 102 Ultra, 103 Master; +26 монстроболов (id104-129) по
  вики league17 (Трансбол/Фаст/Френд/Лав/Нест/Дарк/Супердарк/Люкс + 18 типовых).
  Множители дефолт: типовые/Фаст/Нест ×3, Лав ×4, Great ×1.5, Ultra ×2.
  Пропущены 3 (нет цели в БД, 151 монстр gen1): Новичка, Лайтбол, Браконьера.
  Шайни-бол не делал (шайни-системы нет; есть attractiveness A/B/C).
- Вне боя уже было: GetInventory/UseItem (heal_hp/heal_status/revive/candy) + 🎒-док.
- [smallfixes 2026-07-22] У КАЖДОГО из 30 болов теперь СВОЙ спрайт (было: у всех
  pokeball.png — чисто данные items.image_url). Источник арта — вики league17
  (img.league17.ru/pub/balls/tiny/<wikiID>.png, 85×85 RGBA, тот же стиль, что
  pokeball.png). 18 «типовых» болов — выдуманные, офиц. спрайта Pokémon нет, взяты
  с league17 по решению владельца. Имена: great_ball.png…fairy_ball.png. Декор-бол
  game.js:330 (pokeball.png на карточке) — НЕ инвентарный, не трогать.
- ФАЗА 3 (НЕ делал): магазин/валюта (items.price проставлены, купить негде), ягоды,
  камни эволюции, карточка-форма бола, анимация покачиваний, условные болы
  Dusk/Quick/Timer/Heavy/Repeat.

---

## [6] БОЕВЫЕ (НАДЕВАЕМЫЕ/held) ПРЕДМЕТЫ, шаг 1 — ГОТОВО и ПРОВЕРЕНО (2026-07-16)
«STAB-усилитель» (по факту ТИПОВОЙ усилитель урона). Механики held в проекте НЕ
было — построена с нуля.
- **Эффект (решение владельца):** у предмета есть ТИП; если тип предмета == тип
  покемона-носителя (двутипный — любой из двух) → плоский множитель ко ВСЕМ
  урон-атакам носителя. Гейт по типу ПОКЕМОНА, НЕ атаки (это НЕ настоящий STAB).
  stab не трогали — отдельный `heldMult` в calculateDamage (итоговая строка урона):
  `heldMult=1.0; if HeldAtkMult>1 && HeldItemTypeID!=0 && pokemonHasType(attacker,HeldItemTypeID) → heldMult=HeldAtkMult`.
  Хелперы pokemonHasType/getPokemonTypes уже были. Дикие held не носят.
- Схема: user_pokemons.held_item_id; items.type_id; категория item_categories(9,'held');
  предмет effect_type='type_boost', effect_value=×100. 18 предметов по типам id130-147,
  effect_value=110 (×1.1, «+10% STAB» по вики league17/Модификаторы), выданы users1-4.
  Иконки league17 (stab_<typeid>.png), фон убран (Pillow).
- Код: db/items.go (Item.TypeID/Description, IsHeldItem, HoldItem/UnholdItem —
  транзакц. свап через user_items); held_items.go (Hold/UnholdItemHandler); routes +2;
  battle.go (поля Held*, чтение в playerPokemonToBattle ~1780/1991); user_pokemons.go
  GetUserTeam отдаёт held*; admin items_admin.go+item_form.html (создание held).
- Фронт game.js: слот в карточке команды (heldSlotHtml/openHeldPicker/hold/unhold),
  боевая карточка (createBattleCard заполняет .items). По просьбе владельца: иконку
  ТИПА убрали; тултип = held_item_description (что делает), не имя.
- ВНИМАНИЕ: battle_card.html в проекте НЕТ (боевая карточка рисуется в game.js).
- ДАЛЬШЕ (отдельные HANDOFF): другие held-эффекты (ягоды/восстановление/статы),
  held у диких, снятие атаками (Knock Off/Trick), расходуемые ягоды.

---

## [11] NPC + ДИАЛОГИ (поток A) — ГОТОВО и ПРОВЕРЕНО (2026-07-17). ТЗ: HANDOFF_npc_dialogue.txt
Живой прогон OK (сборка на основном ПК, клик по Ричи работает).
Схема: dialogue_nodes(npc_id, node_key, speaker_text, is_start; ровно один старт на NPC),
dialogue_options(node_id, sort_order, option_text, next_node_id, action_type NULL|'battle'|'close',
+action_param/condition_flag — ЗАДЕЛ под квесты, логики пока НЕТ). ALTER npc: team_mode
('random'|'fixed'), team_size 1..6, min_level, max_level — настройки команды для боя
редактируются в админке, но команда НЕ генерируется (это поток B).
Бэкенд: handlers/api/dialogue.go (GET /api/npc/{id}/dialogue, POST /api/npc/dialogue/option;
приоритет действий battle→node→close), has_dialogue в location.go (клиент подсвечивает NPC).
Админка: handlers/admin/dialogue.go (CRUD узлов/опций) + npc.go (поля команды, каскадное
удаление диалога в DeleteNPC) + templates/admin/npc_form.html (блоки «Команда для боя» и
«Диалог»). Гварды: один is_start, next_node только этого NPC.
UI-РЕШЕНИЕ ВЛАДЕЛЬЦА: диалог НЕ всплывающее окно, а ВСТРОЕН в панель «Персонажи»
(превью-вариант 11, он же бывший «drawer»/вариант 10, но не на всю высоту). Клик по NPC
подменяет содержимое .npc-block: прячется #npc-list и сервисы, показывается #npc-dialogue
(портрет+имя+текст+ответы), «← Назад к персонажам» возвращает список. game.html/#npc-dialogue,
game.js (openDialogue/closeDialogue/renderDialogueNode/chooseOption), game.css (.npc-dialogue*,
.npc-block стал flex-колонкой). Превью всех вариантов — dialogue_preview.html (стартует на 11).
СТЫК С ПОТОКОМ B (тренерский бой) — одна точка: клиент startTrainerBattle(npcId) +
сервер {kind:'battle', npc_id}. РЕАЛИЗОВАНО в [12].

---

## [12] ТРЕНЕРСКИЙ БОЙ vs КОМАНДА NPC (поток B) — ГОТОВО и ПРОВЕРЕНО (2026-07-20). ТЗ: HANDOFF_npc_battle.txt
Живой прогон OK (сборка+vet зелёные, все пункты DoD в TEST_npc_battle.txt=[x],
regression дикого боя подтверждён). Продолжение [11].
СХЕМА: таблица npc_pokemons(npc_id, pokemon_id, level, position) — фикс-ростер
(только для team_mode='fixed'). Поля команды на npc (team_mode/size/min/max) — от [11].
ЯДРО-ПРИЁМ («указательный трюк», минимум правок движка): BattleState +EnemyTeam[]
+EnemyActiveIdx +TrainerName; bs.WildPokemon ВСЕГДА = &EnemyTeam[idx] активного врага.
Срез EnemyTeam строим ОДИН раз, НЕ append-им; смена врага = только индекс+указатель
(иначе указатель повиснет). Для BattleType='wild' EnemyTeam пуст (как было).
Оба victory-места (MoveHandler ~2219, SwitchHandler ~2450) заменены на handleEnemyFaint:
wild -> враг пал=конец (awardExp за дикого); trainer -> следующий живой враг
(applyEntryHazards его стороны, лог «<Тренер> отправляет <имя>»), победа только когда
пала ВСЯ команда, ОПЫТ НИКОМУ. buildPlayerTeam — общий хелпер команды игрока (одна
точка правды для обоих стартов). wild.go: ядро генерации вынесено в
buildGenerateFor(pokemonID,level) — им же строится команда NPC.
[smallfixes 2026-07-22] IV-роллы в buildGenerateFor: rand.Intn(6)→rand.Intn(32)
(0..5 -> 0..31, greenlight владельца, баланс). Т.к. это ЕДИНОЕ ядро — сильнее по
генам стали и дикие, и команды NPC (random/fixed). Если для NPC нужен старый узкий
диапазон — развести роллы параметром.
ФАЙЛЫ: handlers/api/npc_battle.go (НОВЫЙ: TrainerBattleStartHandler,
buildNPCTeam random/fixed, handleEnemyFaint, fillEnemyIndicator); battle.go (поля+
victory-логика+buildPlayerTeam); wild.go (buildGenerateFor); routes.go
(/api/battle/trainer/start + /admin/npc/team/{add,edit,delete}); admin/npc.go (каскад
npc_pokemons + ростер-данные); admin/npc_team.go (НОВЫЙ CRUD ростера); npc_form.html
(секция ростера при fixed); game.js (startTrainerBattle реальный старт -> общий
startBattle; enemy_switch перерисовывает врага; performSwitch тоже; индикатор отряда
updateEnemySquad; «Вы победили тренера X!» без exp); game.css (шарики отряда).
Ловля в тренерском бою уже режется catch.go (BattleType!='wild').
ТЕСТ-NPC: Ричи(id1) random(3,8-12); Тренер Ли(id2,loc2) fixed ростер Бульбазавр10/
Чармандер12 + диалог-бой.
ОСТАТКИ (MVP, согласовано ТЗ): фазинг врага=no-op; двойной обморок за ход->need_switch
(карточка врага освежается при след. смене); победа над диким от остаточного урона в
Switch — без опыта (сохранено исходное поведение, НЕ регресс).

---

## [17] ОНЛАЙН-ИГРОКИ НА ЛОКАЦИИ + ВЫЗОВ НА БОЙ (часть A) — КОД ГОТОВ (2026-07-22). ТЗ: HANDOFF_online_players.txt, отчёт: TEST_online_players.txt
Presence + флоу вызова, БЕЗ самого PvP-боя (часть B — позже). Живой прогон ДВУМЯ
игроками на осн.ПК — PENDING (Go на RDP нет). БД НЕ трогалась (всё в памяти).
ТРАНСПОРТ: HTTP-поллинг, но модель СЕРВЕРНАЯ (presence из БД, вызовы в памяти) ->
позже ws-пуш без смены форматов.
БЭКЕНД handlers/api/online.go (НОВЫЙ): ручки onlineWindow=5мин, challengeTTL=45с.
Presence НЕ хранится — на лету из users.last_seen+location_id (то же правило онлайна,
что в [7]: strftime, <300с). Вызовы эфемерные: map challenges (ключ
"challengerID-targetID") + challengeMu, как battleSessions; своя init()-горутина
cleanupChallenges + инлайн-чистка протухших. 5 эндпоинтов: GET /api/location/players;
POST /api/challenge {target_user_id} (гарды: не себя/цель онлайн+та же локация/нет
дубля); GET /api/challenge/incoming; POST /api/challenge/respond {challenger_id,accept};
GET /api/challenge/outgoing {status,target_name}. routes.go +5 маршрутов.
СТЫК С ЧАСТЬЮ B — ОДНА точка: startPvpBattle(challengerID,targetID) — клиентская
заглушка (game.js, console.log) + серверная (online.go, log + пометка accepted).
TODO(Часть B) в обоих местах.
ФРОНТ: game.html #online-players (.online-panel «🟢 В СЕТИ») — правая КОЛОНКА ВНУТРИ
.chat-panel (место бывшего .users-block, flex:1 рядом с .chat-block). ГРАБЛЯ: сначала
повесил её отдельным flex-ребёнком #divVisios (3-я колонка) — СЛОМАЛО верстку
(location+chat уже в ряд); вернул внутрь .chat-panel. + #online-player-menu +
#challenge-popup. game.js
(секция в конце + хук restartOnlinePolling в loadLocation): поллинги онлайн ~7с/
входящие ~5с(всегда)/исходящий ~2с(пока ждём); контекст-меню по клику иконки
(«Тренеркарта» задизейблена/тост, «Вызвать на бой» активна) по образцу
showBattleSwitchMenu; попап входящего; showToast (в проекте не было). game.css
.online-*/.challenge-*/.pk-toast (--pk-*).
ГРАБЛЯ/ЗАМЕТКА: пре-existing .users-block («👥 ИГРОКИ ОНЛАЙН», ХАРДКОД Игрок1/2/
Тренер Ли) внутри .chat-panel УДАЛЁН (по просьбе владельца) — живая панель это
#online-players. CSS-классы .users-block/.user-list/.user остались неиспользуемыми
(безвредны). last_seen троттлится 60с (учесть при проверке «ушёл офлайн»).

---

## [17B] PvP-БОЙ МЕЖДУ ИГРОКАМИ — КОД ГОТОВ (2026-07-23). ТЗ: HANDOFF_pvp_battle.txt, отчёт: TEST_pvp_battle.txt
Продолжение [17A]. Живой прогон ДВУМЯ игроками на осн.ПК — PENDING (Go на RDP нет).
БД структурно не трогалась (сессии в памяти; персист боя пишет в существующие
user_pokemons/…learnable_attacks). Бэкап перед прогоном: pokemon.db.bak-before-pvp.
КЛЮЧ: PvP ≈ тренерский бой, где «враг» — человек. НЕ второй движок. Сторона A =
BattleState.PlayerTeam/ActiveIdx, сторона B = EnemyTeam/EnemyActiveIdx + WildPokemon
(указатель на активного B — «указательный трюк» из [11]B). Примитивы боя
(executeDamagingMove/applyEffectByID/applyStatChanges/applyHeal/handleCustomStatusMove/
applySideMove/handlePhazeMove/handleCallMove/sideFor) переиспользованы НАПРЯМУЮ.
ФАЙЛЫ: handlers/api/pvp.go (НОВЫЙ: PvpBattle/pvpSessions/pvpMu, startPvpBattle реальный,
pvpResolveTurn=зеркало processTurn+хвост MoveHandler для ДВУХ людей, pvpSwitchB=зеркало
performInBattleSwitch для стороны B, pvpForceMove=форс-состояния обеих сторон,
savePvpState=персист обеих команд, 6 эндпоинтов); online.go (стаб startPvpBattle УДАЛЁН,
реальный в pvp.go, убран импорт log); routes.go (+/api/pvp/{current,state,lead,action,
replace,forfeit}); battle.go НЕ трогался (processTurn цел).
ЭНДПОИНТЫ: GET /current (мой активный бой), GET /state?battle_id=&since= (perspective:
моя сторона=player, соперник=enemy, moves соперника вырезаны, курсор лога since/log_seq),
POST /lead|/action|/replace|/forfeit. Фазы: lead_select->battling->await_replace->finished.
Ход разрешается РОВНО когда сдали ОБА (под pvpMu). Персист по ходу.
ФРОНТ game.js: startPvpBattle->enterPvpBattle (GET /current -> боевой UI, поллинг state
~1.5с, catch-btn скрыт); pvpRefresh рендерит по фазе, реюз createBattleCard/
updateBattleCardHp/renderTeamSelection-стиль. РОУТИНГ-ГВАРДЫ в начале performMove
(->pvpAction move), performSwitch (->pvpReplace в await_replace, иначе pvpAction switch),
runBattle (->exitPvp/pvpForfeit). pvpState=глоб. флаг PvP-режима.
РЕШЕНИЯ ВЛАДЕЛЬЦА (учтены): выбор лида ВСЛЕПУЮ каждым; урон ПЕРСИСТИТСЯ в реальные
user_pokemons обеих команд; опыта/наград/ловли НЕТ; транспорт поллинг.
ОСТАТКИ (в TEST): ходы-со-сменой-внутри-хода/Me First не спец-обработаны (смена=отдельное
действие); handlePhazeMove в PvP не тестирован (риск указателя B); обрыв игрока лечится
кнопкой Сдаться или TTL 3мин->finished/ничья (LastActivity на действиях, не на поллинге);
pvpSwitchB дублирует exit-clear волатилей performInBattleSwitch — держать синхронно.
ГРАБЛЯ-ЗАДЕЛ: BattleType="pvp" -> ловля (catch.go !="wild") и ИИ-тренера (EnemySmart)
не включаются; ActiveIdx/EnemyActiveIdx=-1 до выбора лида (sideFor/executeDamagingMove
звать только в battling, иначе паника на PlayerTeam[-1]).

---

## БЭКЛОГ (потоки [7]–[10]; порядок = приоритет)

### [7] КАРТОЧКА ТРЕНЕРА — ГОТОВО и ПРОВЕРЕНО (2026-07-16). ТЗ: HANDOFF_trainer_card.txt
Полностью по ТЗ, живой прогон OK. Схема: users(+status_text,role_id,last_seen,
avatar_data), player_roles(сид 5: Тренер/Админ/Гим лидер/Полицейский/Наставник),
items.is_award, user_friends, user_wishlist. Бэкенд: db/profile.go, handlers/api/
profile.go (GET /api/profile[/{id}], POST /api/profile/status; пароль/почту не
отдаёт; статус только себе, лимит 100). Онлайн = last_seen<5мин через
lastSeenMiddleware (троттлинг 60с). Награды = предмет с items.is_award (выдача
give_item), исключены из сумки. Фронт: иконка 👤 → док #trainer-dock, КОМПОНОВКА A
(крупный аватар-силуэт слева + вкладки Награды/Желаемые/Друзья). Друзья/желаемые —
только показ. Клик по другу → чужая карточка read-only. Роли пока через БД.

### [8] КЛАНЫ — ТЗ НЕТ (проектировать с нуля)
Таблицы clans + clan_members (user_id, clan_id, роль), создание/вступление/выход/
лидер, показ в карточке [7]. Объём (права/казна/клановые бои) — уточнить у владельца.

### [9] СПОСОБНОСТИ (АБИЛКИ) — КАРКАС ГОТОВ И ПРОВЕРЕНО (2026-07-22). ТЗ: HANDOFF_abilities_framework.txt
Часть 1 (инфраструктура) сделана; живой прогон OK (build+vet зелёные, все DoD [x] в
TEST_abilities_framework.txt). Эффекты «по полкам» — СЛЕДУЮЩИЙ чат.
СХЕМА (миграции в db/database.go, fire-and-ignore ALTER): abilities.effect_key;
pokemon_abilities.slot (1=баз.X,2=баз.Y,3=скрытая, slot=3<=>is_hidden=1);
user_pokemons.ability_id/ability_slot. Миграция слотов + бэкфилл — идемпотентны в
db/abilities.go (migrateAbilitySlots/backfillInstanceAbilities).
ХЕЛПЕРЫ db/abilities.go (НОВЫЙ): SpeciesBasicAbilities/SpeciesHiddenAbility/
RollBasicAbility/AbilityEffectKey/AbilityNameKey/AssignBasicAbility + RemapAbilityOnEvolve.
ЕДИНЫЕ ТОЧКИ: создание = utils.CreateUserPokemon->AssignBasicAbility (случайная БАЗОВАЯ,
скрытая не выпадает; нет базовых->NULL). Ремап при эволюции = db.RemapAbilityOnEvolve,
зовётся из ДВУХ мест (грабля!): evolution.go (EvolveHandler, старый вид ДО UPDATE) И
levelup.go (applyLevelUp — авто-эволюция после боя из battle.go awardExp И от Леденца
inventory.go). Правило слота: скрытая->скрытая; базовая — если число базовых пре==пост
сохранить слот, иначе случайная из базовых новой формы.
ОТОБРАЖЕНИЕ: GetUserTeam берёт ability_name по up.ability_id (не «первую вида на лету»).
Покедекс НЕ трогали (видовая энциклопедия — решение владельца).
БОЙ: BattlePokemon+AbilityID/Key/Name; playerPokemonToBattle грузит у игрока;
wildToBattlePokemon даёт транзиентную случайную базовую (ОДНА точка — покрывает и
дикого, и тренерского, т.к. buildNPCTeam идёт через неё). ДИСПЕТЧЕР
handlers/api/abilities_battle.go (НОВЫЙ): 4 хука ПОКА нейтральны (1.0/false/no-op) +
демо-ветка effect_key='logtest' (лог при выходе). Хук-точки помечены «// [ability hook]»:
onSwitchIn — в applyEntryHazards (hazards.go, единый choke всех входов) + лиды в обоих
стартах; modifyOutgoing/IncomingDamage — множители в calculateDamage; blocksStatus — в
applyEffectByID перед наложением статуса.
АДМИНКА: effect_key у ability; slot(1/2/3) у pokemon_abilities (is_hidden из слота).
ТЕСТ-ДАННЫЕ (были ПУСТЫ: pokemon_abilities 0 строк): засеян ДЕМО-каталог (13 абилок,
logtest у 5) на линии Бульбазавра(1/2/3) и Венонат(48/49) + виды команды владельца;
#349/#350 из ТЗ в базе НЕТ (gen1) -> спец-случай 1->2 перенесён на #1->#2. 3 тест-
покемона nickname='ABILTEST' (поз.4/5/6, user2) для проверки эволюции.
КАТАЛОГ ЗАЛИТ + ДЕМО УБРАН (2026-07-22): демо-скаффолд удалён (заглушки + все
pokemon_abilities + ability_id экземпляров обнулены + logtest снят). В `abilities`
теперь 220 РЕАЛЬНЫХ способностей с рус.описаниями (импорт с вики league17). effect_key
ПУСТ у всех — заполняется под конкретный эффект при реализации в бою. pokemon_abilities=0,
user_pokemons.ability_id=NULL (чистое состояние; бэкфилл сработает, когда появятся
назначения). ЭФФЕКТЫ БАТЧ 1 ГОТОВ (2026-07-22, код; отчёт TEST_abilities_effects.txt): 16 абилок
на существующих 4 хуках, БЕЗ правок движка. abilities_battle.go — реальные switch-case;
battle.go — лог при блоке статуса. effect_key проставлены в БД: swarm/overgrow/blaze/
torrent (пинч ×1.5 по типу при HP<⅓), pure_power/huge_power (физ ×2), reckless (отдача
×1.2), thick_fat (½ огонь/лёд вход), fur_coat (½ физ вход), immunity/limber/insomnia/
vital_spirit (блок яд/паралич/сон), drought/drizzle/snow_warning (погода на выходе через
setBattleWeather). ЖИВОЙ прогон эффектов — ПОСЛЕ назначения видам (сейчас назначений 0).
ЭФФЕКТЫ БАТЧ 2 ГОТОВ (2026-07-22, код): +15 абилок, 2 НОВЫХ хука в executeDamagingMove:
abilityAbsorbsMove (поглощение по типу ДО урона: flash_fire/volt_absorb/water_absorb/
motor_drive/sap_sipper/storm_drain) и abilityOnHitByMove (ответ при контакте ПОСЛЕ урона:
rough_skin/iron_barbs/static/flame_body/poison_point/gooey/cute_charm). Добор на старых
хуках: tough_claws, fluffy, flash_fire-буст. Новое поле BattlePokemon.FlashFireBoosted;
хелперы abilityBumpStat/abilityHeal. Итого effect_key проставлено: 31. Грабля: поглощение
только урон-атак (статус-атаки типа не поглощаются — батч 3). Левитации в каталоге нет.
ЭФФЕКТЫ БАТЧ 3 ГОТОВ (2026-07-22, код): +10 абилок. НОВЫЙ хук abilityOnEndTurn (в
MoveHandler после tickSideTimers, оба активных): speed_boost/rain_dish/ice_body/shed_skin.
НОВЫЙ хук abilityBlocksStatDrop (в applyStatChanges рядом с Туманом): clear_body (Чистое
тело/Белый дым). Расширения: filter (Фильтрация/Монолит/Рефрактор, ¾ по эффективным —
через getTypeEffectiveness в incoming-хуке) и intimidate (Яростный взгляд, −атака врагу в
onSwitchIn, +хелпер abilityOpponentOf). Итого effect_key: 41 строка / 38 ключей. Грабли:
onEndTurn только в MoveHandler (не в SwitchHandler); blocksStatDrop = блок всех статов.
Отложено (батч 4): точность/крит, статус-поглощение, Токсифаг (тик яда), погодные мультихук
(Сухая кожа/Солн.батарея), формы.
НАЗНАЧЕНИЯ ВИДАМ gen1 ГОТОВЫ (2026-07-22): pokemon_abilities заполнена с вики (инверсия
|Способность X/Y=|Скрытая| по нац.дексу, декс≤151). 368 строк / 151 вид (slot1=151 у ВСЕХ,
slot2=87, slot3=130); 0 рассогласований slot<->is_hidden, 0 дублей PK. Каждый вид имеет
базовую X. Канон верен (Charizard=Blaze/Solar Power, Pikachu=Static/Lightning Rod и т.д.).
85 назначений уже с рабочим effect_key (остальные абилки назначены, но инертны — effect_key
пуст, хуки no-op, эффекты добавятся батчами). 12 экземпляров владельца забэкфиллены (многие
с рабочими overgrow/blaze/torrent). Грабля-фикс: Metapod(11)/Kakuna(14) имели Змеиную кожу и
в базовой, и в скрытой → PK(pokemon_id,ability_id) схлопнул в одну строку → промоутнул в slot1.
Бэкап: pokemon.db.bak-before-ability-assign. Парсер/дампы: scratchpad (gen1_assign.json).
ТЕПЕРЬ ВСЁ ЖИВО-ТЕСТИРУЕМО: build+vet + playtest на осн.ПК (DoD батчей 1-3).
ЭФФЕКТЫ БАТЧ 4 ГОТОВ (2026-07-22, код): +7 абилок «точность/крит». Новая точка в checkHit
(compound_eyes ×1.3 точность, bright_body ×0.5 статус-точность врага, keen_eye игнор
уклонения) + хелперы abilityAccuracyMult/abilityIgnoresEvasion. Новая точка в calculateDamage
(battle_armor нет критов по носителю, sniper крит ×2.25) + abilityBlocksCrit. Расширен
blocksStatDrop: keen_eye (точность), big_pecks (защита). Итого 48 effect_key, 114 назначений
с рабочим эффектом. Абилки активны сразу (виды назначены).
ВАЖНО: батчи 1-4 (48 эффектов, 6 хук-точек) ещё НЕ КОМПИЛИРОВАЛИСЬ (нет Go на RDP) —
нужен ЧЕКПОИНТ build+vet+бои на осн.ПК до батча 5. Отложено (батч 5): статус-поглощение,
Токсифаг, погодные мультихук (Сухая кожа/Солн.батарея), Хрупкая броня, формы, sand_veil,
onEndTurn в SwitchHandler.

### [ref] league17 wiki — источник способностей/данных (MediaWiki API)
https://wiki.league17.ru — curl+python работают с этой машины. extracts НЕ установлен →
брать сырой wikitext:
- список: api.php?action=query&list=categorymembers&cmtitle=Категория:Способности&cmlimit=500 (221: 220 abilities + 1 служебная таблица);
- контент: api.php?action=query&prop=revisions&rvprop=content&rvslots=main&titles=A|B|… (до 50 за вызов).
Шаблон {{Способность}}: |название=, |описание= (чистый рус.текст), |Способность X= /
|Способность Y= / |Скрытая способность= со списком носителей {{мNNN}} по НАЦ.ДЕКСУ
(league17 = канон-нумерация!) → готовая привязка вид->абилка->слот для gen1 (декс<=151).
ГРАБЛЯ: англ.оригинала на страницах СПОСОБНОСТЕЙ нет (есть только у ходов, {{Атакдекс}}
|названиеангл=). Рус.имена league17 НЕ совпадают с каноном (Intimidate != «Устрашение»;
«Левитация» = ход Telekinesis) → effect_key маппить по ОПИСАНИЮ, не по имени.

### [10] АВАТАР ТРЕНЕРА + ОДЕЖДА — В РАБОТЕ (стадия арта). Детали: HANDOFF_avatar.txt
PNG paper-doll (SVG отвергнут — «SVG это очень плохо»). База-«болванка» тела
(муж.+жен., без лица/волос/причёски, только бельё) + слои поверх: лицо (глаза/нос/
рот), волосы, одежда (одежда = отдельный предмет). Композиция: прозрачные PNG одного
холста, слои на клиенте (или серверная сборка Go image/draw). РАСТР ИИ НЕ РИСУЕТ —
арт генерирует ВЛАДЕЛЕЦ в Stable Diffusion (промпты болванок в HANDOFF_avatar.txt §4:
faceless-манекен взрослый, бельё, A-pose, холст 832×1216, ControlNet OpenPose для
выравнивания слоёв). Задел от [7]: div.tc-avatar (SVG-заглушка ~game.js 1433),
users.avatar_data, profile.go Avatar. ТЕКУЩИЙ ШАГ: владелец генерит 2 болванки.
ДАЛЕЕ: промпты слоёв лица/волос/одежды → ТЗ агенту на МЕХАНИКУ (слоты одежды,
надевание как held-предметы, рендер слоями; открытые вопросы HANDOFF_avatar.txt §7).
Плюс потом: добавление друзей/желаемых, автовыдача наград.

---

## [gen2] ВТОРОЕ ПОКОЛЕНИЕ #152–251 — ГОТОВО (2026-07-22). ТЗ: HANDOFF_gen2.txt, отчёт: TEST_gen2.txt
Только ДАННЫЕ, Go не трогался (хардкод-лимита 151 в server НЕТ — grep пусто; сервер
достаточно перезапустить). Внесены все 100 видов #152–251 (id=нац.номер), вкл.
легендарных 243–251 обычными видами. Канон — с PokeAPI (статы/типы/пол/base_exp/
вес/catch_rate/эволюции/лёрнсеты), русские имена — транслитерация. Скрипты/кэш —
в scratchpad (вне git). Бэкап: database/pokemon.db.bak-before-gen2.
- **category_power = base_experience** (ГРАБЛЯ): по нему awardExp (battle.go ~2728)
  считает опыт за победу. Проставлено канонично всем 100 (иначе за гена-2 опыт=0).
- **Спрайты**: анимированные .gif с Pokemon Showdown (ani/<eng>.gif) в normal/ И mini/
  (клиент берёт mini заменой "/normal/"→"/mini/" в image_url; shine/ пуст, не нужен).
  Размер варьируется как у гена-1. 100 в каждой папке (стало 251/251).
- **Эволюции**: 42 вида с evolves_from (level29/trade6/stone2/friendship5). Кросс-ген
  (на гена-1): 169←42, 182←44, 186←61, 196/197←133, 199←79, 208←95, 212←123, 230←117,
  233←137, 242←113. Беби-UPDATE гена-1 строк: Пикачу25←Пичу172, Клефэйри35←Клеффа173,
  Джиглипафф39←Иглибафф174 (friendship); Джинкс124←Смучум238, Электабазз125←Элекид239,
  Магмар126←Магби240 (L30). => baseFormOf для этих линий смещается на беби (ОК по ТЗ).
  Иви133 в БД — ЛИНЕЙНАЯ цепочка (134←133←…135←136), НЕ трогалась; 196/197 добавлены
  ветками от 133. Ветка Тайрога по статам не моделируется: задан только 236→237(L20),
  #106/#107 не привязаны (в MISSING).
- **Муваны**: 3422 строки pokemon_attacks (ТОЛЬКО существующие атаки, матч по eng_name;
  level754/tm2399/egg201/tutor68). Гарантия: у каждого вида есть урон-атака (фолбэк по
  типу понадобился 2 видам). Способности гена-2 НЕ проставлены (позже, с каркасом [9]).
- **MISSING_MOVES_gen2.txt** (корень) — вход для след. задачи: 26 недостающих ходов
  (Body Slam/Headbutt/Thunder Punch/Megahorn/Sacred Fire/Steel Wing/Thief… — набора из
  550 атак в них нет) + предметы эволюции которых нет в items (Sun Stone/King's Rock/
  Metal Coat/Dragon Scale/Up-Grade → эволюция задана методом без item_id) + нереализуемые
  условия (день/ночь Иви, развилка Тайрога).
- ЖИВОЙ ПРОГОН OK (2026-07-22). Два фикса всплыли при тесте покедекса (ВАЖНО для гена-3):
  1) **NULL description ломал сервер**: pokedex.go /full и /search сканируют
     description в обычный Go `string` → NULL (я не заполнял при вставке) даёт
     `sql: Scan error` → /full отдаёт 404, а /search молча пропускает строку
     (rows.Scan err → continue) => поиск по id пустой. ФИКС — данные: проставил
     непустой description всем #152–251. ВЫВОД для след. поколений: при вставке видов
     ОБЯЗАТЕЛЬНО заполнять description (непустой), иначе гена-N не открывается/не ищется.
  2) **Хардкод 151 в pokedex.js** (кнопка «След.»): было `pokemon.id < 151` и
     `>= 151` (строки ~421/315). Заменено на динамический `maxPokemonId`, который
     берётся из `/api/pokedex/list` (loadMaxPokemonId при инициализации). Теперь
     навигация покедекса работает для ЛЮБОГО поколения без правок кода — гена-3
     подхватится сам. Правка чисто клиентская (JS), пересборка Go не нужна, только
     Ctrl+F5. Бэкенд лимитов по номеру вида не имеет (grep 151 по server пуст).
  Грабля кэша: Go не шлёт no-cache на /api → браузер кэшировал пустой ответ поиска;
  лечится Ctrl+F5. Если решим бить в лоб — добавить no-cache заголовки на /api (сборка).
- PENDING: реализация 26 ходов гена-2 (MISSING_MOVES_gen2.txt) — владелец добавит позже;
  предметы эволюции; локации дикой ловли (§7 не делалось); особые условия появления
  легендарных.

---

## [ui_fixes2] ПАКЕТ UI-ДОРАБОТОК (3 шт.) — КОД ГОТОВ (2026-07-30). ТЗ: HANDOFF_ui_fixes2.txt, отчёт: TEST_ui_fixes2.txt
СТАТУС НА МОМЕНТ ОСТАНОВКИ: все 3 правки внесены в файлы, БД НЕ трогалась.
`node --check` по game.js/pokedex.js — зелёный (node на RDP ЕСТЬ: C:\Program Files\nodejs).
**PENDING (за владельцем, осн. ПК): go build + go vet + живой прогон 3 пунктов + регрессия.**
ФИКС 2 правил Go => нужна ПЕРЕСБОРКА; ФИКС 1 и 3 чисто клиентские => только Ctrl+F5.
- ФИКС1 «Покедекс» в меню покебола (game.js): ГРАБЛЯ — в DOM карточки команды был
  ТОЛЬКО id ЭКЗЕМПЛЯРА (data-pok-id=p.id), species-id не было вообще; покедекс про ВИД,
  поэтому добавлен отдельный data-species-id=p.pokemon_id, bindBoxLeftEvents кладёт его
  на все .ctx-item рядом с pokId. teamCtxPokedex: closeTeamModal()->openPokedex()->
  selectPokemon(speciesId). Док команды закрываем осознанно: .pokedex-panel fixed/right
  35%/z-index 1000 ложится поверх #team-dock (z-index 5).
- ФИКС2 попап описания способности: устаревшее место в ТЗ — GetUserTeam давно берёт
  способность ЭКЗЕМПЛЯРА (подзапрос по up.ability_id), а не «первую у вида» => добавленное
  ability_description тем же подзапросом СРАЗУ форвард-совместимо с [9], доделывать нечего.
  Фронт: #ability-info-popup (game.html) + showAbilityInfo() по образцу showAttackInfoPopup;
  данные в data-ability-name/desc на .divAbility (без доп. запроса); клик делегированный
  (переживает loadTeam); закрытие вшито в ОБЩИЙ обработчик закрытия попапов с исключением
  .divAbility. Данные ок: abilities 220/220 с непустым description, ability_id заполнен у
  всех покемонов в командах.
  НЕ ДЕЛАЛОСЬ (по ТЗ необязательно): та же плашка в ДЕТАЛИ ПОКЕДЕКСА — там нет клик-цели
  (pokedex.js:305 просто <ul>), и /api/pokedex/{id}/full описание НЕ отдаёт вовсе
  (AbilityInfo = {name,is_hidden}, pokedex.go:61). Цена доделки ~10 строк: +Description в
  AbilityInfo и в SELECT getPokemonAbilities, +клик-цель, +вызов того же showAbilityInfo.
- ФИКС3 ввод количества полем (showQuantitySelector): span -> input[type=number], класс
  .inv-qty-value СОХРАНЁН (старые стили целы). Одна точка правды setQty(val, writeField):
  кламп 1..maxQty + дизейбл +/-. ГРАБЛЯ-РЕШЕНИЕ: поле НЕ перезаписывается на каждый символ
  ввода (иначе «100» не набрать при maxQty=50) — приведение на blur/подтверждении, наружу
  уходит только склампленное. Enter = «Далее», фокус+select при открытии.
