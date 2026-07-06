package main

import (
	"log"
	"net/http"
	"pokemon-game/server/db"
	"pokemon-game/server/handlers/api"
	"pokemon-game/server/routes"
	"pokemon-game/server/session"
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
	log.Println("Сервер запущен на http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}
