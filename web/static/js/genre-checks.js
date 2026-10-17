// Componente genreChecks de Alpine.js para el campo de géneros del formulario de historia
// (genreChecks en internal/stories/story_form.templ).
//
// selected guarda los ids marcados en el orden en que se marcaron: las casillas lo usan con
// x-model, que en un array añade al final al marcar y quita al desmarcar. La vista previa lo
// recorre con x-for y envía un input oculto por género en ese orden. Todo sale de selected,
// así que quitar desde la vista previa es solo sacar el id del array.
document.addEventListener("alpine:init", () => {
  Alpine.data("genreChecks", () => ({
    genres: [], // [{id, name}] con el id como texto
    selected: [],
    max: 3,

    init() {
      const data = this.$el.dataset;
      this.genres = JSON.parse(data.genres);
      this.max = Number(data.max);
      // Si el servidor devolvió el formulario con un id que no existe o repetido, se descarta:
      // el mensaje de error ya explica qué pasó.
      const known = this.genres.map((g) => g.id);
      this.selected = [...new Set(JSON.parse(data.selected))].filter((id) => known.includes(id));
      // Barrera por si algo deja marcar de más: el estado nunca pasa del máximo, y x-model
      // desmarca la casilla sobrante.
      this.$watch("selected", (ids) => {
        if (ids.length > this.max) this.selected = ids.slice(0, this.max);
      });
    },

    get full() {
      return this.selected.length >= this.max;
    },

    // isBlocked desactiva las casillas sin marcar cuando ya se llegó al máximo.
    isBlocked(id) {
      return this.full && !this.selected.includes(id);
    },

    nameOf(id) {
      return this.genres.find((g) => g.id === id)?.name ?? id;
    },

    // remove quita el género de la posición i desde la vista previa. El foco pasa a la × del
    // siguiente (o del anterior si era el último); si ya no queda ninguno, a la casilla del
    // género quitado, para no perderlo al usar el teclado. El destino se decide por id antes
    // de quitar: buscarlo por posición después puede encontrar el botón que x-for aún no borró.
    // $refs también se lee antes: Alpine lo resuelve desde el botón pulsado subiendo por sus
    // ancestros, y en $nextTick ese botón ya está fuera del documento y no encuentra ninguno.
    remove(i) {
      const { preview, options } = this.$refs;
      const id = this.selected[i];
      const nextId = this.selected[i + 1] ?? this.selected[i - 1];
      this.selected.splice(i, 1);
      this.$nextTick(() => {
        const target = nextId
          ? preview.querySelector(`li[data-id="${nextId}"] .remove`)
          : options.querySelector(`input[value="${id}"]`);
        target?.focus();
      });
    },

    // resync vuelve a aplicar selected a las casillas. Se llama en pageshow: al volver con
    // «Atrás», el navegador puede restaurar casillas marcadas sin pasar por x-model
    // (autocomplete="off" lo evita en la mayoría de los casos; esto cubre el resto).
    resync() {
      this.selected = [...this.selected];
    },
  }));
});
