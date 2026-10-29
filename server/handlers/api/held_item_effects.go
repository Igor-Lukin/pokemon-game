package api

// Эффекты надетых (held) предметов в бою. Пока реализован один предмет —
// Зажигалка (items.effect_type = 'burn_holder'), но точка входа общая: сюда
// добавляются новые end-of-turn эффекты предметов, а не в каждый обработчик.

const (
	heldEffectBurnHolder = "burn_holder"
	statusBurn           = 3 // id статуса «Ожог» (таблица statuses)
)

// heldItemOnEndTurn — конец хода для носителя предмета. Вызывается для обоих
// активных рядом с abilityOnEndTurn в одиночном бою, PvP, при смене/ловле и в
// AI-симуляции. Павших пропускает.
func heldItemOnEndTurn(bs *BattleState, p *BattlePokemon, round int, log *[]BattleLogEntry) {
	if p == nil || p.IsFainted || p.AbilityKey == "klutz" {
		return
	}
	switch p.HeldItemEffect {
	case heldEffectBurnHolder:
		// Зажигалка поджигает носителя: основной статус «ожог» (урон 1/16 и
		// снижение физической атаки). Если статус уже есть — не перебиваем его.
		if p.StatusID > 1 {
			return
		}
		// [ability hook] Способности-иммунитеты к статусам действуют и здесь.
		if abilityBlocksStatus(bs, p, statusBurn) {
			return
		}
		if inflictMajorStatus(p, statusBurn, round, log) {
			itemName := p.HeldItemName
			if itemName == "" {
				itemName = "Зажигалка"
			}
			*log = append(*log, BattleLogEntry{
				Round:        round,
				PokemonName:  p.Name,
				PokemonImage: p.ImageURL,
				Message:      itemName + " поджигает " + p.Name + "!",
			})
		}
	}
}
