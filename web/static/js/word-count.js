// Componente de Alpine.js para las áreas de texto con un mínimo de palabras: el texto del
// capítulo (chapter.templ, minPublishWords) y la sinopsis (story_form.templ,
// minSynopsisWords). Cuenta las palabras mientras el autor escribe y avisa de cuántas
// faltan. El mínimo llega en data-min y el final del aviso («faltan 3 para …») en data-goal.
// El servidor valida lo mismo, así que sin JS el autor ve el error al enviar; el recuento
// solo se dibuja con JS (template x-if).

document.addEventListener("alpine:init", () => {
  Alpine.data("wordCount", () => ({
    words: 0,
    min: 0,
    goal: "",

    init() {
      this.min = Number(this.$el.dataset.min);
      this.goal = this.$el.dataset.goal;
    },

    // Separa por espacios en blanco, como strings.Fields en countWords (Go).
    count(text) {
      const trimmed = text.trim();
      this.words = trimmed === "" ? 0 : trimmed.split(/\s+/).length;
    },

    label() {
      const n = this.words === 1 ? "1 palabra" : `${this.words} palabras`;
      if (this.words >= this.min) return n;
      const missing = this.min - this.words;
      return `${n} · ${missing === 1 ? "falta 1" : `faltan ${missing}`} para ${this.goal}`;
    },
  }));
});
