package api

import "testing"

func TestHeldItemBurnHolder(t *testing.T) {
	p := newTestPokemon("Пикачу", 100)
	p.StatusID = 1
	p.HeldItemEffect = heldEffectBurnHolder
	p.HeldItemName = "Зажигалка"

	var log []BattleLogEntry
	bs := &BattleState{}
	heldItemOnEndTurn(bs, p, 1, &log)

	if p.StatusID != statusBurn {
		t.Fatalf("Зажигалка не наложила ожог: status=%d", p.StatusID)
	}
	if !hasMsg(log, "Зажигалка поджигает Пикачу!") {
		t.Fatalf("нет сообщения о поджоге: %+v", log)
	}

	before := len(log)
	heldItemOnEndTurn(bs, p, 2, &log)
	if len(log) != before {
		t.Fatalf("повторный поджог уже обожжённого: %+v", log[before:])
	}
}

func TestHeldItemBurnHolderSkipsFainted(t *testing.T) {
	p := newTestPokemon("Пикачу", 100)
	p.StatusID = 1
	p.IsFainted = true
	p.HeldItemEffect = heldEffectBurnHolder

	var log []BattleLogEntry
	heldItemOnEndTurn(&BattleState{}, p, 1, &log)

	if p.StatusID != 1 {
		t.Fatalf("павший покемон получил статус: %d", p.StatusID)
	}
	if len(log) != 0 {
		t.Fatalf("лишние записи лога: %+v", log)
	}
}
