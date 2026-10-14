(function () {
  try {
    var persisted = JSON.parse(localStorage.getItem("protovoy:ui-state") || "null");
    var mode = persisted && persisted.state && persisted.state.themeMode;
    if (mode !== "light" && mode !== "dark" && mode !== "system") mode = "dark";
    var theme = mode === "system"
      ? (matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light")
      : mode;
    document.documentElement.dataset.theme = theme;
    var locale = localStorage.getItem("protovoy:locale");
    if (locale !== "zh-CN" && locale !== "en-US") {
      locale = navigator.language.toLowerCase().startsWith("zh") ? "zh-CN" : "en-US";
    }
    document.documentElement.lang = locale;
  } catch (_) {
    document.documentElement.dataset.theme = "dark";
  }
}());
