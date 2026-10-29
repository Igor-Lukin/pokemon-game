package main

import (
	"log"
	"net/http"
	"pokemon-game/server/db"
	"pokemon-game/server/handlers/api"
	"pokemon-game/server/routes"
	"pokemon-game/server/session"
	"time"
)

func main() {
	// Инициализация базы данных
	db.InitDB()
	defer db.CloseDB()

	// Инициализация хранилища сессий
	session.Init()

	// Запуск тикера погоды
	api.StartWeatherTicker()

	// Настройка роутов
	r := routes.SetupRoutes()

	// Запуск сервера
	srv := &http.Server{
		Addr:              ":8080",
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	log.Println("Сервер запущен на http://localhost:8080")
	log.Fatal(srv.ListenAndServe())
}
