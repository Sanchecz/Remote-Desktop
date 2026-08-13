try {
  const savedTheme = localStorage.getItem("genesis-theme");
  document.documentElement.dataset.theme = ["dark", "light", "blue"].includes(savedTheme) ? savedTheme : "light";
} catch (_) {
  document.documentElement.dataset.theme = "light";
}
