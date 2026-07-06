document.addEventListener("DOMContentLoaded", () => {
  // Обработка кнопки "Регистрация"
  const registerBtn = document.getElementById("register-btn");
  if (registerBtn) {
    registerBtn.addEventListener("click", (e) => {
      e.preventDefault(); // Отменяем стандартное поведение ссылки
      window.location.href = "/register"; // Перенаправляем на страницу регистрации
    });
  }

  // Обработка кнопки "Авторизация"
  const loginBtn = document.getElementById("login-btn");
  if (loginBtn) {
    loginBtn.addEventListener("click", (e) => {
      e.preventDefault(); // Отменяем стандартное поведение ссылки
      window.location.href = "/login"; // Перенаправляем на страницу авторизации
    });
  }
});
