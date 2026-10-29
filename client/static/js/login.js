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
        window.location.href = "/game";
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
