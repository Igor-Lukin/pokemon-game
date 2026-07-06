package routes

import (
	"net/http"
	"pokemon-game/server/db"
	"pokemon-game/server/handlers"
	"pokemon-game/server/handlers/admin"
	"pokemon-game/server/handlers/api"
	"pokemon-game/server/handlers/auth"
	"pokemon-game/server/handlers/game"

	"github.com/gorilla/mux"
)

func SetupRoutes() *mux.Router {
	r := mux.NewRouter()
	database := db.DB

	// Auth handlers
	auth.InitHandlers(database)

	// Routes
	r.HandleFunc("/", handlers.HomeHandler).Methods("GET")
	r.HandleFunc("/register", auth.RegisterHandler).Methods("GET")
	r.HandleFunc("/register", auth.RegisterPostHandler).Methods("POST")
	r.HandleFunc("/login", auth.LoginHandler).Methods("GET")
	r.HandleFunc("/login", auth.LoginPostHandler).Methods("POST")
	r.HandleFunc("/game", game.GameHandler).Methods("GET")
	r.HandleFunc("/game", game.GamePostHandler).Methods("POST")

	// Admin routes++
	r.HandleFunc("/admin", admin.IndexHandler).Methods("GET")
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

	// Pokedex
	r.HandleFunc("/api/pokedex/pokemon/{id}", api.GetPokemonInfo).Methods("GET")
	r.HandleFunc("/api/pokedex/pokemon/{id}/full", api.GetPokemonFullInfo).Methods("GET")
	r.HandleFunc("/api/pokedex/search", api.SearchPokemons).Methods("GET")
	r.HandleFunc("/api/pokedex/list", api.GetAllPokemons).Methods("GET")

	r.HandleFunc("/api/types", api.GetAllTypes).Methods("GET")

	// Отображение команды покемонов
	r.HandleFunc("/api/user/team", api.GetUserTeam).Methods("GET")
	r.HandleFunc("/api/user/pokemon/{id}/attacks", api.GetUserPokemonAttacks).Methods("GET")
	r.HandleFunc("/api/user/pokemon/{id}/learnable", api.GetLearnableAttacksHandler).Methods("GET")
	r.HandleFunc("/api/user/pokemon/{id}/learn", api.LearnAttackHandler).Methods("POST")
	r.HandleFunc("/api/user/pokemon/{id}/evolution", api.GetEvolutionHandler).Methods("GET")
	r.HandleFunc("/api/user/pokemon/{id}/evolve", api.EvolveHandler).Methods("POST")
	r.HandleFunc("/api/user/pokemon/{id}/ev", api.DistributeEVHandler).Methods("POST")

	// API для локаций
	r.HandleFunc("/api/locations/{id}", api.GetLocation).Methods("GET")

	// API для пользователя
	r.HandleFunc("/api/user/location", api.GetCurrentUserLocation).Methods("GET")
	r.HandleFunc("/api/user/move", api.MoveUser).Methods("POST")

	// API для питомника
	r.HandleFunc("/api/nursery/pokemons", api.GetUserPokemonsForNursery).Methods("GET")
	r.HandleFunc("/api/nursery/take", api.TakeToTeamHandler).Methods("POST")
	r.HandleFunc("/api/nursery/send", api.SendToNurseryHandler).Methods("POST")

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

	return r
}
