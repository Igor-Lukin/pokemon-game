package db

import (
	"log"
	"strconv"
	"strings"
)

// legendaryIDs — легендарные и мифические виды (PokeAPI: is_legendary ||
// is_mythical), поколения I–IX. Список зашит намеренно: в БД нет отдельного
// справочника, а признак нужен драфту AI-бота (0/1/4/6 легенд по уровню).
// При добавлении новых поколений список пополняется вместе с импортом видов.
var legendaryIDs = []int{
	// I
	144, 145, 146, 150, 151,
	// II
	243, 244, 245, 249, 250, 251,
	// III
	377, 378, 379, 380, 381, 382, 383, 384, 385, 386,
	// IV
	480, 481, 482, 483, 484, 485, 486, 487, 488, 489, 490, 491, 492, 493,
	// V
	494, 638, 639, 640, 641, 642, 643, 644, 645, 646, 647, 648, 649,
	// VI
	716, 717, 718, 719, 720, 721,
	// VII (включая Ultra Beasts)
	772, 773, 785, 786, 787, 788, 789, 790, 791, 792, 793, 794, 795, 796,
	797, 798, 799, 800, 801, 802, 803, 804, 805, 806, 807, 808, 809,
	// VIII
	888, 889, 890, 891, 892, 893, 894, 895, 896, 897, 898, 905,
	// IX
	1001, 1002, 1003, 1004, 1007, 1008, 1014, 1015, 1016, 1017, 1024, 1025,
}

// BackfillLegendaryFlags идемпотентно проставляет pokemons.is_legendary.
// Сначала сбрасываем все флаги, затем помечаем список: так правки списка
// подхватываются при следующем старте без отдельных миграций.
func BackfillLegendaryFlags() {
	if DB == nil || len(legendaryIDs) == 0 {
		return
	}
	if _, err := DB.Exec("UPDATE pokemons SET is_legendary = 0"); err != nil {
		log.Printf("BackfillLegendaryFlags: reset: %v", err)
		return
	}
	var b strings.Builder
	b.WriteString("UPDATE pokemons SET is_legendary = 1 WHERE id IN (")
	for i, id := range legendaryIDs {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.Itoa(id))
	}
	b.WriteByte(')')
	if _, err := DB.Exec(b.String()); err != nil {
		log.Printf("BackfillLegendaryFlags: update: %v", err)
	}
}
