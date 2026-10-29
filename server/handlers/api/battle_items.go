package api

import (
	"encoding/json"
	"net/http"

	"pokemon-game/server/db"
	"pokemon-game/server/session"
)

// Боевые расходуемые предметы (не held): лечение HP и снятие статусов.
const (
	BattleItemHealHP      = "battle_heal_hp"
	BattleItemFullRestore = "battle_full_restore"
	BattleItemCureStatus  = "battle_cure_status"
)

// IsBattleItem — предмет можно применить в бою через /api/battle/use-item.
func IsBattleItem(it *db.Item) bool {
	if it == nil {
		return false
	}
	switch it.EffectType {
	case BattleItemHealHP, BattleItemFullRestore, BattleItemCureStatus:
		return true
	}
	return false
}

// battleItemUsable проверяет, даст ли предмет эффект прямо сейчас.
func battleItemUsable(item *db.Item, p *BattlePokemon) (bool, string) {
	if item == nil || p == nil {
		return false, "Предмет недоступен"
	}
	switch item.EffectType {
	case BattleItemHealHP:
		if item.EffectValue <= 0 {
			return false, "Предмет не лечит"
		}
		if p.CurrentHP >= p.MaxHP {
			return false, p.Name + " уже имеет полное HP"
		}
		return true, ""
	case BattleItemFullRestore:
		if p.CurrentHP < p.MaxHP {
			return true, ""
		}
		for i := 1; i < len(p.StatStages); i++ {
			if p.StatStages[i] > 2 {
				return true, ""
			}
		}
		return false, "Нечего восстанавливать"
	case BattleItemCureStatus:
		if item.EffectValue == 13 {
			if !p.Confused {
				return false, p.Name + " не находится в замешательстве"
			}
			return true, ""
		}
		if item.EffectValue == 6 {
			if p.StatusID != 6 && p.StatusID != 7 {
				return false, p.Name + " не отравлен"
			}
			return true, ""
		}
		if p.StatusID != item.EffectValue {
			return false, "У " + p.Name + " нет такого статуса"
		}
		return true, ""
	}
	return false, "Этот предмет нельзя использовать в бою"
}

// applyBattleItem применяет эффект предмета к активному покемону и возвращает
// строки боевого лога. Вызывать после battleItemUsable.
func applyBattleItem(item *db.Item, p *BattlePokemon, round int) []BattleLogEntry {
	log := []BattleLogEntry{}
	if item == nil || p == nil {
		return log
	}

	switch item.EffectType {
	case BattleItemHealHP:
		healed := item.EffectValue
		if healed > p.MaxHP-p.CurrentHP {
			healed = p.MaxHP - p.CurrentHP
		}
		if healed <= 0 {
			return log
		}
		p.CurrentHP += healed
		log = append(log, BattleLogEntry{
			Round:        round,
			PokemonName:  p.Name,
			PokemonImage: p.ImageURL,
			Message:      p.Name + " использует «" + item.Name + "»!",
		})
		log = append(log, BattleLogEntry{Round: round, PokemonName: p.Name, Heal: healed})

	case BattleItemFullRestore:
		healed := p.MaxHP - p.CurrentHP
		capped := false
		for i := 1; i < len(p.StatStages); i++ {
			if p.StatStages[i] > 2 {
				p.StatStages[i] = 2
				capped = true
			}
		}
		p.CurrentHP = p.MaxHP
		msg := p.Name + " использует «" + item.Name + "» — HP полностью восстановлено!"
		if capped {
			msg += " Характеристики выше +2 снижены до +2."
		}
		log = append(log, BattleLogEntry{
			Round:        round,
			PokemonName:  p.Name,
			PokemonImage: p.ImageURL,
			Message:      msg,
		})
		if healed > 0 {
			log = append(log, BattleLogEntry{Round: round, PokemonName: p.Name, Heal: healed})
		}

	case BattleItemCureStatus:
		msg := ""
		switch item.EffectValue {
		case 13:
			if !p.Confused {
				return log
			}
			p.Confused = false
			p.ConfusedTurns = 0
			msg = p.Name + " приходит в себя после «" + item.Name + "»!"
		case 6:
			if p.StatusID != 6 && p.StatusID != 7 {
				return log
			}
			clearMajorStatus(p)
			msg = p.Name + " излечивается от отравления с помощью «" + item.Name + "»!"
		case 2:
			clearMajorStatus(p)
			msg = p.Name + " излечивается от паралича с помощью «" + item.Name + "»!"
		case 3:
			clearMajorStatus(p)
			msg = p.Name + " излечивается от ожога с помощью «" + item.Name + "»!"
		case 4:
			clearMajorStatus(p)
			msg = p.Name + " излечивается от заморозки с помощью «" + item.Name + "»!"
		case 5:
			clearMajorStatus(p)
			msg = p.Name + " просыпается с помощью «" + item.Name + "»!"
		}
		if msg != "" {
			log = append(log, BattleLogEntry{
				Round:        round,
				PokemonName:  p.Name,
				PokemonImage: p.ImageURL,
				Message:      msg,
			})
		}
	}
	return log
}

type useBattleItemRequest struct {
	BattleID string `json:"battle_id"`
	ItemID   int    `json:"item_id"`
}

// UseBattleItemHandler — применение расходуемого предмета в бою (wild/trainer).
// Предмет тратится, игрок теряет ход, затем отвечает враг. PvP идёт через
// /api/pvp/action, чтобы ход разрешался синхронно у обоих игроков.
func UseBattleItemHandler(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.Store.Get(r, "pokemon-session")
	userID, ok := sess.Values["user_id"].(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req useBattleItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	battleMu.Lock()
	bs, exists := battleSessions[req.BattleID]
	battleMu.Unlock()
	if !exists {
		http.Error(w, "Battle not found", http.StatusNotFound)
		return
	}
	if bs.Finished {
		http.Error(w, "Battle already finished", http.StatusBadRequest)
		return
	}
	if bs.BattleType == "pvp" {
		http.Error(w, "В PvP предметы применяются через действие боя", http.StatusBadRequest)
		return
	}

	player := &bs.PlayerTeam[bs.ActiveIdx]
	if player.IsFainted {
		http.Error(w, "Активный покемон без сознания", http.StatusBadRequest)
		return
	}

	item, err := db.GetItemByID(req.ItemID)
	if err != nil {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}
	if !IsBattleItem(item) {
		http.Error(w, "Этот предмет нельзя использовать в бою", http.StatusBadRequest)
		return
	}
	if usable, reason := battleItemUsable(item, player); !usable {
		http.Error(w, reason, http.StatusBadRequest)
		return
	}

	removed, err := db.RemoveItemFromUser(userID, item.ID, 1)
	if err != nil || !removed {
		http.Error(w, "У вас нет этого предмета", http.StatusConflict)
		return
	}

	itemLog := applyBattleItem(item, player, bs.Turn+1)
	prevEnemyIdx := bs.EnemyActiveIdx
	enemyLog := processTurnWithItem(bs)
	enemySwitchedByAI := bs.EnemyActiveIdx != prevEnemyIdx
	log := append(itemLog, enemyLog...)

	resp := finishBattleTurn(bs, req.BattleID, log, enemySwitchedByAI)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
