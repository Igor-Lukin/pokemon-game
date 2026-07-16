package api

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"pokemon-game/server/db"
	"pokemon-game/server/session"
	"strconv"

	"github.com/gorilla/mux"
)

type holdItemRequest struct {
	ItemID int `json:"item_id"`
}

type holdItemResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// HoldItemHandler — POST /api/user/pokemon/{id}/hold {item_id}. Надевает held-предмет
// на покемона игрока: свап под транзакцией (старый предмет возвращается в сумку).
func HoldItemHandler(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.Store.Get(r, "pokemon-session")
	userID, ok := sess.Values["user_id"].(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	userPokemonID, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "Invalid pokemon id", http.StatusBadRequest)
		return
	}

	var req holdItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ItemID <= 0 {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Предмет должен существовать и быть надеваемым.
	item, err := db.GetItemByID(req.ItemID)
	if err != nil {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}
	if !db.IsHeldItem(item) {
		http.Error(w, "Этот предмет нельзя надеть", http.StatusBadRequest)
		return
	}

	held, err := db.HoldItem(userID, userPokemonID, req.ItemID)
	if err == sql.ErrNoRows {
		http.Error(w, "Покемон не найден", http.StatusForbidden)
		return
	}
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	if !held {
		http.Error(w, "У вас нет этого предмета", http.StatusConflict)
		return
	}

	log.Printf("User %d held item %s (id=%d) on pokemon %d", userID, item.Name, item.ID, userPokemonID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(holdItemResponse{Success: true, Message: item.Name + " надет на покемона"})
}

// UnholdItemHandler — POST /api/user/pokemon/{id}/unhold. Снимает надетый предмет,
// возвращая его в сумку.
func UnholdItemHandler(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.Store.Get(r, "pokemon-session")
	userID, ok := sess.Values["user_id"].(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	userPokemonID, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "Invalid pokemon id", http.StatusBadRequest)
		return
	}

	removed, err := db.UnholdItem(userID, userPokemonID)
	if err == sql.ErrNoRows {
		http.Error(w, "Покемон не найден", http.StatusForbidden)
		return
	}
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	if !removed {
		http.Error(w, "На покемоне нет предмета", http.StatusConflict)
		return
	}

	log.Printf("User %d unheld item from pokemon %d", userID, userPokemonID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(holdItemResponse{Success: true, Message: "Предмет снят"})
}
