document.addEventListener("DOMContentLoaded", () => {
  const form = document.getElementById("login-form");

  form.addEventListener("submit", async (e) => {
    e.preventDefault();

    const formData = new FormData(form);

    try {
      const response = await fetch("/login", {
        method: "POST",
        body: formData,
      });

      if (response.ok) {
        document.getElementsByTagName("h1")[0].textContent =
          "Авторизация успешна. Войдите в мир игры!";
        document.getElementById("login-form").hidden = true;

        const newButton = document.createElement("button");
        newButton.textContent = "Войти в мир!";
        newButton.addEventListener("click", async () => {
          console.log("Кнопка нажата"); // отладка

          try {
            const gameResponse = await fetch("/game", {
              method: "POST",
              headers: {
                "Content-Type": "application/json",
              },
              body: formData, // Пример тела запроса
            });
            console.log("Статус ответа /game:", gameResponse.status); // отладка
            if (gameResponse.ok) {
              // Перенаправляем на страницу игры
              console.log("Редирект на /game"); // отладка
              window.location.href = "/game";
            } else {
              const error = await gameResponse.text();
              console.error("Ошибка входа в игру:", error);
              alert("Ошибка входа в игру: " + error);
            }
          } catch (error) {
            console.error("Ошибка сети:", error);
            alert("Ошибка сети: " + error.message);
          }
        });

        document.getElementsByClassName("container")[0].appendChild(newButton);
      } else if (!response.ok) {
        const error = await response.text();
        showError(error);
      }
    } catch (error) {
      showError("Ошибка сети");
    }
  });

  function showError(message) {
    const errorDiv = document.createElement("div");
    errorDiv.className = "error-message";
    errorDiv.textContent = message;
    form.prepend(errorDiv);
  }
});
