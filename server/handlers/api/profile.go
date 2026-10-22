package api

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"pokemon-game/server/db"
	"pokemon-game/server/session"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
)

// TrainerProfile — карточка тренера. Пароль/почта НЕ включаются.
type TrainerProfile struct {
	UserID     int                  `json:"user_id"`
	Username   string               `json:"username"`
	Online     bool                 `json:"online"`
	RegionName string               `json:"region_name"`
	StatusText string               `json:"status_text"`
	RoleName   string               `json:"role_name"`
	IsSelf     bool                 `json:"is_self"`
	Awards     []db.AwardInfo       `json:"awards"`
	Wishlist   []db.WishlistPokemon `json:"wishlist"`
	Friends    []db.FriendInfo      `json:"friends"`
	Avatar     string               `json:"avatar"` // заглушка под будущий аватар
}

const statusMaxLen = 100

// ProfileHandler обслуживает GET /api/profile (своя, is_self=true) и
// GET /api/profile/{id} (чужая, read-only). Возвращает TrainerProfile.
func ProfileHandler(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.Store.Get(r, "pokemon-session")
	sessUserID, ok := sess.Values["user_id"].(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Целевой профиль: {id} -> чужой; без {id} -> свой.
	targetID := sessUserID
	if idStr, hasID := mux.Vars(r)["id"]; hasID {
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "Invalid user id", http.StatusBadRequest)
			return
		}
		targetID = id
	}
	isSelf := targetID == sessUserID

	profile := TrainerProfile{
		UserID: targetID,
		IsSelf: isSelf,
	}

	var online int
	err := db.DB.QueryRow(`
		SELECT u.username,
		       COALESCE(u.status_text, ''),
		       COALESCE(pr.name, 'Тренер'),
		       COALESCE(r.name, ''),
		       CASE WHEN `+onlineProfileExpr+` THEN 1 ELSE 0 END
		FROM users u
		LEFT JOIN locations l ON u.location_id = l.id
		LEFT JOIN regions r ON l.region_id = r.id
		LEFT JOIN player_roles pr ON u.role_id = pr.id
		WHERE u.id = ?
	`, targetID).Scan(&profile.Username, &profile.StatusText, &profile.RoleName, &profile.RegionName, &online)
	if err == sql.ErrNoRows {
		http.Error(w, "Игрок не найден", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	// Свою карточку всегда показываем онлайн (игрок сейчас смотрит её).
	profile.Online = isSelf || online == 1

	if profile.Awards, err = db.GetUserAwards(targetID); err != nil {
		log.Printf("ProfileHandler awards: %v", err)
		profile.Awards = []db.AwardInfo{}
	}
	if profile.Wishlist, err = db.GetUserWishlist(targetID); err != nil {
		log.Printf("ProfileHandler wishlist: %v", err)
		profile.Wishlist = []db.WishlistPokemon{}
	}
	if profile.Friends, err = db.GetUserFriends(targetID); err != nil {
		log.Printf("ProfileHandler friends: %v", err)
		profile.Friends = []db.FriendInfo{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(profile)
}

// onlineProfileExpr — тот же критерий онлайна, что в db.GetUserFriends (алиас u).
const onlineProfileExpr = "u.last_seen IS NOT NULL AND (strftime('%s','now') - strftime('%s', u.last_seen)) < 300"

type setStatusRequest struct {
	Text string `json:"text"`
}

type setStatusResponse struct {
	Success bool   `json:"success"`
	Status  string `json:"status_text"`
}

// SetStatusHandler — POST /api/profile/status {text}. Меняет статус ТОЛЬКО себе.
func SetStatusHandler(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.Store.Get(r, "pokemon-session")
	userID, ok := sess.Values["user_id"].(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req setStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	text := strings.TrimSpace(req.Text)
	if len([]rune(text)) > statusMaxLen {
		text = string([]rune(text)[:statusMaxLen])
	}

	if _, err := db.DB.Exec("UPDATE users SET status_text = ? WHERE id = ?", text, userID); err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(setStatusResponse{Success: true, Status: text})
}
