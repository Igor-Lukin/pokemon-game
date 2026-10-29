package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"pokemon-game/server/db"
	"pokemon-game/server/session"
)

func testBattleItem(effectType string, value int) *db.Item {
	return &db.Item{Name: "Тест", EffectType: effectType, EffectValue: value}
}

func TestBattleItemHealHP(t *testing.T) {
	p := &BattlePokemon{Name: "Пикачу", CurrentHP: 50, MaxHP: 100}
	item := testBattleItem(BattleItemHealHP, 10)

	if ok, _ := battleItemUsable(item, p); !ok {
		t.Fatal("ожидалось, что лечение применимо")
	}
	log := applyBattleItem(item, p, 1)
	if p.CurrentHP != 60 {
		t.Fatalf("HP = %d, ожидалось 60", p.CurrentHP)
	}
	if len(log) != 2 || log[1].Heal != 10 {
		t.Fatalf("лог лечения неверен: %+v", log)
	}
}

func TestBattleItemHealHPClampsToMax(t *testing.T) {
	p := &BattlePokemon{Name: "Пикачу", CurrentHP: 95, MaxHP: 100}
	applyBattleItem(testBattleItem(BattleItemHealHP, 100), p, 1)
	if p.CurrentHP != 100 {
		t.Fatalf("HP = %d, ожидалось 100", p.CurrentHP)
	}
}

func TestBattleItemFullRestoreCapsBoosts(t *testing.T) {
	p := &BattlePokemon{Name: "Гengar", CurrentHP: 30, MaxHP: 120}
	p.StatStages[1] = 4
	p.StatStages[2] = 2
	p.StatStages[3] = 6
	p.StatStages[7] = 3

	item := testBattleItem(BattleItemFullRestore, 0)
	if ok, _ := battleItemUsable(item, p); !ok {
		t.Fatal("ожидалось, что Стимпак применим")
	}
	applyBattleItem(item, p, 1)

	if p.CurrentHP != 120 {
		t.Fatalf("HP = %d, ожидалось 120", p.CurrentHP)
	}
	if p.StatStages[1] != 2 || p.StatStages[3] != 2 || p.StatStages[7] != 2 {
		t.Fatalf("бусты выше +2 не срезаны: %v", p.StatStages)
	}
	if p.StatStages[2] != 2 {
		t.Fatalf("буст +2 не должен меняться: %v", p.StatStages)
	}
}

func TestBattleItemFullRestoreUsableOnlyIfNeeded(t *testing.T) {
	p := &BattlePokemon{Name: "Слоупок", CurrentHP: 100, MaxHP: 100}
	item := testBattleItem(BattleItemFullRestore, 0)
	if ok, _ := battleItemUsable(item, p); ok {
		t.Fatal("Стимпак не должен применяться без нужды")
	}
	p.StatStages[1] = 3
	if ok, _ := battleItemUsable(item, p); !ok {
		t.Fatal("Стимпак должен применяться, если есть буст выше +2")
	}
}

func TestBattleItemCureMajorStatuses(t *testing.T) {
	cases := []struct {
		effectValue int
		statusID    int
	}{
		{2, 2}, {3, 3}, {4, 4}, {5, 5}, {6, 6}, {6, 7},
	}
	for _, c := range cases {
		p := &BattlePokemon{Name: "Покемон", CurrentHP: 10, MaxHP: 10, StatusID: c.statusID, SleepTurns: 2, PoisonTurns: 3}
		item := testBattleItem(BattleItemCureStatus, c.effectValue)
		if ok, reason := battleItemUsable(item, p); !ok {
			t.Fatalf("статус %d: предмет неприменим: %s", c.statusID, reason)
		}
		applyBattleItem(item, p, 1)
		if p.StatusID != 1 || p.SleepTurns != 0 || p.PoisonTurns != 0 {
			t.Fatalf("статус %d не снят: status=%d sleep=%d poison=%d", c.statusID, p.StatusID, p.SleepTurns, p.PoisonTurns)
		}
	}
}

func TestBattleItemCureConfusion(t *testing.T) {
	p := &BattlePokemon{Name: "Покемон", CurrentHP: 10, MaxHP: 10, StatusID: 1, Confused: true, ConfusedTurns: 3}
	item := testBattleItem(BattleItemCureStatus, 13)
	if ok, _ := battleItemUsable(item, p); !ok {
		t.Fatal("Успокоительное должно применяться при замешательстве")
	}
	applyBattleItem(item, p, 1)
	if p.Confused || p.ConfusedTurns != 0 {
		t.Fatalf("замешательство не снято: %v %d", p.Confused, p.ConfusedTurns)
	}
}

func TestBattleItemWrongStatusNotUsable(t *testing.T) {
	p := &BattlePokemon{Name: "Покемон", CurrentHP: 10, MaxHP: 10, StatusID: 2}
	item := testBattleItem(BattleItemCureStatus, 5)
	if ok, _ := battleItemUsable(item, p); ok {
		t.Fatal("Пробуждение не должно применяться без сна")
	}
}

// Предмет тратит ход игрока, но не мешает врагу ответить атакой.
func TestProcessTurnWithItemLetsEnemyAct(t *testing.T) {
	bs, player, wild := gen2TestBattle()
	player.StatusID = 5
	player.SleepTurns = 2
	wild.Moves = []BattleMove{{
		ID: 1, Name: "Удар", TypeID: 1, CategoryID: 1,
		Power: 40, Accuracy: 100, PP: 10, MaxPP: 10,
	}}

	item := testBattleItem(BattleItemCureStatus, 5)
	applyBattleItem(item, player, bs.Turn+1)
	log := processTurnWithItem(bs)

	if player.StatusID != 1 || player.SleepTurns != 0 {
		t.Fatalf("сон не снят: status=%d sleep=%d", player.StatusID, player.SleepTurns)
	}
	if bs.Turn != 1 {
		t.Fatalf("раунд = %d, ожидался 1", bs.Turn)
	}
	if len(log) == 0 {
		t.Fatal("ожидался лог хода врага")
	}
	if player.CurrentHP >= player.MaxHP {
		t.Fatal("враг не ответил атакой")
	}
}

// Интеграционный тест: HTTP-хендлер списывает предмет, лечит активного и
// пропускает ход врага.
func TestUseBattleItemHandler(t *testing.T) {
	db.DB.Exec(`CREATE TABLE IF NOT EXISTS item_categories (id INTEGER PRIMARY KEY, name TEXT)`)
	db.DB.Exec(`CREATE TABLE IF NOT EXISTS items (
		id INTEGER PRIMARY KEY, name TEXT, description TEXT, category_id INTEGER,
		price INTEGER, effect_type TEXT, effect_value INTEGER, image_url TEXT,
		catch_multiplier INTEGER, catch_condition TEXT, catch_cond_value INTEGER,
		gene_bonus INTEGER, happiness_bonus INTEGER, force_nursery INTEGER,
		type_id INTEGER, attack_id INTEGER)`)
	db.DB.Exec(`CREATE TABLE IF NOT EXISTS user_items (
		user_id INTEGER, item_id INTEGER, quantity INTEGER,
		PRIMARY KEY (user_id, item_id))`)
	db.DB.Exec(`INSERT OR REPLACE INTO item_categories (id, name) VALUES (2, 'potion')`)
	db.DB.Exec(`INSERT OR REPLACE INTO items
		(id, name, description, category_id, price, effect_type, effect_value, image_url)
		VALUES (170, 'Стимулятор', 'тест', 2, 0, 'battle_heal_hp', 10, '')`)
	db.DB.Exec(`INSERT OR REPLACE INTO user_items (user_id, item_id, quantity) VALUES (777, 170, 1)`)

	t.Setenv("SESSION_KEY", "battle-items-test")
	session.Init()

	req0 := httptest.NewRequest("GET", "/", nil)
	rec0 := httptest.NewRecorder()
	sess, _ := session.Store.Get(req0, "pokemon-session")
	sess.Values["user_id"] = 777
	if err := sess.Save(req0, rec0); err != nil {
		t.Fatal(err)
	}
	cookies := rec0.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("сессионная cookie не создана")
	}

	bs, player, wild := gen2TestBattle()
	player.CurrentHP = 40
	wild.Moves = []BattleMove{{
		ID: 1, Name: "Удар", TypeID: 1, CategoryID: 1,
		Power: 40, Accuracy: 100, PP: 10, MaxPP: 10,
	}}
	battleID := "test-item-battle"
	battleMu.Lock()
	battleSessions[battleID] = bs
	battleMu.Unlock()
	t.Cleanup(func() {
		battleMu.Lock()
		delete(battleSessions, battleID)
		battleMu.Unlock()
	})

	body, _ := json.Marshal(useBattleItemRequest{BattleID: battleID, ItemID: 170})
	req := httptest.NewRequest("POST", "/api/battle/use-item", bytes.NewReader(body))
	req.AddCookie(cookies[0])
	rec := httptest.NewRecorder()
	UseBattleItemHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("HTTP %d: %s", rec.Code, rec.Body.String())
	}
	var resp BattleResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Type != "turn" {
		t.Fatalf("type = %q, ожидался turn", resp.Type)
	}
	if resp.Player == nil || resp.Player.CurrentHP >= 50 {
		t.Fatalf("лечение/ход врага не применились: %+v", resp.Player)
	}

	var qty int
	db.DB.QueryRow("SELECT COALESCE(quantity,0) FROM user_items WHERE user_id=777 AND item_id=170").Scan(&qty)
	if qty != 0 {
		t.Fatalf("предмет не списан: quantity=%d", qty)
	}
}
