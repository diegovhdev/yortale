// Componente de Alpine.js para el campo de advertencias del formulario de historia
// (story_form.templ). «No aplica ninguna» (none) no se puede combinar con otras: al marcarla
// se desmarcan las demás, y al marcar otra se desmarca ella. El servidor valida lo mismo,
// así que sin JS el autor ve el error al enviar.

document.addEventListener("alpine:init", () => {
  const NONE = "none";

  Alpine.data("warningChecks", () => ({
    selected: [],

    init() {
      this.selected = JSON.parse(this.$el.dataset.selected);
      // Se mira qué se acaba de marcar para saber cuál de las dos reglas aplicar.
      this.$watch("selected", (now, before) => {
        const added = now.filter((v) => !before.includes(v));
        if (added.length === 0 || !now.includes(NONE) || now.length === 1) return;
        this.selected = added.includes(NONE) ? [NONE] : now.filter((v) => v !== NONE);
      });
    },

    // resync vuelve a aplicar selected a las casillas al volver con «Atrás» (ver genre-checks.js).
    resync() {
      this.selected = [...this.selected];
    },
  }));
});
