document.addEventListener("DOMContentLoaded", () => {
  const form = document.getElementById("register-form");

  form.addEventListener("submit", async (e) => {
    e.preventDefault();

    const data = {
      username: form.username.value,
      email: form.email.value,
      password: form.password.value,
      confirm_password: form.confirm_password.value,
    };

    try {
      const response = await fetch("/register", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify(data),
      });

      if (response.ok) {
        window.location.href = "/";
      } else {
        const error = await response.text();
        showError(error);
      }
    } catch (error) {
      showError("Network error");
    }
  });

  function showError(message) {
    const errorDiv = document.createElement("div");
    errorDiv.className = "error-message";
    errorDiv.textContent = message;
    form.prepend(errorDiv);
  }
});
