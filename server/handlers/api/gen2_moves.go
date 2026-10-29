package api

import "pokemon-game/server/db"

const (
	moveRage         = 567
	moveSacredFire   = 568
	moveSelfDestruct = 570
	moveThief        = 574
	moveTripleKick   = 576
	moveBind         = 552
	moveWrap         = 577
	moveTrapStatus   = 11
	moveFrozen       = 4
	moveHealthy      = 1
)

// isSelfDestructMove — атаки, после которых пользователь падает в обморок
// (Самоподрыв/Взрыв/Туманный взрыв). Используется Сыростью и обработкой фейнта.
func isSelfDestructMove(id int) bool {
	return id == moveSelfDestruct || id == 100 || id == 707
}

func tripleKickPower(base, hit int) int {
	return base * (hit + 1)
}

func applyRageOnHit(defender *BattlePokemon, round int, log *[]BattleLogEntry) {
	if defender == nil || !defender.Raging || defender.IsFainted {
		return
	}
	curseBump(defender, 1, 1, round, log)
}

func applyThief(attacker, defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) {
	if move == nil || move.ID != moveThief || attacker == nil || defender == nil {
		return
	}
	if attacker.HeldItemID != 0 || defender.HeldItemID == 0 {
		return
	}
	if defender.AbilityKey == "sticky_hold" {
		abilityLog(defender, round, log, "", "удерживает предмет — украсть нельзя")
		return
	}

	itemID := defender.HeldItemID
	itemName := defender.HeldItemName
	item, itemErr := db.GetItemByID(itemID)
	if itemErr == nil && item != nil {
		itemName = item.Name
	}

	defender.HeldItemID = 0
	defender.HeldItemTypeID = 0
	defender.HeldItemEffect = ""
	defender.HeldAtkMult = 1.0
	defender.HeldItemName = ""
	defender.HeldItemImage = ""
	defender.HeldItemDescription = ""

	attacker.HeldItemID = itemID
	attacker.HeldItemName = itemName
	attacker.HeldItemTypeID = 0
	attacker.HeldItemEffect = ""
	attacker.HeldAtkMult = 1.0
	attacker.HeldItemImage = ""
	attacker.HeldItemDescription = ""

	if itemErr == nil && item != nil {
		attacker.HeldItemImage = item.ImageURL
		attacker.HeldItemDescription = item.Description
		attacker.HeldItemEffect = item.EffectType
		if item.EffectType == "type_boost" && item.EffectValue > 0 {
			attacker.HeldItemTypeID = item.TypeID
			attacker.HeldAtkMult = float64(item.EffectValue) / 100.0
		}
	}

	*log = append(*log, BattleLogEntry{
		Round:        round,
		PokemonName:  attacker.Name,
		PokemonImage: attacker.ImageURL,
		Message:      attacker.Name + " крадёт " + itemName + " у " + defender.Name + "!",
	})
}

func selfDestructFaint(attacker *BattlePokemon, round int, log *[]BattleLogEntry) {
	if attacker == nil || attacker.IsFainted {
		return
	}
	attacker.CurrentHP = 0
	attacker.IsFainted = true
	*log = append(*log, BattleLogEntry{
		Round:        round,
		PokemonName:  attacker.Name,
		PokemonImage: attacker.ImageURL,
		Message:      attacker.Name + " взрывается и падает в обморок!",
	})
	*log = append(*log, BattleLogEntry{
		Round:       round,
		PokemonName: attacker.Name,
		Fainted:     true,
	})
}

func thawBySacredFire(p *BattlePokemon, round int, log *[]BattleLogEntry) {
	if p == nil || p.StatusID != moveFrozen {
		return
	}
	p.StatusID = moveHealthy
	*log = append(*log, BattleLogEntry{
		Round:        round,
		PokemonName:  p.Name,
		PokemonImage: p.ImageURL,
		Message:      p.Name + " оттаивает от Священного огня!",
	})
}

func sacredFireCanAct(p *BattlePokemon, move *BattleMove, canAct bool, round int, log *[]BattleLogEntry) bool {
	if canAct || p == nil || move == nil || move.ID != moveSacredFire || p.StatusID != moveFrozen {
		return canAct
	}
	thawBySacredFire(p, round, log)
	return true
}
