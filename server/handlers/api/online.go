package api

// Онлайн-игроки на локации + флоу вызова на бой (поток [17], часть A).
// Presence НЕ храним отдельно — считаем из users.last_seen + location_id на лету
// (то же правило онлайна, что в карточке тренера [7]: last_seen < 5 минут).
// Вызовы — эфемерные, в памяти под мьютексом с TTL (как battleSessions). Транспорт —
// HTTP-поллинг, но модель серверная: позже поверх можно навесить ws-пуш без смены
// форматов ответов.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"pokemon-game/server/db"
	"pokemon-game/server/session"
	"sync"
	"time"
)

// Константы-«ручки».
const (
	onlineWindow = 5 * time.Minute  // порог онлайна (как в [7])
	challengeTTL = 45 * time.Second // время жизни вызова
)

// challenge — эфемерный вызов на бой (в памяти).
type challenge struct {
	ChallengerID   int
	ChallengerName string
	TargetID       int
	TargetName     string
	CreatedAt      time.Time
	Status         string // pending | accepted | declined
}

var (
	// Ключ — пара "challengerID-targetID".
	challenges  = map[string]*challenge{}
	challengeMu sync.Mutex
)

func challengeKey(challengerID, targetID int) string {
	return fmt.Sprintf("%d-%d", challengerID, targetID)
}

func init() {
	go cleanupChallenges()
}

// cleanupChallenges убирает протухшие (>TTL) вызовы.
func cleanupChallenges() {
	for {
		time.Sleep(challengeTTL)
		challengeMu.Lock()
		now := time.Now()
		for k, c := range challenges {
			if now.Sub(c.CreatedAt) > challengeTTL {
				delete(challenges, k)
			}
		}
		challengeMu.Unlock()
	}
}

// onlineUserID — userID из сессии (или Unauthorized).
func onlineUserID(w http.ResponseWriter, r *http.Request) (int, bool) {
	sess, _ := session.Store.Get(r, "pokemon-session")
	uid, ok := sess.Values["user_id"].(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return 0, false
	}
	return uid, true
}

// onlineSQLExpr — SQL-условие онлайна по last_seen (UTC), окно onlineWindow.
func onlineSQLExpr(alias string) string {
	return fmt.Sprintf(
		"%s.last_seen IS NOT NULL AND (strftime('%%s','now') - strftime('%%s', %s.last_seen)) < %d",
		alias, alias, int(onlineWindow.Seconds()))
}

type onlinePlayer struct {
	UserID   int    `json:"user_id"`
	Username string `json:"username"`
}

// LocationPlayersHandler — GET /api/location/players.
// Кто онлайн на МОЕЙ локации (кроме меня).
func LocationPlayersHandler(w http.ResponseWriter, r *http.Request) {
	uid, ok := onlineUserID(w, r)
	if !ok {
		return
	}

	var myLoc int
	if err := db.DB.QueryRow("SELECT location_id FROM users WHERE id = ?", uid).Scan(&myLoc); err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	q := fmt.Sprintf(`
		SELECT u.id, u.username
		FROM users u
		WHERE u.location_id = ? AND u.id != ? AND %s
		ORDER BY u.username`, onlineSQLExpr("u"))

	rows, err := db.DB.Query(q, myLoc, uid)
	players := []onlinePlayer{}
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var p onlinePlayer
			if rows.Scan(&p.UserID, &p.Username) == nil {
				players = append(players, p)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(players)
}

type challengeRequest struct {
	TargetUserID int `json:"target_user_id"`
}

// ChallengeCreateHandler — POST /api/challenge {target_user_id}.
// Гарды: не себя; цель онлайн и на той же локации; нет активного дубля.
func ChallengeCreateHandler(w http.ResponseWriter, r *http.Request) {
	uid, ok := onlineUserID(w, r)
	if !ok {
		return
	}

	var req challengeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.TargetUserID == 0 {
		http.Error(w, "Некорректный запрос", http.StatusBadRequest)
		return
	}
	if req.TargetUserID == uid {
		http.Error(w, "Нельзя вызвать самого себя", http.StatusBadRequest)
		return
	}

	// Моя локация и имя.
	var myLoc int
	var myName string
	if err := db.DB.QueryRow("SELECT location_id, username FROM users WHERE id = ?", uid).Scan(&myLoc, &myName); err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// Цель: локация, имя, онлайн?
	var tgtLoc, tgtOnline int
	var tgtName string
	err := db.DB.QueryRow(fmt.Sprintf(
		"SELECT location_id, username, CASE WHEN %s THEN 1 ELSE 0 END FROM users WHERE id = ?", onlineSQLExpr("users")),
		req.TargetUserID).Scan(&tgtLoc, &tgtName, &tgtOnline)
	if err == sql.ErrNoRows {
		http.Error(w, "Игрок не найден", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	if tgtOnline != 1 {
		http.Error(w, "Игрок не в сети", http.StatusBadRequest)
		return
	}
	if tgtLoc != myLoc {
		http.Error(w, "Игрок ушёл с локации", http.StatusBadRequest)
		return
	}

	challengeMu.Lock()
	key := challengeKey(uid, req.TargetUserID)
	if c, exists := challenges[key]; exists && c.Status == "pending" && time.Since(c.CreatedAt) <= challengeTTL {
		challengeMu.Unlock()
		http.Error(w, "Вызов уже отправлен", http.StatusBadRequest)
		return
	}
	challenges[key] = &challenge{
		ChallengerID:   uid,
		ChallengerName: myName,
		TargetID:       req.TargetUserID,
		TargetName:     tgtName,
		CreatedAt:      time.Now(),
		Status:         "pending",
	}
	challengeMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

type incomingChallenge struct {
	ChallengerID   int    `json:"challenger_id"`
	ChallengerName string `json:"challenger_name"`
}

// ChallengeIncomingHandler — GET /api/challenge/incoming.
// Входящие pending-вызовы мне (протухшие чистим и не отдаём).
func ChallengeIncomingHandler(w http.ResponseWriter, r *http.Request) {
	uid, ok := onlineUserID(w, r)
	if !ok {
		return
	}

	list := []incomingChallenge{}
	challengeMu.Lock()
	now := time.Now()
	for k, c := range challenges {
		if now.Sub(c.CreatedAt) > challengeTTL {
			delete(challenges, k)
			continue
		}
		if c.TargetID == uid && c.Status == "pending" {
			list = append(list, incomingChallenge{ChallengerID: c.ChallengerID, ChallengerName: c.ChallengerName})
		}
	}
	challengeMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

type respondRequest struct {
	ChallengerID int  `json:"challenger_id"`
	Accept       bool `json:"accept"`
}

// ChallengeRespondHandler — POST /api/challenge/respond {challenger_id, accept}.
// accept=false -> declined; accept=true -> accepted + заглушка startPvpBattle.
func ChallengeRespondHandler(w http.ResponseWriter, r *http.Request) {
	uid, ok := onlineUserID(w, r)
	if !ok {
		return
	}

	var req respondRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Некорректный запрос", http.StatusBadRequest)
		return
	}

	key := challengeKey(req.ChallengerID, uid)
	challengeMu.Lock()
	c, exists := challenges[key]
	if !exists || c.Status != "pending" || time.Since(c.CreatedAt) > challengeTTL {
		challengeMu.Unlock()
		http.Error(w, "Вызов не найден или истёк", http.StatusBadRequest)
		return
	}
	if req.Accept {
		c.Status = "accepted"
	} else {
		c.Status = "declined"
	}
	challengeMu.Unlock()

	if req.Accept {
		// Обе стороны увидят accepted по поллингу; тут дёргаем серверную заглушку.
		startPvpBattle(req.ChallengerID, uid)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// startPvpBattle реализован в pvp.go (часть B): создаёт PvP-сессию на «принять».

type outgoingStatus struct {
	Status     string `json:"status"` // none | pending | accepted | declined
	TargetName string `json:"target_name"`
}

// ChallengeOutgoingHandler — GET /api/challenge/outgoing.
// Статус моего исходящего вызова. После терминального статуса вызов чистим.
func ChallengeOutgoingHandler(w http.ResponseWriter, r *http.Request) {
	uid, ok := onlineUserID(w, r)
	if !ok {
		return
	}

	resp := outgoingStatus{Status: "none"}
	challengeMu.Lock()
	now := time.Now()
	for k, c := range challenges {
		if now.Sub(c.CreatedAt) > challengeTTL {
			delete(challenges, k)
			continue
		}
		if c.ChallengerID == uid {
			resp.Status = c.Status
			resp.TargetName = c.TargetName
			if c.Status == "accepted" || c.Status == "declined" {
				delete(challenges, k) // показали инициатору — можно очистить
			}
			break
		}
	}
	challengeMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
