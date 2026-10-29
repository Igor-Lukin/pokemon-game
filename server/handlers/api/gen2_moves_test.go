package api

import (
	"strconv"
	"strings"
	"testing"

	"pokemon-game/server/db"
)

func gen2TestBattle() (*BattleState, *BattlePokemon, *BattlePokemon) {
	atk := BattlePokemon{
		Name: "Атакующий", Level: 50, MaxHP: 500, CurrentHP: 500,
		PrimaryTypeID: 1, Attack: 100, Defense: 100, Speed: 200,
		SpecialAttack: 100, SpecialDefense: 100, HeldAtkMult: 1.0,
	}
	def := BattlePokemon{
		Name: "Защитник", Level: 50, MaxHP: 500, CurrentHP: 500,
		PrimaryTypeID: 1, Attack: 100, Defense: 100, Speed: 50,
		SpecialAttack: 100, SpecialDefense: 100, HeldAtkMult: 1.0,
	}
	bs := &BattleState{
		PlayerTeam: []BattlePokemon{atk}, ActiveIdx: 0,
		WildPokemon: &def, BattleType: "wild",
	}
	return bs, &bs.PlayerTeam[0], bs.WildPokemon
}

func gen2TestMove(id, typ, cat, power, acc, hitsMin, hitsMax, effectID, chance int, neverMiss bool) *BattleMove {
	return &BattleMove{
		ID: id, Name: "Тестовый ход", TypeID: typ, CategoryID: cat,
		Power: power, Accuracy: acc, PP: 10, MaxPP: 10,
		HitsMin: hitsMin, HitsMax: hitsMax,
		EffectID: effectID, EffectChance: chance, NeverMisses: neverMiss,
	}
}

func runTestHit(t *testing.T, bs *BattleState, atk, def *BattlePokemon, move *BattleMove) ([]BattleLogEntry, bool) {
	t.Helper()
	var log []BattleLogEntry
	fainted, _ := executeDamagingMove(bs, atk, def, move, 0, 1, &log,
		sideFor(bs, def), sideFor(bs, atk), false)
	return log, fainted
}

func ensureTestItems(t *testing.T) {
	t.Helper()
	if _, err := db.DB.Exec(`CREATE TABLE IF NOT EXISTS item_categories (id INTEGER PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.Exec(`CREATE TABLE IF NOT EXISTS items (
		id INTEGER PRIMARY KEY, name TEXT, description TEXT, category_id INTEGER,
		price INTEGER, effect_type TEXT, effect_value INTEGER, image_url TEXT,
		catch_multiplier INTEGER, catch_condition TEXT, catch_cond_value INTEGER,
		gene_bonus INTEGER, happiness_bonus INTEGER, force_nursery INTEGER, type_id INTEGER)`); err != nil {
		t.Fatal(err)
	}
	db.DB.Exec(`INSERT OR REPLACE INTO item_categories (id, name) VALUES (9, 'Held')`)
	if _, err := db.DB.Exec(`INSERT OR REPLACE INTO items
		(id, name, description, category_id, price, effect_type, effect_value, image_url,
		 catch_multiplier, catch_condition, catch_cond_value, gene_bonus, happiness_bonus,
		 force_nursery, type_id)
		VALUES (9001, 'Тестовый предмет', 'Описание', 9, 100, 'type_boost', 120, '',
		        100, '', 0, 0, 0, 0, 1)`); err != nil {
		t.Fatal(err)
	}
}

type gen2MoveSpec struct {
	id, typ, cat, power, acc, min, max, effect, chance int
	neverMiss                                          bool
}

var gen2MoveSpecs = []gen2MoveSpec{
	{552, 1, 1, 15, 100, 1, 1, 11, 100, false},
	{553, 1, 1, 85, 100, 1, 1, 2, 100, false},
	{554, 1, 1, 18, 100, 2, 5, 0, 0, false},
	{555, 1, 1, 50, 100, 1, 1, 0, 0, false},
	{556, 10, 1, 80, 100, 1, 1, 0, 0, false},
	{557, 1, 1, 100, 100, 1, 1, 0, 0, false},
	{558, 1, 1, 40, 100, 1, 1, 0, 0, false},
	{559, 16, 1, 60, 100, 1, 1, 0, 0, true},
	{560, 1, 1, 15, 100, 2, 5, 0, 0, false},
	{561, 12, 1, 40, 100, 1, 1, 0, 0, false},
	{562, 1, 1, 70, 100, 1, 1, 8, 100, false},
	{563, 1, 1, 65, 100, 1, 1, 0, 0, false},
	{564, 1, 1, 80, 100, 1, 1, 0, 0, false},
	{565, 12, 1, 120, 100, 1, 1, 0, 0, false},
	{566, 11, 3, 0, 100, 1, 1, 0, 0, false},
	{567, 1, 1, 20, 100, 1, 1, 0, 0, false},
	{568, 2, 1, 100, 100, 1, 1, 3, 100, false},
	{569, 7, 1, 1, 100, 1, 1, 0, 0, false},
	{570, 1, 1, 200, 100, 1, 1, 0, 0, false},
	{571, 1, 1, 80, 100, 1, 1, 0, 0, false},
	{572, 17, 1, 70, 100, 1, 1, 0, 0, false},
	{573, 1, 1, 65, 100, 1, 1, 8, 100, false},
	{574, 16, 1, 60, 100, 1, 1, 0, 0, false},
	{575, 4, 1, 75, 100, 1, 1, 2, 100, false},
	{576, 7, 1, 10, 100, 3, 3, 0, 0, false},
	{577, 1, 1, 15, 100, 1, 1, 11, 100, false},
}

func TestGen2AllMovesExecuteWildAndTrainer(t *testing.T) {
	for _, m := range gen2MoveSpecs {
		for _, mode := range []string{"wild", "trainer"} {
			t.Run(mode+"/"+strconv.Itoa(m.id), func(t *testing.T) {
				bs, atk, def := gen2TestBattle()
				bs.BattleType = mode
				if mode == "trainer" {
					bs.EnemyTeam = []BattlePokemon{*def}
					bs.EnemyActiveIdx = 0
					bs.WildPokemon = &bs.EnemyTeam[0]
					def = bs.WildPokemon
				}
				move := gen2TestMove(m.id, m.typ, m.cat, m.power, m.acc, m.min, m.max, m.effect, m.chance, m.neverMiss)
				if m.id == 566 {
					atk.DamageTakenSpec = 20
					var log []BattleLogEntry
					if !handleCustomStatusMove(bs, atk, def, move, 1, &log) {
						t.Fatal("Mirror Coat не обработан")
					}
					if def.CurrentHP >= 500 {
						t.Fatal("Mirror Coat не вернул урон")
					}
					return
				}
				log, _ := runTestHit(t, bs, atk, def, move)
				if len(log) == 0 {
					t.Fatal("нет записей в логе")
				}
			})
		}
	}
}

func TestGen2AllMovesThroughProcessTurn(t *testing.T) {
	for _, m := range gen2MoveSpecs {
		for _, mode := range []string{"wild", "trainer"} {
			t.Run(mode+"/"+strconv.Itoa(m.id), func(t *testing.T) {
				bs, atk, def := gen2TestBattle()
				bs.BattleType = mode
				if mode == "trainer" {
					bs.EnemyTeam = []BattlePokemon{*def}
					bs.EnemyActiveIdx = 0
					bs.WildPokemon = &bs.EnemyTeam[0]
					bs.TrainerName = "Тренер"
					def = bs.WildPokemon
				}
				def.MaxHP, def.CurrentHP = 100000, 100000
				if m.id == 566 {
					atk.Speed, def.Speed = 10, 300
					def.SpecialAttack = 200
					def.Moves = []BattleMove{*gen2TestMove(9998, 11, 2, 40, 100, 1, 1, 0, 0, false)}
				} else {
					atk.Speed, def.Speed = 300, 10
					def.Moves = []BattleMove{*gen2TestMove(9999, 1, 3, 0, 100, 1, 1, 0, 0, false)}
				}
				move := gen2TestMove(m.id, m.typ, m.cat, m.power, m.acc, m.min, m.max, m.effect, m.chance, m.neverMiss)
				log := processTurn(bs, move, 0)
				if len(log) == 0 {
					t.Fatal("пустой лог боевого хода")
				}
				if m.id == 566 {
					found := false
					for _, e := range log {
						if strings.Contains(e.Message, "возвращает удар вдвое") {
							found = true
						}
					}
					if !found {
						t.Fatalf("Mirror Coat не отразил урон: %+v", log)
					}
				}
			})
		}
	}
}

func TestGen2MultiHitDistribution(t *testing.T) {
	seen := map[int]int{}
	for i := 0; i < 300; i++ {
		bs, atk, def := gen2TestBattle()
		def.MaxHP, def.CurrentHP = 100000, 100000
		move := gen2TestMove(554, 1, 1, 18, 100, 2, 5, 0, 0, false)
		log, _ := runTestHit(t, bs, atk, def, move)
		if len(log) == 0 || log[len(log)-1].HitCount == 0 {
			t.Fatal("multi-hit не попал")
		}
		n := log[len(log)-1].HitCount
		if n < 2 || n > 5 {
			t.Fatalf("hitCount=%d вне 2..5", n)
		}
		seen[n]++
	}
	for n := 2; n <= 5; n++ {
		if seen[n] == 0 {
			t.Fatalf("не встретилось %d ударов: %v", n, seen)
		}
	}
}

func TestGen2BindWrapTrapTurns(t *testing.T) {
	for _, id := range []int{552, 577} {
		minSeen, maxSeen := 99, 0
		for i := 0; i < 100; i++ {
			bs, atk, def := gen2TestBattle()
			move := gen2TestMove(id, 1, 1, 15, 100, 1, 1, 11, 100, false)
			runTestHit(t, bs, atk, def, move)
			if !def.Trapped {
				t.Fatalf("ход %d не поставил ловушку", id)
			}
			if def.TrapTurns < 2 || def.TrapTurns > 5 {
				t.Fatalf("ход %d: TrapTurns=%d вне 2..5", id, def.TrapTurns)
			}
			if def.TrapTurns < minSeen {
				minSeen = def.TrapTurns
			}
			if def.TrapTurns > maxSeen {
				maxSeen = def.TrapTurns
			}
		}
		if minSeen != 2 || maxSeen != 5 {
			t.Fatalf("ход %d: диапазон TrapTurns %d..%d, ожидался 2..5", id, minSeen, maxSeen)
		}
	}
}

func TestGen2FuryCutterScaling(t *testing.T) {
	atk := &BattlePokemon{Name: "A"}
	move := &BattleMove{ID: 561}
	for streak, want := range map[int]int{1: 40, 2: 80, 3: 160, 4: 160} {
		atk.SameStreak = streak
		if got := movePower(atk, &BattlePokemon{}, move); got != want {
			t.Fatalf("streak=%d: power=%d, want %d", streak, got, want)
		}
	}
}

func TestGen2TripleKickPower(t *testing.T) {
	for hit, want := range map[int]int{0: 10, 1: 20, 2: 30} {
		if got := tripleKickPower(10, hit); got != want {
			t.Fatalf("hit=%d: power=%d, want %d", hit, got, want)
		}
	}
	bs, atk, def := gen2TestBattle()
	def.MaxHP, def.CurrentHP = 100000, 100000
	move := gen2TestMove(576, 7, 1, 10, 100, 3, 3, 0, 0, false)
	log, _ := runTestHit(t, bs, atk, def, move)
	if len(log) == 0 || log[len(log)-1].HitCount != 3 {
		t.Fatalf("Triple Kick: ожидалось 3 удара, лог=%+v", log)
	}
}

func TestGen2FalseSwipeLeavesOneHP(t *testing.T) {
	bs, atk, def := gen2TestBattle()
	def.MaxHP, def.CurrentHP = 100, 5
	move := gen2TestMove(558, 1, 1, 200, 100, 1, 1, 0, 0, false)
	log, fainted := runTestHit(t, bs, atk, def, move)
	if def.CurrentHP != 1 || fainted || def.IsFainted {
		t.Fatalf("False Swipe: HP=%d fainted=%v log=%+v", def.CurrentHP, fainted, log)
	}
}

func TestGen2SeismicTossUsesLevel(t *testing.T) {
	bs, atk, def := gen2TestBattle()
	atk.Level = 37
	move := gen2TestMove(569, 7, 1, 1, 100, 1, 1, 0, 0, false)
	log, _ := runTestHit(t, bs, atk, def, move)
	if len(log) == 0 || log[len(log)-1].Damage != 37 {
		t.Fatalf("Seismic Toss: ожидалось 37, лог=%+v", log)
	}
}

func TestGen2SelfDestructFaints(t *testing.T) {
	for _, protected := range []bool{false, true} {
		bs, atk, def := gen2TestBattle()
		def.Protected = protected
		move := gen2TestMove(570, 1, 1, 200, 100, 1, 1, 0, 0, false)
		runTestHit(t, bs, atk, def, move)
		if !atk.IsFainted || atk.CurrentHP != 0 {
			t.Fatalf("Self Destruct (protected=%v): пользователь не упал", protected)
		}
	}
}

func TestGen2ThiefStealsHeldItem(t *testing.T) {
	ensureTestItems(t)
	bs, atk, def := gen2TestBattle()
	def.HeldItemID = 9001
	def.HeldItemName = "Тестовый предмет"
	move := gen2TestMove(574, 16, 1, 60, 100, 1, 1, 0, 0, false)
	log, _ := runTestHit(t, bs, atk, def, move)
	if atk.HeldItemID != 9001 || def.HeldItemID != 0 {
		t.Fatalf("Thief: attacker=%d defender=%d log=%+v", atk.HeldItemID, def.HeldItemID, log)
	}
	if atk.HeldItemName != "Тестовый предмет" {
		t.Fatalf("Thief: имя предмета=%q", atk.HeldItemName)
	}
}

func TestGen2MirrorCoatReflectsSpecial(t *testing.T) {
	bs, atk, def := gen2TestBattle()
	atk.DamageTakenSpec = 20
	atk.DamageTakenPhys = 999
	move := gen2TestMove(566, 11, 3, 0, 100, 1, 1, 0, 0, false)
	var log []BattleLogEntry
	if !handleCustomStatusMove(bs, atk, def, move, 1, &log) {
		t.Fatal("Mirror Coat не обработан")
	}
	if def.CurrentHP != 460 {
		t.Fatalf("Mirror Coat: HP=%d, ожидалось 460", def.CurrentHP)
	}
}

func TestGen2RageRaisesAttack(t *testing.T) {
	bs, atk, def := gen2TestBattle()
	rage := gen2TestMove(567, 1, 1, 20, 100, 1, 1, 0, 0, false)
	runTestHit(t, bs, atk, def, rage)
	if !atk.Raging {
		t.Fatal("Rage не включил состояние ярости")
	}
	def.Raging = true
	hit := gen2TestMove(555, 1, 1, 40, 100, 1, 1, 0, 0, false)
	runTestHit(t, bs, atk, def, hit)
	if def.StatStages[1] != 1 {
		t.Fatalf("Rage: Atk stage=%d, ожидалось 1", def.StatStages[1])
	}
}

func TestGen2SecondaryStatuses(t *testing.T) {
	cases := []struct {
		id, want int
	}{
		{553, 2},
		{575, 2},
		{568, 3},
	}
	for _, tc := range cases {
		bs, atk, def := gen2TestBattle()
		move := gen2TestMove(tc.id, 1, 1, 40, 100, 1, 1, tc.want, 100, false)
		runTestHit(t, bs, atk, def, move)
		if def.StatusID != tc.want {
			t.Fatalf("ход %d: StatusID=%d, ожидалось %d", tc.id, def.StatusID, tc.want)
		}
	}
}

func TestGen2SteelWingRaisesDefense(t *testing.T) {
	bs, atk, def := gen2TestBattle()
	move := gen2TestMove(572, 17, 1, 70, 100, 1, 1, 0, 0, false)
	move.StatChanges = []StatChange{{StatID: 2, Stages: 1, Target: 1, Chance: 100}}
	runTestHit(t, bs, atk, def, move)
	if atk.StatStages[2] != 1 {
		t.Fatalf("Steel Wing: Def stage=%d, ожидалось 1", atk.StatStages[2])
	}
}

func TestGen2SacredFireThaw(t *testing.T) {
	bs, atk, def := gen2TestBattle()
	def.StatusID = 4
	move := gen2TestMove(568, 2, 1, 100, 100, 1, 1, 0, 0, false)
	runTestHit(t, bs, atk, def, move)
	if def.StatusID != 1 {
		t.Fatalf("Sacred Fire: цель StatusID=%d, ожидалось 1", def.StatusID)
	}
	frozen := &BattlePokemon{Name: "Замороженный", StatusID: 4}
	var log []BattleLogEntry
	if !sacredFireCanAct(frozen, move, false, 1, &log) || frozen.StatusID != 1 {
		t.Fatal("Sacred Fire не разморозил пользователя")
	}
}

func TestGen2FeintAttackNeverMisses(t *testing.T) {
	bs, atk, def := gen2TestBattle()
	move := gen2TestMove(559, 16, 1, 60, 1, 1, 1, 0, 0, true)
	for i := 0; i < 100; i++ {
		if !checkHit(bs, move, atk, def, nil) {
			t.Fatal("Feint Attack промахнулся")
		}
	}
}

func TestGen2FlinchSkipsSecondMove(t *testing.T) {
	bs, atk, def := gen2TestBattle()
	headbutt := gen2TestMove(562, 1, 1, 1, 100, 1, 1, 8, 100, false)
	def.Moves = []BattleMove{*gen2TestMove(555, 1, 1, 50, 100, 1, 1, 0, 0, false)}
	log := processTurn(bs, headbutt, 0)
	if atk.CurrentHP != atk.MaxHP {
		t.Fatalf("испуг не остановил второй ход: HP атакующего=%d", atk.CurrentHP)
	}
	found := false
	for _, e := range log {
		if e.Message == def.Name+" испуган(а) и пропускает ход!" {
			found = true
		}
	}
	if !found {
		t.Fatalf("в логе нет сообщения об испуге: %+v", log)
	}
}
