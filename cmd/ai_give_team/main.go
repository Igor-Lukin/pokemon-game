package main

// Временная утилита для живого теста: выдать ADMIN (user_id=2) трёх
// покемонов 100 уровня, чтобы получился бой 6x6. Использует единую точку
// создания utils.CreateUserPokemon.

import (
	"fmt"
	"log"

	"pokemon-game/server/db"
	"pokemon-game/server/utils"
)

func main() {
	db.InitDB()
	defer db.CloseDB()

	// Garchomp, Metagross, Dragapult — сильные финальные эволюции.
	for _, pid := range []int{445, 376, 887} {
		gender, err := db.RollGender(pid)
		if err != nil {
			gender = db.GenderNeutral
		}
		iv := [6]int{31, 31, 31, 31, 31, 31}
		id, err := utils.CreateUserPokemon(2, pid, 100, iv, 1, gender, 0, 0, false)
		if err != nil {
			log.Printf("give #%d: %v", pid, err)
			continue
		}
		fmt.Printf("created user_pokemon id=%d species=%d\n", id, pid)
	}
}
