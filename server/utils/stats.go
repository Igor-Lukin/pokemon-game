package utils

import "pokemon-game/server/db"

// CalculateStats рассчитывает статы покемона на основе параметров.
// Порядок индексов: 0-HP, 1-Attack, 2-Defense, 3-Speed, 4-SpecialAttack, 5-SpecialDefense
func CalculateStats(
	base [6]int,
	iv [6]int,
	ev [6]int,
	level int,
	nature [6]float64,
	train [6]float64,
) (currentHP, maxHP, attack, defense, speed, specialAttack, specialDefense int) {
	// Вспомогательная функция для расчёта не-HP стата
	calcStat := func(base, iv, ev int, nature, train float64) int {
		// ( (base*2 + iv + ev/2) * level / 100 + 5 ) * nature * train
		mid := (base*2 + iv + ev/2) * level / 100
		mid += 5
		statFloat := float64(mid) * nature * train
		return int(statFloat)
	}

	// HP
	hpMid := (base[0]*2 + iv[0] + ev[0]/2) * level / 100
	maxHP = hpMid + 10 + level
	currentHP = maxHP

	// Остальные статы
	attack = calcStat(base[1], iv[1], ev[1], nature[1], train[1])
	defense = calcStat(base[2], iv[2], ev[2], nature[2], train[2])
	speed = calcStat(base[3], iv[3], ev[3], nature[3], train[3])
	specialAttack = calcStat(base[4], iv[4], ev[4], nature[4], train[4])
	specialDefense = calcStat(base[5], iv[5], ev[5], nature[5], train[5])
	return
}

// GetNatureFactors возвращает массив множителей для 6 статов на основе natureID
func GetNatureFactors(natureID int) ([6]float64, error) {
	var inc, dec string
	err := db.DB.QueryRow("SELECT increased_stat, decreased_stat FROM natures WHERE id = ?", natureID).Scan(&inc, &dec)
	if err != nil {
		return [6]float64{1, 1, 1, 1, 1, 1}, err
	}
	factors := [6]float64{1.0, 1.0, 1.0, 1.0, 1.0, 1.0}
	statIdx := map[string]int{
		"hp":              0,
		"attack":          1,
		"defense":         2,
		"speed":           3,
		"special_attack":  4,
		"special_defense": 5,
	}
	if idx, ok := statIdx[inc]; ok {
		factors[idx] = 1.1
	}
	if idx, ok := statIdx[dec]; ok {
		factors[idx] = 0.9
	}
	return factors, nil
}
