package main

// ai_bench — офлайн-бенчмарк и автоподбор AI-команд NPC.
//
// Запускать из корня репозитория (БД открывается как ./database/pokemon.db):
//
//	go run ./cmd/ai_bench -mode bench -npc 1 -ai 2 -level 50 -seeds 100
//	go run ./cmd/ai_bench -mode auto  -npc 1 -ai 2 -level 50 -variants 30 -seeds 20
//
// Автоподбор пишет в npc_ai_teams (source=auto, top-5). На время массовой
// записи сервер лучше остановить, чтобы не ловить database is locked.

import (
	"flag"
	"fmt"
	"os"
	"sort"

	"pokemon-game/server/db"
	"pokemon-game/server/handlers/api"
)

func main() {
	mode := flag.String("mode", "bench", "bench | auto")
	npcID := flag.Int("npc", 1, "ID NPC")
	aiLevel := flag.Int("ai", 2, "AI-уровень (1/2)")
	pkmnLevel := flag.Int("level", 50, "уровень покемонов")
	size := flag.Int("size", 6, "размер команды 1..6")
	seeds := flag.Int("seeds", 100, "боёв на вариант")
	tactics := flag.Bool("tactics", false, "обе стороны получают одну команду (проверка тактики)")
	variants := flag.Int("variants", 20, "вариантов для auto")
	flag.Parse()

	db.InitDB()
	defer db.CloseDB()

	switch *mode {
	case "bench":
		rep, err := api.RunAIBenchmarkForNPC(*npcID, *aiLevel, *size, *pkmnLevel, *seeds, *tactics)
		if err != nil {
			fmt.Fprintln(os.Stderr, "ошибка:", err)
			os.Exit(1)
		}
		fmt.Printf("npc=%d ai=%d level=%d size=%d seeds=%d tactics=%v\n",
			rep.NPCID, rep.AILevel, rep.Level, rep.Size, rep.Seeds, rep.Tactics)
		fmt.Printf("bot wins=%d losses=%d timeouts=%d winrate=%.3f\n",
			rep.BotWins, rep.BotLosses, rep.Timeouts, rep.WinRate)
	case "auto":
		reps, err := api.RunAIAutoPick(*npcID, *aiLevel, *size, *pkmnLevel, *variants, *seeds)
		if err != nil {
			fmt.Fprintln(os.Stderr, "ошибка:", err)
			os.Exit(1)
		}
		sort.Slice(reps, func(i, j int) bool { return reps[i].WinRate > reps[j].WinRate })
		for i := range reps {
			fmt.Printf("variant %2d: bot wins=%d losses=%d timeouts=%d winrate=%.3f\n",
				i+1, reps[i].BotWins, reps[i].BotLosses, reps[i].Timeouts, reps[i].WinRate)
		}
		fmt.Println("лучшие auto-команды сохранены в npc_ai_teams (top-5)")
	default:
		fmt.Fprintln(os.Stderr, "неизвестный режим:", *mode)
		os.Exit(1)
	}
}
