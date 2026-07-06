package api

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"pokemon-game/server/db"
	"pokemon-game/server/session"
)

type UserLocationResponse struct {
	LocationID int `json:"location_id"`
}

type MoveRequest struct {
	LocationID int `json:"location_id"`
}

// GetCurrentUserLocation возвращает location_id текущего пользователя
func GetCurrentUserLocation(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.Store.Get(r, "pokemon-session")
	userID, ok := sess.Values["user_id"].(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var locationID int
	err := db.DB.QueryRow("SELECT location_id FROM users WHERE id = ?", userID).Scan(&locationID)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(UserLocationResponse{LocationID: locationID})
}

// MoveUser обновляет локацию пользователя
func MoveUser(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.Store.Get(r, "pokemon-session")
	userID, ok := sess.Values["user_id"].(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Читаем тело запроса
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Cannot read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	log.Printf("Received body: %s", string(body))

	var req MoveRequest
	if err := json.Unmarshal(body, &req); err != nil {
		log.Printf("JSON parse error: %v", err)
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.LocationID == 0 {
		http.Error(w, "location_id is required", http.StatusBadRequest)
		return
	}

	// Проверяем существование локации
	var exists int
	err = db.DB.QueryRow("SELECT COUNT(*) FROM locations WHERE id = ?", req.LocationID).Scan(&exists)
	if err != nil || exists == 0 {
		http.Error(w, "Location not found", http.StatusBadRequest)
		return
	}

	_, err = db.DB.Exec("UPDATE users SET location_id = ? WHERE id = ?", req.LocationID, userID)
	if err != nil {
		http.Error(w, "Failed to update location", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
