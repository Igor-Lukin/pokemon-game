package routes

import (
	"net/http"
	"pokemon-game/server/db"
	"pokemon-game/server/handlers"
	"pokemon-game/server/handlers/admin"
	"pokemon-game/server/handlers/api"
	"pokemon-game/server/handlers/auth"
	"pokemon-game/server/handlers/game"
	"pokemon-game/server/session"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
)

// Троттлинг обновления users.last_seen: не чаще раза в 60с на игрока.
var (
	lastSeenMu    sync.Mutex
	lastSeenCache = map[int]time.Time{}
)

// lastSeenMiddleware обновляет users.last_seen для авторизованного игрока
// (онлайн/оффлайн в карточке тренера). Пишет в БД не чаще раза в 60с на игрока.
func lastSeenMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if sess, err := session.Store.Get(r, "pokemon-session"); err == nil {
			if uid, ok := sess.Values["user_id"].(int); ok {
				now := time.Now()
				lastSeenMu.Lock()
				last, seen := lastSeenCache[uid]
				stale := !seen || now.Sub(last) > 60*time.Second
				if stale {
					lastSeenCache[uid] = now
				}
				lastSeenMu.Unlock()
				if stale {
					db.DB.Exec("UPDATE users SET last_seen = CURRENT_TIMESTAMP WHERE id = ?", uid)
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

// noCacheAPIMiddleware запрещает кэширование ответов /api (статика не
// затрагивается и продолжает кэшироваться).
func noCacheAPIMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func SetupRoutes() *mux.Router {
	r := mux.NewRouter()
	database := db.DB

	// Auth handlers
	auth.InitHandlers(database)

	// Анти-брутфорс: 10 попыток входа с IP за 10 минут, 5 регистраций в час.
	loginLimiter := newRateLimiter(10, 10*time.Minute)
	registerLimiter := newRateLimiter(5, time.Hour)

	// Routes
	r.HandleFunc("/", handlers.HomeHandler).Methods("GET")
	r.HandleFunc("/register", auth.RegisterHandler).Methods("GET")
	r.Handle("/register", registerLimiter.middleware(http.HandlerFunc(auth.RegisterPostHandler))).Methods("POST")
	r.HandleFunc("/login", auth.LoginHandler).Methods("GET")
	r.Handle("/login", loginLimiter.middleware(http.HandlerFunc(auth.LoginPostHandler))).Methods("POST")
	r.HandleFunc("/game", game.GameHandler).Methods("GET")
	r.HandleFunc("/game", game.GamePostHandler).Methods("POST")

	// Admin routes++
	r.HandleFunc("/admin", admin.IndexHandler).Methods("GET")
	// regions
	r.HandleFunc("/admin/regions", admin.RegionsHandler).Methods("GET")
	r.HandleFunc("/admin/regions/create", admin.CreateRegionHandler).Methods("GET", "POST")
	r.HandleFunc("/admin/regions/edit", admin.EditRegionHandler).Methods("GET", "POST")
	r.HandleFunc("/admin/regions/delete", admin.DeleteRegionHandler).Methods("POST")
	// locations
	r.HandleFunc("/admin/locations", admin.LocationsHandler).Methods("GET")
	r.HandleFunc("/admin/locations/create", admin.CreateLocationHandler).Methods("GET", "POST")
	r.HandleFunc("/admin/locations/edit", admin.EditLocationHandler).Methods("GET", "POST")
	r.HandleFunc("/admin/locations/delete", admin.DeleteLocationHandler).Methods("POST")
	r.HandleFunc("/admin/locations/exit/add", admin.AdminAddExitHandler).Methods("POST")
	r.HandleFunc("/admin/locations/exit/remove", admin.AdminRemoveExitHandler).Methods("POST")
	// npc
	r.HandleFunc("/admin/npc", admin.NonPlayerCharacterHandler).Methods("GET")
	r.HandleFunc("/admin/npc/create", admin.CreateNonPlayerCharacterHandler).Methods("GET", "POST")
	r.HandleFunc("/admin/npc/edit", admin.EditNonPlayerCharacterHandler).Methods("GET", "POST")
	r.HandleFunc("/admin/npc/delete", admin.DeleteNonPlayerCharacterHandler).Methods("POST")
	// npc — редактор диалогов (узлы/опции)
	r.HandleFunc("/admin/npc/dialogue/node/add", admin.AddDialogueNodeHandler).Methods("POST")
	r.HandleFunc("/admin/npc/dialogue/node/edit", admin.EditDialogueNodeHandler).Methods("POST")
	r.HandleFunc("/admin/npc/dialogue/node/delete", admin.DeleteDialogueNodeHandler).Methods("POST")
	r.HandleFunc("/admin/npc/dialogue/option/add", admin.AddDialogueOptionHandler).Methods("POST")
	r.HandleFunc("/admin/npc/dialogue/option/edit", admin.EditDialogueOptionHandler).Methods("POST")
	r.HandleFunc("/admin/npc/dialogue/option/delete", admin.DeleteDialogueOptionHandler).Methods("POST")
	// npc — фикс-ростер команды (поток B)
	r.HandleFunc("/admin/npc/team/add", admin.AddNPCPokemonHandler).Methods("POST")
	r.HandleFunc("/admin/npc/team/edit", admin.EditNPCPokemonHandler).Methods("POST")
	r.HandleFunc("/admin/npc/team/delete", admin.DeleteNPCPokemonHandler).Methods("POST")
	// npc — AI-команда (генерация/предпросмотр/пул)
	r.HandleFunc("/admin/npc/ai-team/generate", admin.GenerateNPCAITeamHandler).Methods("POST")
	r.HandleFunc("/admin/npc/ai-team/save", admin.SaveNPCAITeamHandler).Methods("POST")
	r.HandleFunc("/admin/npc/ai-team/delete", admin.DeleteNPCAITeamHandler).Methods("POST")
	// abilities
	r.HandleFunc("/admin/abilities", admin.AbilitiesHandler).Methods("GET")
	r.HandleFunc("/admin/abilities/create", admin.CreateAbilityHandler).Methods("GET", "POST")
	r.HandleFunc("/admin/abilities/edit", admin.EditAbilityHandler).Methods("GET", "POST")
	r.HandleFunc("/admin/abilities/delete", admin.DeleteAbilityHandler).Methods("POST")
	// attacks
	r.HandleFunc("/admin/attacks", admin.AttacksHandler).Methods("GET")
	r.HandleFunc("/admin/attacks/create", admin.CreateAttackHandler).Methods("GET", "POST")
	r.HandleFunc("/admin/attacks/edit", admin.EditAttackHandler).Methods("GET", "POST")
	r.HandleFunc("/admin/attacks/delete", admin.DeleteAttackHandler).Methods("POST")
	// pokemons
	r.HandleFunc("/admin/pokemons", admin.PokemonsHandler).Methods("GET")
	r.HandleFunc("/admin/pokemons/create", admin.CreatePokemonHandler).Methods("GET", "POST")
	r.HandleFunc("/admin/pokemons/edit", admin.EditPokemonHandler).Methods("GET", "POST")
	r.HandleFunc("/admin/pokemons/delete", admin.DeletePokemonHandler).Methods("POST")
	r.HandleFunc("/admin/pokemons/ability/add", admin.AddPokemonAbilityHandler).Methods("POST")
	r.HandleFunc("/admin/pokemons/ability/remove", admin.RemovePokemonAbilityHandler).Methods("POST")
	r.HandleFunc("/admin/pokemons/location/add", admin.AddPokemonLocationHandler).Methods("POST")
	r.HandleFunc("/admin/pokemons/location/edit", admin.EditPokemonLocationHandler).Methods("POST")
	r.HandleFunc("/admin/pokemons/location/remove", admin.RemovePokemonLocationHandler).Methods("POST")
	r.HandleFunc("/admin/pokemons/attack/add", admin.AddPokemonAttackHandler).Methods("POST")
	r.HandleFunc("/admin/pokemons/attack/remove", admin.RemovePokemonAttackHandler).Methods("POST")

	//выдача покемонов
	r.HandleFunc("/admin/give_pokemon", admin.GivePokemonHandler).Methods("GET", "POST")
	// items
	r.HandleFunc("/admin/items", admin.ItemsListHandler).Methods("GET")
	r.HandleFunc("/admin/items/create", admin.CreateItemHandler).Methods("GET", "POST")
	r.HandleFunc("/admin/items/edit", admin.EditItemHandler).Methods("GET", "POST")
	r.HandleFunc("/admin/items/delete", admin.DeleteItemHandler).Methods("POST")
	// item categories
	r.HandleFunc("/admin/items/categories", admin.ItemsCategoriesHandler).Methods("GET")
	r.HandleFunc("/admin/items/categories/create", admin.CreateItemCategoryHandler).Methods("GET", "POST")
	r.HandleFunc("/admin/items/categories/edit", admin.EditItemCategoryHandler).Methods("GET", "POST")
	r.HandleFunc("/admin/items/categories/delete", admin.DeleteItemCategoryHandler).Methods("POST")
	// выдача предметов
	r.HandleFunc("/admin/give_item", admin.GiveItemHandler).Methods("GET", "POST")
	// Admin routes--

	// API для погоды
	r.HandleFunc("/api/weather/current", api.GetCurrentWeatherHandler).Methods("GET")

	// Бой
	r.HandleFunc("/api/battle/start", api.StartBattleHandler).Methods("POST")
	r.HandleFunc("/api/battle/move", api.MoveHandler).Methods("POST")
	r.HandleFunc("/api/battle/switch", api.SwitchHandler).Methods("POST")
	r.HandleFunc("/api/battle/run", api.RunHandler).Methods("POST")
	r.HandleFunc("/api/battle/catch", api.CatchHandler).Methods("POST")
	r.HandleFunc("/api/battle/use-item", api.UseBattleItemHandler).Methods("POST")
	// Тренерский бой vs команда NPC (поток B)
	r.HandleFunc("/api/battle/trainer/start", api.TrainerBattleStartHandler).Methods("POST")

	// Диалоги NPC (поток A)
	r.HandleFunc("/api/npc/{id}/dialogue", api.NpcDialogueStartHandler).Methods("GET")
	r.HandleFunc("/api/npc/dialogue/option", api.NpcDialogueOptionHandler).Methods("POST")

	// Pokedex
	r.HandleFunc("/api/pokedex/pokemon/{id}", api.GetPokemonInfo).Methods("GET")
	r.HandleFunc("/api/pokedex/pokemon/{id}/full", api.GetPokemonFullInfo).Methods("GET")
	r.HandleFunc("/api/pokedex/search", api.SearchPokemons).Methods("GET")
	r.HandleFunc("/api/pokedex/type/{id}", api.GetPokemonsByType).Methods("GET")
	r.HandleFunc("/api/pokedex/list", api.GetAllPokemons).Methods("GET")

	r.HandleFunc("/api/types", api.GetAllTypes).Methods("GET")

	// Отображение команды покемонов
	r.HandleFunc("/api/user/team", api.GetUserTeam).Methods("GET")
	r.HandleFunc("/api/user/pokemon/{id}/attacks", api.GetUserPokemonAttacks).Methods("GET")
	r.HandleFunc("/api/user/pokemon/{id}/learnable", api.GetLearnableAttacksHandler).Methods("GET")
	r.HandleFunc("/api/user/pokemon/{id}/learn", api.LearnAttackHandler).Methods("POST")
	r.HandleFunc("/api/user/pokemon/{id}/use-tm", api.UseTMHandler).Methods("POST")
	r.HandleFunc("/api/tm/{item_id}/targets", api.GetTMTargetsHandler).Methods("GET")
	r.HandleFunc("/api/user/pokemon/{id}/evolution", api.GetEvolutionHandler).Methods("GET")
	r.HandleFunc("/api/user/pokemon/{id}/evolve", api.EvolveHandler).Methods("POST")
	r.HandleFunc("/api/evolution-items/{item_id}/targets", api.GetEvolutionItemTargetsHandler).Methods("GET")
	r.HandleFunc("/api/user/pokemon/{id}/ev", api.DistributeEVHandler).Methods("POST")
	r.HandleFunc("/api/user/pokemon/{id}/starter", api.SetStarterHandler).Methods("POST")
	r.HandleFunc("/api/user/pokemon/{id}/hold", api.HoldItemHandler).Methods("POST")
	r.HandleFunc("/api/user/pokemon/{id}/unhold", api.UnholdItemHandler).Methods("POST")

	// Онлайн-игроки на локации + вызов на бой (поток [17], часть A)
	r.HandleFunc("/api/location/players", api.LocationPlayersHandler).Methods("GET")
	r.HandleFunc("/api/challenge", api.ChallengeCreateHandler).Methods("POST")
	r.HandleFunc("/api/challenge/incoming", api.ChallengeIncomingHandler).Methods("GET")
	r.HandleFunc("/api/challenge/respond", api.ChallengeRespondHandler).Methods("POST")
	r.HandleFunc("/api/challenge/outgoing", api.ChallengeOutgoingHandler).Methods("GET")

	// PvP-бой между игроками (поток [17], часть B)
	r.HandleFunc("/api/pvp/current", api.PvpCurrentHandler).Methods("GET")
	r.HandleFunc("/api/pvp/state", api.PvpStateHandler).Methods("GET")
	r.HandleFunc("/api/pvp/lead", api.PvpLeadHandler).Methods("POST")
	r.HandleFunc("/api/pvp/action", api.PvpActionHandler).Methods("POST")
	r.HandleFunc("/api/pvp/replace", api.PvpReplaceHandler).Methods("POST")
	r.HandleFunc("/api/pvp/forfeit", api.PvpForfeitHandler).Methods("POST")

	// API для локаций
	r.HandleFunc("/api/locations/{id}", api.GetLocation).Methods("GET")

	// API для пользователя
	r.HandleFunc("/api/user/location", api.GetCurrentUserLocation).Methods("GET")
	r.HandleFunc("/api/user/move", api.MoveUser).Methods("POST")

	// Карточка тренера (профиль). status — до {id}, чтобы не перехватывался.
	r.HandleFunc("/api/profile", api.ProfileHandler).Methods("GET")
	r.HandleFunc("/api/profile/status", api.SetStatusHandler).Methods("POST")
	r.HandleFunc("/api/profile/{id}", api.ProfileHandler).Methods("GET")

	// API для питомника
	r.HandleFunc("/api/nursery/pokemons", api.GetUserPokemonsForNursery).Methods("GET")
	r.HandleFunc("/api/nursery/take", api.TakeToTeamHandler).Methods("POST")
	r.HandleFunc("/api/nursery/send", api.SendToNurseryHandler).Methods("POST")
	// Разведение (поток [13])
	r.HandleFunc("/api/nursery/breed", api.BreedHandler).Methods("POST")
	r.HandleFunc("/api/nursery/eggs", api.EggsHandler).Methods("GET")
	r.HandleFunc("/api/nursery/hatch", api.HatchHandler).Methods("POST")

	// API для лечения
	r.HandleFunc("/api/heal", api.HealHandler).Methods("POST")

	// API для инвентаря
	r.HandleFunc("/api/inventory", api.GetInventoryHandler).Methods("GET")
	r.HandleFunc("/api/inventory/use", api.UseItemHandler).Methods("POST")

	// Static files
	r.PathPrefix("/static/").Handler(
		http.StripPrefix("/static/", http.FileServer(http.Dir("./client/static"))),
	)

	r.PathPrefix("/images/").Handler(
		http.StripPrefix("/images/", http.FileServer(http.Dir("./client/static/images"))),
	)

	// Отметка активности игрока (last_seen) на всех маршрутах.
	r.Use(lastSeenMiddleware)
	r.Use(noCacheAPIMiddleware)

	return r
}
