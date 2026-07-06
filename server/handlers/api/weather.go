package api

import (
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"pokemon-game/server/db"
	"pokemon-game/server/session"
	"time"
)

type WeatherResponse struct {
	RegionName   string `json:"region_name"`
	WeatherID    int    `json:"weather_id"`
	WeatherName  string `json:"weather_name"`
}

func GetCurrentWeatherHandler(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.Store.Get(r, "pokemon-session")
	userID, ok := sess.Values["user_id"].(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var regionID int
	var regionName string
	err := db.DB.QueryRow(`
		SELECT r.id, r.name FROM regions r
		JOIN locations l ON l.region_id = r.id
		JOIN users u ON u.location_id = l.id
		WHERE u.id = ?
	`, userID).Scan(&regionID, &regionName)
	if err != nil {
		http.Error(w, "Region not found", http.StatusNotFound)
		return
	}

	var weatherID int
	var weatherName string
	err = db.DB.QueryRow(`
		SELECT w.id, w.name FROM weather w
		JOIN regions r ON r.current_weather_id = w.id
		WHERE r.id = ?
	`, regionID).Scan(&weatherID, &weatherName)
	if err != nil {
		http.Error(w, "Weather not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(WeatherResponse{
		RegionName:  regionName,
		WeatherID:   weatherID,
		WeatherName: weatherName,
	})
}

func StartWeatherTicker() {
	go func() {
		randomizeWeather()
		ticker := time.NewTicker(20 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			randomizeWeather()
		}
	}()
	log.Println("Погода: тикер запущен (20 мин)")
}

func randomizeWeather() {
	rows, err := db.DB.Query("SELECT id FROM regions")
	if err != nil {
		log.Printf("Погода: ошибка получения регионов: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var regionID int
		rows.Scan(&regionID)
		newWeather := rand.Intn(5) + 1
		db.DB.Exec("UPDATE regions SET current_weather_id = ? WHERE id = ?", newWeather, regionID)
	}
	log.Println("Погода: обновлена для всех регионов")
}
