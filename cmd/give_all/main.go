package main

import (
	"log"
	"os"
	"strconv"
	"pokemon-game/server/db"
	"pokemon-game/server/utils"
)

func main() {
	db.InitDB()
	defer db.CloseDB()

	uid := 3

	pokemonIDs := []int{}
	targetStr := ""

	if len(os.Args) > 1 {
		targetStr = os.Args[1]
	}

	if targetStr != "" {
		pid, err := strconv.Atoi(targetStr)
		if err != nil {
			log.Fatalf("Invalid pokemon ID: %s", targetStr)
		}
		pokemonIDs = append(pokemonIDs, pid)
	} else {
		rows, err := db.DB.Query("SELECT id FROM pokemons ORDER BY id")
		if err != nil {
			log.Fatalf("Failed to query pokemons: %v", err)
		}
		defer rows.Close()
		for rows.Next() {
			var pid int
			if err := rows.Scan(&pid); err != nil {
				log.Printf("Scan error: %v", err)
				continue
			}
			pokemonIDs = append(pokemonIDs, pid)
		}
	}

	log.Printf("Creating %d pokemon for user %d", len(pokemonIDs), uid)

	for _, pid := range pokemonIDs {
		gender, err := db.RollGender(pid)
		if err != nil {
			log.Printf("RollGender error for pokemon %d: %v", pid, err)
			gender = db.GenderNeutral
		}

		iv := [6]int{0, 0, 0, 0, 0, 0}
		natureID := 1

		_, err = utils.CreateUserPokemon(uid, pid, 1, iv, natureID, gender, 0, 0, false)
		if err != nil {
			log.Printf("Failed to create pokemon %d: %v", pid, err)
			continue
		}
	}

	log.Println("Done!")
}
